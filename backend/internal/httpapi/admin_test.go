package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"relations/internal/auth"
)

func TestAdminHTTPRoleOriginStrictFieldsAndSecretRedaction(t *testing.T) {
	db := httpAuthDB(t)
	cfg := httpAuthConfig()
	cfg.EncryptionKey = []byte(strings.Repeat("k", 32))
	cfg.GlobalDailyLimit = 100
	cfg.UserDailyLimit = 10
	accounts := auth.New(cfg, db)
	ctx := context.Background()
	ordinary, err := accounts.Register(ctx, auth.RegisterInput{Email: "ordinary@example.test", Name: "Ordinary account", Password: "strong-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	administrator, err := accounts.Register(ctx, auth.RegisterInput{Email: "administrator@example.test", Name: "Administrator", Password: "strong-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&administrator.User).Update("role", "admin").Error; err != nil {
		t.Fatal(err)
	}
	administrator, err = accounts.Login(ctx, auth.LoginInput{Email: "administrator@example.test", Password: "strong-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	router := New(cfg, db, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if response := authRequest(router, "GET", "/api/v1/admin/users", "", cfg.PublicURL, "", nil); response.Code != 401 {
		t.Fatal(response.Code, response.Body.String())
	}
	if response := authRequest(router, "GET", "/api/v1/admin/users", "", cfg.PublicURL, ordinary.AccessToken, nil); response.Code != 403 {
		t.Fatal(response.Code, response.Body.String())
	}
	if response := authRequest(router, "PUT", "/api/v1/admin/limits", `{"global_daily":20,"user_daily":2}`, "", administrator.AccessToken, nil); response.Code != 403 {
		t.Fatal(response.Code, response.Body.String())
	}
	if response := authRequest(router, "PUT", "/api/v1/admin/limits", `{"global_daily":20,"user_daily":2,"unexpected":1}`, cfg.PublicURL, administrator.AccessToken, nil); response.Code != 400 {
		t.Fatal(response.Code, response.Body.String())
	}
	secret := "sensitive-key-for-provider-123"
	body := `{"provider":"openai","model":"sample-model","api_key":"` + secret + `","max_tokens":4096,"timeout_seconds":60,"retries":1,"active":true}`
	if response := authRequest(router, "PUT", "/api/v1/admin/provider", body, cfg.PublicURL, administrator.AccessToken, nil); response.Code != 200 || strings.Contains(response.Body.String(), secret) {
		t.Fatal(response.Code, response.Body.String())
	}
	for _, path := range []string{"/api/v1/admin/provider", "/api/v1/admin/audit", "/api/v1/admin/users", "/api/v1/admin/analytics?days=7"} {
		response := authRequest(router, "GET", path, "", cfg.PublicURL, administrator.AccessToken, nil)
		if response.Code != 200 || strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), "password_hash") || strings.Contains(response.Body.String(), "encrypted_key") {
			t.Fatal(path, response.Code, response.Body.String())
		}
		var data map[string]any
		if json.Unmarshal(response.Body.Bytes(), &data) != nil {
			t.Fatal("invalid JSON")
		}
	}
	if response := authRequest(router, "PATCH", "/api/v1/admin/users/"+ordinary.User.ID, `{"disabled":true}`, cfg.PublicURL, administrator.AccessToken, nil); response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	if response := authRequest(router, "GET", "/api/v1/users/me", "", cfg.PublicURL, ordinary.AccessToken, nil); response.Code != 401 {
		t.Fatal("disabled token authenticated", response.Code, response.Body.String())
	}
	if response := authRequest(router, "DELETE", "/api/v1/admin/users/"+ordinary.User.ID, `{"current_password":"strong-password-123","confirm_email":"ordinary@example.test"}`, cfg.PublicURL, administrator.AccessToken, nil); response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
}

func TestAdminLocalPreviewHasRoleOriginAndStrictInputChecks(t *testing.T) {
	db := httpAuthDB(t)
	cfg := httpAuthConfig()
	accounts := auth.New(cfg, db)
	ctx := context.Background()
	user, err := accounts.Register(ctx, auth.RegisterInput{Email: "preview@example.test", Name: "Preview administrator", Password: "strong-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	ordinaryToken := user.AccessToken
	router := New(cfg, db, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if response := authRequest(router, "POST", "/api/v1/admin/provider/preview", `{"module":"self","locale":"en"}`, cfg.PublicURL, ordinaryToken, nil); response.Code != 403 {
		t.Fatal("ordinary account reached admin preview", response.Code, response.Body.String())
	}
	if err := db.Model(&user.User).Update("role", "admin").Error; err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		body, origin string
		status       int
	}{
		{`{"module":"self","locale":"en"}`, "", 403},
		{`{"module":"self","locale":"en","api_key":"unexpected"}`, cfg.PublicURL, 400},
		{`{"module":"self","answers":null}`, cfg.PublicURL, 400},
		{`{"module":"conflict","locale":"en"}`, cfg.PublicURL, 200},
	} {
		response := authRequest(router, "POST", "/api/v1/admin/provider/preview", test.body, test.origin, ordinaryToken, nil)
		if response.Code != test.status {
			t.Fatal(response.Code, response.Body.String())
		}
		if test.status == 200 && (!strings.Contains(response.Body.String(), `"kind":"example"`) || !strings.Contains(response.Body.String(), `"external_request":false`) || !strings.Contains(response.Body.String(), `"level":"caution"`)) {
			t.Fatal("preview not local or safety-sensitive", response.Body.String())
		}
	}
}
