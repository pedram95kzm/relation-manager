package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"relations/internal/ai"
	"relations/internal/auth"
	"relations/internal/config"
	"relations/internal/database"
	"relations/internal/platform"
)

func adminTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for isolated PostgreSQL admin integration test")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(strings.ToLower(u.Path), "test") {
		t.Fatal("refusing non-test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { _ = sql.Close() })
	tx := db.WithContext(ctx).Begin()
	t.Cleanup(func() { _ = tx.Rollback().Error })
	schemaName := "test_admin_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = tx.Exec(`CREATE SCHEMA "` + schemaName + `"`).Error; err != nil {
		t.Fatal(err)
	}
	if err = tx.Exec(`SET LOCAL search_path TO "` + schemaName + `"`).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, tx); err != nil {
		t.Fatal(err)
	}
	return tx
}
func adminConfig() config.Config {
	return config.Config{EncryptionKey: []byte(strings.Repeat("k", 32)), JWTSecret: strings.Repeat("s", 48), AccessTTL: 15 * time.Minute, RefreshTTL: 30 * 24 * time.Hour, GlobalDailyLimit: 100, UserDailyLimit: 7}
}
func makeUser(t *testing.T, db *gorm.DB, role string) database.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("strong-password-123"), 4)
	if err != nil {
		t.Fatal(err)
	}
	u := database.User{ID: uuid.NewString(), Email: uuid.NewString() + "@example.test", Name: "Account", PasswordHash: string(hash), Role: role, AuthVersion: 1}
	if err = db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u
}
func adminErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	var appErr *platform.Error
	if !errors.As(err, &appErr) || appErr.Code != want {
		t.Fatal("expected", want, "got", err)
	}
}

