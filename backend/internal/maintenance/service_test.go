package maintenance

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"relations/internal/auth"
	"relations/internal/config"
	"relations/internal/database"
)

func maintenanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for isolated PostgreSQL maintenance integration test")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(strings.ToLower(u.Path), "test") {
		t.Fatal("refusing non-test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	manager, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	managerSQL, _ := manager.DB()
	t.Cleanup(func() { _ = managerSQL.Close() })
	schemaName := "test_maintenance_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = manager.Exec(`CREATE SCHEMA "` + schemaName + `"`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Exec(`DROP SCHEMA "` + schemaName + `" CASCADE`).Error })
	query := u.Query()
	query.Set("search_path", schemaName)
	u.RawQuery = query.Encode()
	db, err := database.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { _ = sql.Close() })
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db
}

func maintenanceUser(t *testing.T, db *gorm.DB) database.User {
	t.Helper()
	user := database.User{ID: uuid.NewString(), Email: uuid.NewString() + "@example.test", Name: "Maintenance fixture", Role: "user", AuthVersion: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func maintenanceSession(t *testing.T, db *gorm.DB, user database.User, family string, expires time.Time, index int) database.RefreshSession {
	t.Helper()
	session := database.RefreshSession{ID: uuid.NewString(), UserID: user.ID, FamilyID: family, TokenHash: fmt.Sprintf("%064x", index), CreatedAt: expires.Add(-30 * 24 * time.Hour), ExpiresAt: expires}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	return session
}

func TestCleanupIsBoundedAndPreservesLiveFamilies(t *testing.T) {
	db := maintenanceTestDB(t)
	service := New(db)
	service.batch = 2
	now := time.Now().UTC()
	service.now = func() time.Time { return now }
	user := maintenanceUser(t, db)
	family := uuid.NewString()
	expired := maintenanceSession(t, db, user, family, now.Add(-time.Hour), 1)
	usedAt := now.Add(-2 * time.Hour)
	if err := db.Model(&expired).Update("used_at", usedAt).Error; err != nil {
		t.Fatal(err)
	}
	live := maintenanceSession(t, db, user, family, now.Add(time.Hour), 2)
	for i := 3; i <= 5; i++ {
		maintenanceSession(t, db, user, uuid.NewString(), now.Add(-time.Hour), i)
	}
	for i := 1; i <= 3; i++ {
		state := database.OAuthState{StateHash: fmt.Sprintf("%064x", i), NonceHash: strings.Repeat("a", 64), Verifier: strings.Repeat("v", 43), CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute)}
		if err := db.Create(&state).Error; err != nil {
			t.Fatal(err)
		}
	}
	validState := database.OAuthState{StateHash: strings.Repeat("f", 64), NonceHash: strings.Repeat("b", 64), Verifier: strings.Repeat("w", 43), CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	if err := db.Create(&validState).Error; err != nil {
		t.Fatal(err)
	}
	stats, err := service.Cleanup(context.Background())
	if err != nil || stats.RefreshSessions != 2 || stats.OAuthStates != 2 {
		t.Fatal(stats, err)
	}
	var count int64
	if err := db.Model(&database.RefreshSession{}).Where("family_id = ?", family).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("deleted live-family replay ancestor", count, err)
	}
	stats, err = service.Cleanup(context.Background())
	if err != nil || stats.RefreshSessions != 1 || stats.OAuthStates != 1 {
		t.Fatal(stats, err)
	}
	if err := db.Model(&live).Update("revoked_at", now).Error; err != nil {
		t.Fatal(err)
	}
	stats, err = service.Cleanup(context.Background())
	if err != nil || stats.RefreshSessions != 1 || stats.OAuthStates != 0 {
		t.Fatal(stats, err)
	}
	if err := db.Model(&database.RefreshSession{}).Where("id = ?", live.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("deleted unexpired revoked session", count, err)
	}
	if err := db.Model(&database.OAuthState{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("deleted unexpired OAuth state", count, err)
	}
}

func TestCleanupSkipsUsersLockedBySessionMutation(t *testing.T) {
	db := maintenanceTestDB(t)
	service := New(db)
	user := maintenanceUser(t, db)
	now := time.Now().UTC()
	service.now = func() time.Time { return now }
	maintenanceSession(t, db, user, uuid.NewString(), now.Add(-time.Hour), 1)
	tx := db.Begin()
	defer tx.Rollback()
	if err := tx.Exec("SELECT id FROM users WHERE id = ? FOR UPDATE", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stats, err := service.Cleanup(ctx)
	if err != nil || stats.RefreshSessions != 0 {
		t.Fatal("cleanup waited for active session operation", stats, err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	stats, err = service.Cleanup(context.Background())
	if err != nil || stats.RefreshSessions != 1 {
		t.Fatal(stats, err)
	}
}

func TestCleanupHonorsCancellation(t *testing.T) {
	db := maintenanceTestDB(t)
	service := New(db)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Cleanup(ctx); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestCleanupPreservesExpiredAncestorReplayRevocation(t *testing.T) {
	db := maintenanceTestDB(t)
	ctx := context.Background()
	accounts := auth.New(config.Config{JWTSecret: strings.Repeat("s", 48), AccessTTL: 15 * time.Minute, RefreshTTL: 30 * 24 * time.Hour, MinAge: 18}, db)
	first, err := accounts.Register(ctx, auth.RegisterInput{Email: "replay@example.test", Name: "Replay fixture", Password: "strong-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	next, err := accounts.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err = db.Model(&database.RefreshSession{}).Where("user_id = ? AND used_at IS NOT NULL", first.User.ID).Updates(map[string]any{"created_at": now.Add(-2 * time.Hour), "expires_at": now.Add(-time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	stats, err := New(db).Cleanup(ctx)
	if err != nil || stats.RefreshSessions != 0 {
		t.Fatal("removed replay tombstone", stats, err)
	}
	if _, err = accounts.Refresh(ctx, first.RefreshToken); err == nil {
		t.Fatal("reused ancestor accepted")
	}
	if _, err = accounts.Authenticate(ctx, next.AccessToken); err == nil {
		t.Fatal("expired ancestor replay no longer revoked live descendant")
	}
	stats, err = New(db).Cleanup(ctx)
	if err != nil || stats.RefreshSessions != 1 {
		t.Fatal("retired expired ancestor retained", stats, err)
	}
}
