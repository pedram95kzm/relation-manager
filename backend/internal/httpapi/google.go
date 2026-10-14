package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"relations/internal/auth"
	"relations/internal/config"
	"relations/internal/platform"
)

const googleStateCookie = "relations_oauth_state"

type googleAuthService interface {
	BeginGoogle(context.Context) (auth.GoogleStart, error)
	CompleteGoogle(context.Context, string, string, string, string) (auth.SessionTokens, error)
}

func RegisterGoogleRoutes(r *gin.Engine, service *auth.Service, cfg config.Config) {
	registerGoogleRoutes(r, service, cfg)
}

func registerGoogleRoutes(r *gin.Engine, service googleAuthService, cfg config.Config) {
	startLimit := platform.NewLimiter(10, 15*time.Minute)
	r.GET("/api/v1/auth/google", startLimit.Middleware(), func(c *gin.Context) {
		start, err := service.BeginGoogle(c.Request.Context())
		if err != nil {
			c.Redirect(http.StatusSeeOther, googleErrorRedirect(cfg, err))
			return
		}
		setGoogleStateCookie(c, cfg, start.State, int(auth.GoogleStateTTL.Seconds()))
		c.Header("Referrer-Policy", "no-referrer")
		c.Redirect(http.StatusSeeOther, start.AuthorizationURL)
	})
	r.GET("/api/v1/auth/google/callback", func(c *gin.Context) {
		cookieState, _ := c.Cookie(googleStateCookie)
		setGoogleStateCookie(c, cfg, "", -1)
		c.Header("Referrer-Policy", "no-referrer")
		query := c.Request.URL.Query()
		result, err := service.CompleteGoogle(c.Request.Context(), singleGoogleQuery(query, "state"), cookieState, singleGoogleQuery(query, "code"), singleGoogleQuery(query, "error"))
		if err != nil {
			c.Redirect(http.StatusSeeOther, googleErrorRedirect(cfg, err))
			return
		}
		setRefreshCookie(c, cfg, result.RefreshToken)
		// The SPA retrieves its short-lived access token through /auth/refresh.
		c.Redirect(http.StatusSeeOther, cfg.PublicURL+"/auth/callback")
	})
}

func singleGoogleQuery(query url.Values, key string) string {
	values := query[key]
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

func setGoogleStateCookie(c *gin.Context, cfg config.Config, value string, maxAge int) {
	cookie := &http.Cookie{Name: googleStateCookie, Value: value, Path: "/api/v1/auth/google",
		MaxAge: maxAge, HttpOnly: true, Secure: cfg.CookieSecure, SameSite: http.SameSiteLaxMode}
	if maxAge < 0 {
		cookie.Expires = time.Unix(1, 0)
	}
	http.SetCookie(c.Writer, cookie)
}

func googleErrorRedirect(cfg config.Config, err error) string {
	code := "oauth_failed"
	var appErr *platform.Error
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case "google_unavailable", "oauth_state_invalid", "oauth_identity_invalid", "oauth_cancelled", "account_link_required":
			code = appErr.Code
		}
	}
	return cfg.PublicURL + "/login?error=" + code
}
