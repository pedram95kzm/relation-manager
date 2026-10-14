package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Public credentials are for local development only and never reset accounts.
const DefaultAdminEmail = "admin@relations.local"
const DefaultAdminPassword = "RelationsAdmin2026!"
const DefaultUserEmail = "user@relations.local"
const DefaultUserPassword = "RelationsUser2026!"

type Config struct {
	Environment, HTTPAddr, DatabaseURL, PublicURL, JWTSecret         string
	AccessTTL, RefreshTTL, AITimeout                                 time.Duration
	CookieSecure                                                     bool
	AllowedOrigins                                                   []string
	TrustedProxies                                                   []string
	EncryptionKey                                                    []byte
	GoogleClientID, GoogleClientSecret, GoogleCallbackURL            string
	AdminEmail, AdminPassword                                        string
	UserEmail, UserPassword                                          string
	SeedDefaultAccounts                                              bool
	MinAge, GlobalDailyLimit, UserDailyLimit, AIRetries, AIMaxTokens int
	LogLevel                                                         string
}

func Load() (Config, error) { return From(os.Getenv) }

func From(get func(string) string) (Config, error) {
	str := func(k, d string) string {
		if v := get(k); v != "" {
			return v
		}
		return d
	}
	c := Config{Environment: str("APP_ENV", "development"), HTTPAddr: str("HTTP_ADDR", ":8080"), DatabaseURL: get("DATABASE_URL"), PublicURL: str("PUBLIC_URL", "http://localhost:5173"), JWTSecret: get("JWT_SECRET"), GoogleClientID: get("GOOGLE_CLIENT_ID"), GoogleClientSecret: get("GOOGLE_CLIENT_SECRET"), GoogleCallbackURL: get("GOOGLE_CALLBACK_URL"), AdminEmail: get("ADMIN_EMAIL"), AdminPassword: get("ADMIN_PASSWORD"), LogLevel: str("LOG_LEVEL", "info")}
	if c.Environment != "development" && c.Environment != "test" && c.Environment != "production" {
		return c, errors.New("APP_ENV must be development, test or production")
	}
	var err error
	c.SeedDefaultAccounts, err = strconv.ParseBool(str("SEED_DEFAULT_ACCOUNTS", strconv.FormatBool(c.Environment != "production")))
	if err != nil {
		return c, errors.New("SEED_DEFAULT_ACCOUNTS must be true or false")
	}
	c.UserEmail, c.UserPassword = get("USER_EMAIL"), get("USER_PASSWORD")
	if c.Environment == "production" && (c.AdminPassword == DefaultAdminPassword || c.AdminPassword == DefaultUserPassword || c.UserPassword == DefaultAdminPassword || c.UserPassword == DefaultUserPassword) {
		return c, errors.New("production configuration rejects public demonstration passwords")
	}
	if c.SeedDefaultAccounts {
		if c.Environment == "production" {
			for _, entry := range []struct{ key, value string }{{"ADMIN_EMAIL", c.AdminEmail}, {"ADMIN_PASSWORD", c.AdminPassword}, {"USER_EMAIL", c.UserEmail}, {"USER_PASSWORD", c.UserPassword}} {
				if entry.value == "" {
					return c, fmt.Errorf("%s is required for production account initialization", entry.key)
				}
			}
			for _, password := range []string{c.AdminPassword, c.UserPassword} {
				if password == DefaultAdminPassword || password == DefaultUserPassword || len(password) < 12 || len(password) > 72 || !utf8.ValidString(password) || strings.TrimSpace(password) == "" {
					return c, errors.New("production account initialization requires private passwords of 12 to 72 UTF-8 bytes")
				}
			}
		} else {
			c.AdminEmail, c.AdminPassword = str("ADMIN_EMAIL", DefaultAdminEmail), str("ADMIN_PASSWORD", DefaultAdminPassword)
			c.UserEmail, c.UserPassword = str("USER_EMAIL", DefaultUserEmail), str("USER_PASSWORD", DefaultUserPassword)
		}
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	db, err := url.Parse(c.DatabaseURL)
	if err != nil || (db.Scheme != "postgres" && db.Scheme != "postgresql") || db.Host == "" {
		return c, errors.New("DATABASE_URL must be a PostgreSQL URL")
	}
	if len(c.JWTSecret) < 32 || strings.Contains(strings.ToLower(c.JWTSecret), "change") || strings.Contains(strings.ToLower(c.JWTSecret), "example") {
		return c, errors.New("JWT_SECRET must be a generated secret of at least 32 characters")
	}
	c.EncryptionKey, err = base64.StdEncoding.DecodeString(get("AI_ENCRYPTION_KEY"))
	if err != nil || len(c.EncryptionKey) != 32 {
		return c, errors.New("AI_ENCRYPTION_KEY must be base64 encoding of exactly 32 random bytes")
	}
	public, err := url.Parse(c.PublicURL)
	if err != nil || public.Host == "" || (public.Scheme != "http" && public.Scheme != "https") || public.User != nil || public.RawQuery != "" || public.Fragment != "" || (public.Path != "" && public.Path != "/") {
		return c, errors.New("PUBLIC_URL must be an HTTP(S) origin")
	}
	c.PublicURL = strings.TrimRight(c.PublicURL, "/")
	c.CookieSecure, err = strconv.ParseBool(str("COOKIE_SECURE", "false"))
	if err != nil {
		return c, errors.New("COOKIE_SECURE must be true or false")
	}
	if c.Environment == "production" && (public.Scheme != "https" || !c.CookieSecure) {
		return c, errors.New("production requires HTTPS PUBLIC_URL and COOKIE_SECURE=true")
	}
	if c.Environment == "production" && db.Query().Get("sslmode") != "verify-full" && db.Hostname() != "database" {
		return c, errors.New("production remote database requires TLS (sslmode=verify-full)")
	}
	c.AllowedOrigins = strings.Split(str("CORS_ORIGINS", c.PublicURL), ",")
	for i, origin := range c.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		u, e := url.Parse(origin)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return c, errors.New("CORS_ORIGINS must contain exact HTTP(S) origins")
		}
		c.AllowedOrigins[i] = origin
		if c.Environment == "production" && u.Scheme != "https" {
			return c, errors.New("production CORS origins require HTTPS")
		}
	}
	if raw := get("TRUSTED_PROXIES"); raw != "" {
		for _, value := range strings.Split(raw, ",") {
			value = strings.TrimSpace(value)
			if net.ParseIP(value) == nil {
				_, network, err := net.ParseCIDR(value)
				if err != nil {
					return c, errors.New("TRUSTED_PROXIES must contain IP addresses or CIDRs")
				}
				bits, _ := network.Mask.Size()
				if bits == 0 {
					return c, errors.New("TRUSTED_PROXIES cannot trust every address")
				}
			}
			c.TrustedProxies = append(c.TrustedProxies, value)
		}
	}
	ints := []struct {
		k           string
		d, min, max int
		p           *int
	}{{"MIN_AGE", 18, 18, 120, &c.MinAge}, {"GLOBAL_DAILY_LIMIT", 1000, 1, 1000000, &c.GlobalDailyLimit}, {"USER_DAILY_LIMIT", 10, 1, 10000, &c.UserDailyLimit}, {"AI_RETRIES", 1, 0, 3, &c.AIRetries}, {"AI_MAX_TOKENS", 4096, 512, 16000, &c.AIMaxTokens}}
	for _, v := range ints {
		n, e := strconv.Atoi(str(v.k, strconv.Itoa(v.d)))
		if e != nil || n < v.min || n > v.max {
			return c, fmt.Errorf("%s must be between %d and %d", v.k, v.min, v.max)
		}
		*v.p = n
	}
	for _, v := range []struct {
		k, d     string
		min, max time.Duration
		p        *time.Duration
	}{{"ACCESS_TOKEN_TTL", "15m", time.Minute, time.Hour, &c.AccessTTL}, {"REFRESH_TOKEN_TTL", "720h", time.Hour, 90 * 24 * time.Hour, &c.RefreshTTL}, {"AI_TIMEOUT", "60s", time.Second, 120 * time.Second, &c.AITimeout}} {
		d, e := time.ParseDuration(str(v.k, v.d))
		if e != nil || d < v.min || d > v.max {
			return c, fmt.Errorf("invalid %s", v.k)
		}
		*v.p = d
	}
	if c.GoogleClientID != "" || c.GoogleClientSecret != "" || c.GoogleCallbackURL != "" {
		u, e := url.Parse(c.GoogleCallbackURL)
		if c.GoogleClientID == "" || c.GoogleClientSecret == "" || e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
			return c, errors.New("all Google OAuth fields must be configured with a valid callback URL")
		}
		if u.Path != "/api/v1/auth/google/callback" || u.Hostname() != public.Hostname() {
			return c, errors.New("GOOGLE_CALLBACK_URL must use the public application hostname and /api/v1/auth/google/callback")
		}
		if c.Environment == "production" && u.Scheme != "https" {
			return c, errors.New("production Google callback requires HTTPS")
		}
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return c, errors.New("LOG_LEVEL must be debug, info, warn or error")
	}
	return c, nil
}