func TestUserManagementRevokesSessionsAndProtectsAdministrators(t *testing.T) {
	db := adminTestDB(t)
	cfg := adminConfig()
	service := New(cfg, db, ai.New(cfg, db))
	ctx := context.Background()
	actor := makeUser(t, db, "admin")
	target := makeUser(t, db, "user")
	accounts := auth.New(cfg, db)
	tokens, err := accounts.Login(ctx, auth.LoginInput{Email: target.Email, Password: "strong-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	disabled := true
	user, err := service.UpdateUser(ctx, actor.ID, target.ID, UserInput{Disabled: &disabled, DailyLimit: json.RawMessage("3")})
	if err != nil || !user.Disabled || user.DailyLimit == nil || *user.DailyLimit != 3 {
		t.Fatal(user, err)
	}
	if _, err = accounts.Authenticate(ctx, tokens.AccessToken); err == nil {
		t.Fatal("disabled access token still valid")
	}
	if _, err = accounts.Refresh(ctx, tokens.RefreshToken); err == nil {
		t.Fatal("disabled refresh token still valid")
	}
	otherAdmin := makeUser(t, db, "admin")
	_, err = service.UpdateUser(ctx, actor.ID, otherAdmin.ID, UserInput{Disabled: &disabled})
	adminErrorCode(t, err, "protected_administrator")
	_, err = service.UpdateUser(ctx, actor.ID, actor.ID, UserInput{Disabled: &disabled})
	adminErrorCode(t, err, "protected_administrator")
	disabled = false
	user, err = service.UpdateUser(ctx, actor.ID, target.ID, UserInput{Disabled: &disabled, DailyLimit: json.RawMessage("null")})
	if err != nil || user.Disabled || user.DailyLimit != nil {
		t.Fatal("clear override", user, err)
	}
	users, err := service.Users(ctx, target.Email, 1, 20)
	if err != nil || users.Total != 1 {
		t.Fatal(users, err)
	}
	data, _ := json.Marshal(users)
	if strings.Contains(string(data), "password_hash") || strings.Contains(string(data), `"age":`) || strings.Contains(string(data), `"gender":`) {
		t.Fatal("unneeded personal data")
	}
	users, err = service.Users(ctx, "%", 1, 20)
	if err != nil || users.Total != 0 {
		t.Fatal("search wildcard not escaped", users, err)
	}
	_, err = service.UpdateUser(ctx, actor.ID, target.ID, UserInput{DailyLimit: json.RawMessage("0")})
	adminErrorCode(t, err, "invalid_limit")
	audit, err := service.Audit(ctx, 1, 20)
	if err != nil || audit.Total != 2 {
		t.Fatal(audit, err)
	}
}

func TestLimitsAndAnalyticsUsePersistedOperationalData(t *testing.T) {
	db := adminTestDB(t)
	cfg := adminConfig()
	service := New(cfg, db, ai.New(cfg, db))
	ctx := context.Background()
	actor := makeUser(t, db, "admin")
	limits, err := service.GetLimits(ctx)
	if err != nil || limits.GlobalDaily != 100 || limits.UserDaily != 7 {
		t.Fatal(limits, err)
	}
	if _, err = service.SetLimits(ctx, actor.ID, Limits{50, 4}); err != nil {
		t.Fatal(err)
	}
	limits, err = service.GetLimits(ctx)
	if err != nil || limits.GlobalDaily != 50 || limits.UserDaily != 4 {
		t.Fatal("defaults replaced saved limits", limits, err)
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	if err = db.Create(&database.DailyUsage{Day: today, ScopeKey: "global", Counter: 2}).Error; err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"success", "error"} {
		if err = db.Create(&database.UsageEvent{ID: uuid.NewString(), UserID: &actor.ID, Provider: "openai", Model: "sample-model", Status: status, InputTokens: 10, OutputTokens: 5}).Error; err != nil {
			t.Fatal(err)
		}
	}
	result, err := service.Analytics(ctx, 7)
	if err != nil || result.Users != 1 || result.Analyses != 0 || result.Generations != 2 || result.SuccessfulGenerations != 1 || result.FailedGenerations != 1 || result.InputTokens != 20 || result.OutputTokens != 10 || result.TodayGlobalUsage != 2 || len(result.Daily) != 7 || result.Daily[6].Generations != 2 || len(result.Providers) != 1 {
		t.Fatal(result, err)
	}
	_, err = service.Analytics(ctx, 91)
	adminErrorCode(t, err, "invalid_range")
}

func TestAdministratorErasureSupportsGoogleOnlyAndRequiresReauthentication(t *testing.T) {
	db := adminTestDB(t)
	cfg := adminConfig()
	service := New(cfg, db, ai.New(cfg, db))
	ctx := context.Background()
	actor := makeUser(t, db, "admin")
	target := makeUser(t, db, "user")
	if err := db.Model(&target).Update("password_hash", "").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.AuthIdentity{ID: uuid.NewString(), UserID: target.ID, Provider: "google", Subject: "google-subject"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.DailyUsage{Day: time.Now().UTC(), ScopeKey: "user:" + target.ID, Counter: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.UsageEvent{ID: uuid.NewString(), UserID: &target.ID, Provider: "openai", Status: "success"}).Error; err != nil {
		t.Fatal(err)
	}
	adminErrorCode(t, service.DeleteUser(ctx, actor.ID, target.ID, DeleteUserInput{CurrentPassword: "wrong", ConfirmEmail: target.Email}), "reauthentication_required")
	adminErrorCode(t, service.DeleteUser(ctx, actor.ID, target.ID, DeleteUserInput{CurrentPassword: "strong-password-123", ConfirmEmail: "wrong@example.test"}), "confirmation_required")
	adminErrorCode(t, service.DeleteUser(ctx, actor.ID, actor.ID, DeleteUserInput{CurrentPassword: "strong-password-123", ConfirmEmail: actor.Email}), "protected_administrator")
	if err := service.DeleteUser(ctx, actor.ID, target.ID, DeleteUserInput{CurrentPassword: "strong-password-123", ConfirmEmail: target.Email}); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&database.User{}).Where("id = ?", target.ID).Count(&count)
	if count != 0 {
		t.Fatal("account survived")
	}
	db.Model(&database.AuthIdentity{}).Where("user_id = ?", target.ID).Count(&count)
	if count != 0 {
		t.Fatal("identity survived")
	}
	db.Model(&database.DailyUsage{}).Where("scope_key = ?", "user:"+target.ID).Count(&count)
	if count != 0 {
		t.Fatal("quota row survived")
	}
	var event database.UsageEvent
	if err := db.First(&event).Error; err != nil || event.UserID != nil {
		t.Fatal("operational anonymization", event, err)
	}
	audit, err := service.Audit(ctx, 1, 20)
	if err != nil || audit.Total != 1 || audit.Items[0].Action != "user.deleted" || strings.Contains(string(audit.Items[0].Metadata), target.Email) {
		t.Fatal(audit, err)
	}
}
