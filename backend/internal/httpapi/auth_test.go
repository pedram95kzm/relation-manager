package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"relations/internal/auth"
	"relations/internal/config"
	"relations/internal/database"
)

func httpAuthConfig() config.Config {
	return config.Config{Environment: "test", JWTSecret: strings.Repeat("s", 48), PublicURL: "https://app.example.test", AllowedOrigins: []string{"https://app.example.test"}, CookieSecure: true, MinAge: 18, AccessTTL: 15 * time.Minute, RefreshTTL: 30 * 24 * time.Hour}
}

func httpAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for isolated PostgreSQL HTTP integration test")
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
	schema := "test_http_auth_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = tx.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	if err = tx.Exec(`SET LOCAL search_path TO "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, tx); err != nil {
		t.Fatal(err)
	}
	return tx
}

func authRequest(router http.Handler, method, path, body, origin, access string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if access != "" {
		request.Header.Set("Authorization", "Bearer "+access)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestAuthHTTPProfileAuthorizationOwnershipAndCookies(t *testing.T) {
	db := httpAuthDB(t)
	cfg := httpAuthConfig()
	router := New(cfg, db, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	accounts := auth.New(cfg, db)
	router.GET("/api/v1/test/profile", accounts.Middleware(), accounts.RequireProfile(), func(c *gin.Context) { c.Status(200) })
	router.GET("/api/v1/test/admin", accounts.Middleware(), accounts.RequireAdmin(), func(c *gin.Context) { c.Status(200) })
	response := authRequest(router, "POST", "/api/v1/auth/register", `{"email":"http@example.test","name":"آزمایش","password":"strong-password-123","role":"admin"}`, cfg.PublicURL, "", nil)
	if response.Code != 400 {
		t.Fatal("accepted privileged registration field", response.Code, response.Body.String())
	}
	response = authRequest(router, "POST", "/api/v1/auth/register", `{"email":"http@example.test","name":"آزمایش","password":"strong-password-123"}`, cfg.PublicURL, "", nil)
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body.String())
	}
	var envelope struct {
		Data auth.SessionTokens `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	var refresh *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == refreshCookieName(cfg) {
			refresh = cookie
		}
	}
	if refresh == nil || !refresh.HttpOnly || !refresh.Secure || refresh.SameSite != http.SameSiteLaxMode || refresh.Path != "/api/v1/auth" || refresh.Domain != "" {
		t.Fatal("unsafe refresh cookie", refresh)
	}
	if strings.Contains(response.Body.String(), refresh.Value) || strings.Contains(response.Body.String(), "password_hash") || strings.Contains(response.Body.String(), "refresh_token") {
		t.Fatal("response exposed credentials")
	}
	access := envelope.Data.AccessToken
	response = authRequest(router, "GET", "/api/v1/test/profile", "", "", access, nil)
	if response.Code != 403 {
		t.Fatal("incomplete profile entered analysis workflow", response.Code)
	}
	response = authRequest(router, "PATCH", "/api/v1/users/me", `{"name":"نام","age":25,"gender":"other","id":"`+uuid.NewString()+`"}`, cfg.PublicURL, access, nil)
	if response.Code != 400 {
		t.Fatal("accepted another owner's identifier", response.Code)
	}
	response = authRequest(router, "PATCH", "/api/v1/users/me", `{"name":"نام","age":25,"gender":"other"}`, cfg.PublicURL, access, nil)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	response = authRequest(router, "GET", "/api/v1/test/profile", "", "", access, nil)
	if response.Code != 200 {
		t.Fatal("completed profile denied", response.Code)
	}
	response = authRequest(router, "GET", "/api/v1/users/"+uuid.NewString(), "", "", access, nil)
	if response.Code != 404 {
		t.Fatal("arbitrary account exposed", response.Code)
	}
	response = authRequest(router, "GET", "/api/v1/test/admin", "", "", access, nil)
	if response.Code != 403 {
		t.Fatal("regular user accessed admin", response.Code)
	}
	if err := db.Model(&database.User{}).Where("id = ?", envelope.Data.User.ID).Update("role", "admin").Error; err != nil {
		t.Fatal(err)
	}
	response = authRequest(router, "GET", "/api/v1/test/admin", "", "", access, nil)
	if response.Code != 200 {
		t.Fatal("current database role not respected", response.Code)
	}
	response = authRequest(router, "POST", "/api/v1/auth/logout", "", cfg.PublicURL, "", refresh)
	if response.Code != 200 || len(response.Result().Cookies()) == 0 || response.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout failed to expire cookie", response.Code)
	}
	response = authRequest(router, "GET", "/api/v1/users/me", "", "", access, nil)
	if response.Code != 401 {
		t.Fatal("logout did not revoke access session", response.Code)
	}
}

func TestAuthHTTPOriginAndRateLimits(t *testing.T) {
	cfg := httpAuthConfig()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAuthRoutes(router, auth.New(cfg, nil), cfg)
	for _, origin := range []string{"", "https://attacker.example.test", "null"} {
		response := authRequest(router, "POST", "/api/v1/auth/refresh", "", origin, "", nil)
		if response.Code != 403 {
			t.Fatal("cookie mutation accepted untrusted/missing origin", origin, response.Code)
		}
	}
	for i := range 11 {
		response := authRequest(router, "POST", "/api/v1/auth/login", `{"unexpected":true}`, cfg.PublicURL, "", nil)
		if i < 10 && response.Code != 400 {
			t.Fatal(response.Code)
		}
		if i == 10 && (response.Code != 429 || response.Header().Get("Retry-After") == "") {
			t.Fatal("login rate limit not enforced", response.Code)
		}
	}
}

func TestAuthHTTPOversizedBodiesAreRejectedBeforeHandlers(t *testing.T) {
	cfg := httpAuthConfig()
	router := New(cfg, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := authRequest(router, "POST", "/api/v1/auth/logout", strings.Repeat("x", 65537), cfg.PublicURL, "", nil)
	if response.Code != 413 || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("unbounded or unhardened oversized response", response.Code)
	}
}
