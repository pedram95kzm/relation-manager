package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"relations/internal/auth"
	"relations/internal/config"
	"relations/internal/platform"
)

type stubGoogleService struct {
	start               auth.GoogleStart
	result              auth.SessionTokens
	err                 error
	state, cookie, code string
	providerError       string
	starts              int
}

func (s *stubGoogleService) BeginGoogle(context.Context) (auth.GoogleStart, error) {
	s.starts++
	return s.start, s.err
}

func (s *stubGoogleService) CompleteGoogle(_ context.Context, state, cookie, code, providerError string) (auth.SessionTokens, error) {
	s.state, s.cookie, s.code, s.providerError = state, cookie, code, providerError
	return s.result, s.err
}

func googleTestRouter(service *stubGoogleService) (*gin.Engine, config.Config) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	cfg := config.Config{PublicURL: "https://app.example", CookieSecure: true, RefreshTTL: 24 * time.Hour}
	registerGoogleRoutes(router, service, cfg)
	return router, cfg
}

func findGoogleCookie(t *testing.T, response *http.Response, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}

func TestGoogleStartHTTPStateCookieAndRateLimit(t *testing.T) {
	service := &stubGoogleService{start: auth.GoogleStart{State: "random-browser-state", AuthorizationURL: "https://accounts.google.com/auth?state=random-browser-state"}}
	router, _ := googleTestRouter(service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google?redirect=https://attacker.example", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != service.start.AuthorizationURL || response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("OAuth start did not redirect to the provider", response.Code, response.Header())
	}
	cookie := findGoogleCookie(t, response.Result(), googleStateCookie)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/api/v1/auth/google" || cookie.MaxAge != 300 || cookie.Domain != "" {
		t.Fatal("unsafe OAuth state cookie", cookie)
	}
	for range 9 {
		router.ServeHTTP(httptest.NewRecorder(), request)
	}
	throttled := httptest.NewRecorder()
	router.ServeHTTP(throttled, request)
	if throttled.Code != http.StatusTooManyRequests || service.starts != 10 {
		t.Fatal("start requests must be rate-limited before DB work", throttled.Code, service.starts)
	}
}

func TestGoogleCallbackHTTPFixedRedirectAndRefreshCookie(t *testing.T) {
	service := &stubGoogleService{result: auth.SessionTokens{AccessToken: "private-access-token", RefreshToken: "private-refresh-token"}}
	router, cfg := googleTestRouter(service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?state=bound-state&code=provider-code&redirect=https://attacker.example", nil)
	request.AddCookie(&http.Cookie{Name: googleStateCookie, Value: "bound-state"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if service.state != "bound-state" || service.cookie != "bound-state" || service.code != "provider-code" || service.providerError != "" {
		t.Fatal("callback did not pass bound values to service")
	}
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "https://app.example/auth/callback" ||
		strings.Contains(response.Body.String(), "private-") || strings.Contains(response.Header().Get("Location"), "token") {
		t.Fatal("callback must use fixed frontend URL without credentials", response.Code, response.Header(), response.Body.String())
	}
	state := findGoogleCookie(t, response.Result(), googleStateCookie)
	if state.MaxAge != -1 || !state.HttpOnly || state.Value != "" {
		t.Fatal("callback must clear the single-flow state cookie", state)
	}
	refresh := findGoogleCookie(t, response.Result(), refreshCookieName(cfg))
	if refresh.Value != "private-refresh-token" || !refresh.HttpOnly || !refresh.Secure || refresh.SameSite != http.SameSiteLaxMode || refresh.Path != "/api/v1/auth" {
		t.Fatal("callback refresh-cookie configuration mismatch", refresh)
	}
}

func TestGoogleCallbackHTTPFailureUsesSafeErrorCodes(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{platform.Err(409, "account_link_required", "do not leak email"), "account_link_required"},
		{platform.Err(401, "oauth_cancelled", "safe"), "oauth_cancelled"},
		{platform.Err(401, "oauth_state_invalid", "safe"), "oauth_state_invalid"},
		{platform.Err(401, "oauth_identity_invalid", "safe"), "oauth_identity_invalid"},
		{platform.Err(503, "google_unavailable", "safe"), "google_unavailable"},
		{platform.Err(500, "unsafe-code&token=provider-secret", "unsafe"), "oauth_failed"},
		{errors.New("provider-secret"), "oauth_failed"},
	} {
		t.Run(test.code, func(t *testing.T) {
			service := &stubGoogleService{err: test.err}
			router, _ := googleTestRouter(service)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?state=state&error=access_denied&error_description=provider-secret", nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "https://app.example/login?error="+test.code || strings.Contains(response.Body.String(), "secret") {
				t.Fatal("untrusted OAuth details escaped error allowlist", response.Header(), response.Body.String())
			}
			findGoogleCookie(t, response.Result(), googleStateCookie)
			for _, cookie := range response.Result().Cookies() {
				if cookie.Name == refreshCookieName(config.Config{CookieSecure: true}) {
					t.Fatal("failed login set refresh cookie")
				}
			}
		})
	}
}

func TestGoogleCallbackRejectsAmbiguousState(t *testing.T) {
	service := &stubGoogleService{err: platform.Err(401, "oauth_state_invalid", "safe")}
	router, _ := googleTestRouter(service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?state=first&state=second&code=code", nil)
	router.ServeHTTP(httptest.NewRecorder(), request)
	if service.state != "" {
		t.Fatal("duplicate state must not be interpreted as either value")
	}
}
