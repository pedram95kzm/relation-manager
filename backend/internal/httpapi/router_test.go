package httpapi

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"relations/internal/config"
)

func TestRecoveryNeverLogsCredentials(t *testing.T) {
	var logs bytes.Buffer
	r := New(config.Config{AllowedOrigins: []string{"http://localhost:5173"}}, nil, slog.New(slog.NewJSONHandler(&logs, nil)))
	r.GET("/panic", func(c *gin.Context) { panic("sensitive-private-answer") })
	req := httptest.NewRequest("GET", "/panic", nil)
	req.Header.Set("Cookie", "refresh=private-cookie-value")
	req.Header.Set("Authorization", "Bearer private-access-value")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 500 || !strings.Contains(w.Body.String(), "internal_error") {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, secret := range []string{"private-cookie-value", "private-access-value", "sensitive-private-answer"} {
		if strings.Contains(logs.String(), secret) || strings.Contains(w.Body.String(), secret) {
			t.Fatal("secret escaped sanitized recovery")
		}
	}
}
func TestOriginsAndHeaders(t *testing.T) {
	r := New(config.Config{AllowedOrigins: []string{"https://app.example.org"}}, nil, slog.Default())
	for _, tc := range []struct {
		origin string
		want   int
	}{{"https://evil.example.org", 403}, {"https://app.example.org", 200}} {
		req := httptest.NewRequest("GET", "/api/v1/health/live", nil)
		req.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.want || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal(w.Code, w.Header())
		}
	}
}
