package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestConfiguration(t *testing.T) {
	valid := map[string]string{"DATABASE_URL": "postgres://test@localhost/relationstest?sslmode=disable", "JWT_SECRET": strings.Repeat("s", 48), "AI_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))}
	get := func(k string) string { return valid[k] }
	c, e := From(get)
	if e != nil || c.MinAge != 18 || c.AccessTTL.Minutes() != 15 {
		t.Fatal(c, e)
	}
	for _, tc := range []struct{ k, v string }{{"JWT_SECRET", "short"}, {"AI_ENCRYPTION_KEY", "broken"}, {"MIN_AGE", "12"}, {"CORS_ORIGINS", "*"}, {"AI_RETRIES", "100"}, {"APP_ENV", "production"}, {"TRUSTED_PROXIES", "0.0.0.0/0"}, {"TRUSTED_PROXIES", "not-a-proxy"}} {
		old := valid[tc.k]
		valid[tc.k] = tc.v
		if _, err := From(get); err == nil {
			t.Errorf("accepted unsafe %s", tc.k)
		}
		valid[tc.k] = old
	}
}

func TestOAuthCallbackConfiguration(t *testing.T) {
	values := map[string]string{"DATABASE_URL": "postgres://test@localhost/relationstest?sslmode=disable", "JWT_SECRET": strings.Repeat("s", 48), "AI_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))), "GOOGLE_CLIENT_ID": "configured-client", "GOOGLE_CLIENT_SECRET": "configured-secret", "GOOGLE_CALLBACK_URL": "http://localhost:5173/api/v1/auth/google/callback"}
	get := func(k string) string { return values[k] }
	if _, e := From(get); e != nil {
		t.Fatal(e)
	}
	for _, callback := range []string{"http://localhost:5173/incorrect", "http://other.example.org/api/v1/auth/google/callback", "http://localhost:5173/api/v1/auth/google/callback?next=bad"} {
		values["GOOGLE_CALLBACK_URL"] = callback
		if _, e := From(get); e == nil {
			t.Fatal("accepted invalid callback", callback)
		}
	}
}

func TestProductionTransportSafety(t *testing.T) {
	values := map[string]string{"APP_ENV": "production", "PUBLIC_URL": "https://relations.example.org", "COOKIE_SECURE": "true", "DATABASE_URL": "postgres://user@db.example.org/relations?sslmode=verify-full", "JWT_SECRET": strings.Repeat("s", 48), "AI_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))}
	get := func(k string) string { return values[k] }
	if _, e := From(get); e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"", "prefer", "require", "disable"} {
		values["DATABASE_URL"] = "postgres://user@db.example.org/relations?sslmode=" + mode
		if _, e := From(get); e == nil {
			t.Errorf("accepted unverified remote TLS %s", mode)
		}
	}
	values["DATABASE_URL"] = "postgres://user@database/relations?sslmode=disable"
	values["TRUSTED_PROXIES"] = "172.30.0.0/24"
	if _, e := From(get); e != nil {
		t.Fatal("internal network exception", e)
	}
}

func TestDefaultAccountConfiguration(t *testing.T) {
	values := map[string]string{"DATABASE_URL": "postgres://test@localhost/relationstest?sslmode=disable", "JWT_SECRET": strings.Repeat("s", 48), "AI_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))}
	get := func(k string) string { return values[k] }
	c, err := From(get)
	if err != nil || !c.SeedDefaultAccounts || c.AdminEmail != DefaultAdminEmail || c.AdminPassword != DefaultAdminPassword || c.UserEmail != DefaultUserEmail || c.UserPassword != DefaultUserPassword {
		t.Fatal("local default accounts", err)
	}
	values["SEED_DEFAULT_ACCOUNTS"] = "false"
	c, err = From(get)
	if err != nil || c.SeedDefaultAccounts || c.AdminPassword != "" {
		t.Fatal("disabled bootstrap", err)
	}
	values["SEED_DEFAULT_ACCOUNTS"] = "invalid"
	if _, err = From(get); err == nil {
		t.Fatal("accepted invalid boolean")
	}
	values["APP_ENV"], values["PUBLIC_URL"], values["COOKIE_SECURE"] = "production", "https://relations.example.org", "true"
	values["DATABASE_URL"] = "postgres://test@database/relationstest?sslmode=disable"
	delete(values, "SEED_DEFAULT_ACCOUNTS")
	c, err = From(get)
	if err != nil || c.SeedDefaultAccounts || c.UserPassword != "" {
		t.Fatal("production defaults enabled", err)
	}
	values["SEED_DEFAULT_ACCOUNTS"] = "true"
	if _, err = From(get); err == nil {
		t.Fatal("accepted production bootstrap without private credentials")
	}
	values["ADMIN_EMAIL"], values["USER_EMAIL"] = "owner@example.org", "member@example.org"
	values["ADMIN_PASSWORD"], values["USER_PASSWORD"] = "PrivateAdminPassword2026!", "PrivateMemberPassword2026!"
	if _, err = From(get); err != nil {
		t.Fatal("private production initialization", err)
	}
	for _, key := range []string{"ADMIN_PASSWORD", "USER_PASSWORD"} {
		old := values[key]
		for _, password := range []string{DefaultAdminPassword, DefaultUserPassword, "short"} {
			values[key] = password
			if _, err = From(get); err == nil {
				t.Fatal("accepted public/invalid production password", key)
			}
		}
		values[key] = old
	}
}
