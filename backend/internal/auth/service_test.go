package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"relations/internal/config"
	"relations/internal/database"
	"relations/internal/platform"
)

const testPassword = "strong-password-123"

func authTestConfig() config.Config {
	return config.Config{Environment: "test", JWTSecret: strings.Repeat("s", 48), PublicURL: "http://localhost:5173", AllowedOrigins: []string{"http://localhost:5173"}, MinAge: 18, AccessTTL: 15 * time.Minute, RefreshTTL: 30 * 24 * time.Hour}
}

// Separate schemas permit real multi-connection concurrency tests without
// rolling back or deleting another package's application fixtures.
func authTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for isolated PostgreSQL integration test")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || !strings.Contains(strings.ToLower(u.Path), "test") {
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
	schema := "test_auth_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = manager.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Exec(`DROP SCHEMA "` + schema + `" CASCADE`).Error })
	query := u.Query()
	query.Set("search_path", schema)
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

func expectCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *platform.Error
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}

func registerFixture(t *testing.T, service *Service) SessionTokens {
	t.Helper()
	result, err := service.Register(context.Background(), RegisterInput{Email: uuid.NewString() + "@example.test", Name: "آزمایش", Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestAccountProfileAndRotatingSessions(t *testing.T) {
	db := authTestDB(t)
	service := New(authTestConfig(), db)
	ctx := context.Background()
	first, err := service.Register(ctx, RegisterInput{Email: "  PERSON@EXAMPLE.TEST  ", Name: "First account", Password: testPassword})
	if err != nil || first.User.Email != "person@example.test" || first.User.Role != "user" || service.ProfileComplete(first.User) {
		t.Fatal(first.User, err)
	}
	if cost, err := bcrypt.Cost([]byte(first.User.PasswordHash)); err != nil || cost != 12 {
		t.Fatal("password hash cost", cost, err)
	}
	if _, err = service.Authenticate(ctx, first.AccessToken); err != nil {
		t.Fatal(err)
	}
	if _, err = service.UpdateProfile(ctx, first.User.ID, ProfileInput{Name: "Name", Age: 17, Gender: "other"}); err == nil {
		t.Fatal("accepted underage profile")
	}
	if _, err = service.UpdateProfile(ctx, first.User.ID, ProfileInput{Name: "Name", Age: 25, Gender: "invalid"}); err == nil {
		t.Fatal("accepted invalid gender")
	}
	user, err := service.UpdateProfile(ctx, first.User.ID, ProfileInput{Name: "نام", Age: 25, Gender: "prefer_not_to_say"})
	if err != nil || !service.ProfileComplete(user) {
		t.Fatal(user, err)
	}
	for _, login := range []LoginInput{{Email: user.Email, Password: "wrong-password-123"}, {Email: "missing@example.test", Password: testPassword}} {
		_, err := service.Login(ctx, login)
		expectCode(t, err, "invalid_credentials")
	}
	second, err := service.Login(ctx, LoginInput{Email: user.Email, Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := service.Refresh(ctx, first.RefreshToken)
	if err != nil || rotated.RefreshToken == first.RefreshToken {
		t.Fatal("refresh did not rotate", err)
	}
	var session database.RefreshSession
	if err = db.Where("token_hash = ?", hashToken(first.RefreshToken)).First(&session).Error; err != nil || session.UsedAt == nil || strings.Contains(session.TokenHash, first.RefreshToken) {
		t.Fatal("refresh state was not hashed/marked used", err)
	}
	_, err = service.Refresh(ctx, first.RefreshToken)
	expectCode(t, err, "invalid_credentials")
	for _, access := range []string{first.AccessToken, rotated.AccessToken} {
		_, err = service.Authenticate(ctx, access)
		expectCode(t, err, "invalid_credentials")
	}
	if _, err = service.Authenticate(ctx, second.AccessToken); err != nil {
		t.Fatal("reuse revoked unrelated family", err)
	}
	if err = service.Logout(ctx, second.RefreshToken); err != nil {
		t.Fatal(err)
	}
	_, err = service.Authenticate(ctx, second.AccessToken)
	expectCode(t, err, "invalid_credentials")
}

func TestPasswordLogoutAllDisabledAndDeletion(t *testing.T) {
	db := authTestDB(t)
	service := New(authTestConfig(), db)
	ctx := context.Background()
	first := registerFixture(t, service)
	second, err := service.Login(ctx, LoginInput{Email: first.User.Email, Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.ChangePassword(ctx, first.User.ID, PasswordInput{CurrentPassword: "wrong-password-123", NewPassword: "another-password-123"}); err == nil {
		t.Fatal("changed password without reauthentication")
	}
	if err = service.ChangePassword(ctx, first.User.ID, PasswordInput{CurrentPassword: testPassword, NewPassword: "another-password-123"}); err != nil {
		t.Fatal(err)
	}
	for _, old := range []SessionTokens{first, second} {
		_, err = service.Authenticate(ctx, old.AccessToken)
		expectCode(t, err, "invalid_credentials")
		_, err = service.Refresh(ctx, old.RefreshToken)
		expectCode(t, err, "invalid_credentials")
	}
	_, err = service.Login(ctx, LoginInput{Email: first.User.Email, Password: testPassword})
	expectCode(t, err, "invalid_credentials")
	current, err := service.Login(ctx, LoginInput{Email: first.User.Email, Password: "another-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.LogoutAll(ctx, first.User.ID); err != nil {
		t.Fatal(err)
	}
	_, err = service.Authenticate(ctx, current.AccessToken)
	expectCode(t, err, "invalid_credentials")
	current, err = service.Login(ctx, LoginInput{Email: first.User.Email, Password: "another-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&User{}).Where("id = ?", first.User.ID).Update("disabled", true).Error; err != nil {
		t.Fatal(err)
	}
	_, err = service.Authenticate(ctx, current.AccessToken)
	expectCode(t, err, "invalid_credentials")
	_, err = service.Refresh(ctx, current.RefreshToken)
	expectCode(t, err, "invalid_credentials")
	_, err = service.Login(ctx, LoginInput{Email: first.User.Email, Password: "another-password-123"})
	expectCode(t, err, "invalid_credentials")
	if err = db.Model(&User{}).Where("id = ?", first.User.ID).Update("disabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err = service.DeleteAccount(ctx, first.User.ID, "wrong-password-123"); err == nil {
		t.Fatal("deleted account without reauthentication")
	}
	if err = service.DeleteAccount(ctx, first.User.ID, "another-password-123"); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = db.Model(&User{}).Where("id = ?", first.User.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("account survived deletion", count, err)
	}
}

func TestConcurrentRefreshReplayRevokesWinner(t *testing.T) {
	db := authTestDB(t)
	service := New(authTestConfig(), db)
	first := registerFixture(t, service)
	var wg sync.WaitGroup
	results := make(chan SessionTokens, 2)
	failures := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			result, err := service.Refresh(context.Background(), first.RefreshToken)
			if err != nil {
				failures <- err
			} else {
				results <- result
			}
		})
	}
	wg.Wait()
	if len(results) != 1 || len(failures) != 1 {
		t.Fatalf("expected one rotation and one replay rejection, got %d/%d", len(results), len(failures))
	}
	expectCode(t, <-failures, "invalid_credentials")
	winner := <-results
	_, err := service.Authenticate(context.Background(), winner.AccessToken)
	expectCode(t, err, "invalid_credentials")
}

func TestExpiredSessionAndSessionOwnership(t *testing.T) {
	db := authTestDB(t)
	cfg := authTestConfig()
	cfg.RefreshTTL = 2 * time.Second
	service := New(cfg, db)
	now := time.Now().UTC().Truncate(time.Second)
	service.now = func() time.Time { return now }
	first, second := registerFixture(t, service), registerFixture(t, service)
	var firstSession database.RefreshSession
	if err := db.Where("token_hash = ?", hashToken(first.RefreshToken)).First(&firstSession).Error; err != nil {
		t.Fatal(err)
	}
	wrongOwner, err := service.signSession(second.User, firstSession.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Authenticate(context.Background(), wrongOwner.AccessToken)
	expectCode(t, err, "invalid_credentials")
	now = now.Add(3 * time.Second)
	_, err = service.Authenticate(context.Background(), first.AccessToken)
	expectCode(t, err, "invalid_credentials")
	_, err = service.Refresh(context.Background(), first.RefreshToken)
	expectCode(t, err, "invalid_credentials")
}

func TestGoogleProvisioningAndAdminSeed(t *testing.T) {
	db := authTestDB(t)
	cfg := authTestConfig()
	cfg.AdminEmail, cfg.AdminPassword = "admin@example.test", testPassword
	service := New(cfg, db)
	ctx := context.Background()
	regular := registerFixture(t, service)
	_, err := service.GoogleAccount(ctx, "unlinked-google-subject", regular.User.Email, "Google name")
	expectCode(t, err, "account_link_required")
	var wg sync.WaitGroup
	accounts := make(chan User, 2)
	provisionErrors := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			user, err := service.GoogleAccount(ctx, "concurrent-google-subject", "google@example.test", "Google user")
			if err != nil {
				provisionErrors <- err
			} else {
				accounts <- user
			}
		})
	}
	wg.Wait()
	if len(provisionErrors) != 0 || len(accounts) != 2 {
		t.Fatal("concurrent Google provisioning failed", len(provisionErrors), len(accounts))
	}
	google1, google2 := <-accounts, <-accounts
	if google1.ID != google2.ID || google1.PasswordHash != "" {
		t.Fatal("Google identity duplicated or password generated")
	}
	if err = service.DeleteAccount(ctx, google1.ID, testPassword); err == nil {
		t.Fatal("Google-only account deleted without fresh provider authentication")
	}
	created, err := service.SeedAdmin(ctx)
	if err != nil || !created {
		t.Fatal(created, err)
	}
	service.cfg.AdminPassword = "replacement-password-123"
	created, err = service.SeedAdmin(ctx)
	if err != nil || created {
		t.Fatal("seed changed existing administrator", created, err)
	}
	admin, err := service.Login(ctx, LoginInput{Email: cfg.AdminEmail, Password: testPassword})
	if err != nil || admin.User.Role != "admin" {
		t.Fatal("admin password reset by repeated seed", err)
	}
	if err = service.ChangePassword(ctx, admin.User.ID, PasswordInput{CurrentPassword: testPassword, NewPassword: "admin-new-password-123"}); err != nil {
		t.Fatal("administrator cannot change own password", err)
	}
}

func TestJWTValidationAndInputBounds(t *testing.T) {
	cfg := authTestConfig()
	service := New(cfg, nil)
	if _, err := normalizeEmail("Name <person@example.test>"); err == nil {
		t.Fatal("accepted display-name email")
	}
	if validPassword(strings.Repeat("a", 73)) || validPassword("too-short") || validPassword(strings.Repeat(" ", 12)) {
		t.Fatal("unsafe password bounds")
	}
	now := time.Now().UTC().Truncate(time.Second)
	service.now = func() time.Time { return now }
	base := accessClaims{SessionID: uuid.NewString(), AuthVersion: 1, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: tokenIssuer, Subject: uuid.NewString(), Audience: jwt.ClaimStrings{tokenAudience}, ID: uuid.NewString(),
		IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
	}}
	for _, scenario := range []string{"expired", "wrong_issuer", "wrong_audience", "missing_expiry", "wrong_algorithm", "invalid_subject"} {
		t.Run(scenario, func(t *testing.T) {
			claims := base
			method := jwt.SigningMethodHS256
			switch scenario {
			case "expired":
				claims.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Second))
			case "wrong_issuer":
				claims.Issuer = "untrusted"
			case "wrong_audience":
				claims.Audience = jwt.ClaimStrings{"other"}
			case "missing_expiry":
				claims.ExpiresAt = nil
			case "wrong_algorithm":
				method = jwt.SigningMethodHS384
			case "invalid_subject":
				claims.Subject = "not-a-uuid"
			}
			raw, err := jwt.NewWithClaims(method, claims).SignedString([]byte(cfg.JWTSecret))
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Authenticate(context.Background(), raw)
			expectCode(t, err, "invalid_credentials")
		})
	}
	if actual := hashToken("sensitive-refresh-token"); len(actual) != 64 || strings.Contains(actual, "sensitive") {
		t.Fatal(fmt.Sprintf("unexpected token hash length %d", len(actual)))
	}
}
