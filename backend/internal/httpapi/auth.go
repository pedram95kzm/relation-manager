package httpapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"relations/internal/auth"
	"relations/internal/config"
	"relations/internal/platform"
)

func refreshCookieName(cfg config.Config) string {
	if cfg.CookieSecure {
		return "__Secure-rm_refresh"
	}
	return "rm_refresh"
}

func setRefreshCookie(c *gin.Context, cfg config.Config, raw string) {
	http.SetCookie(c.Writer, &http.Cookie{Name: refreshCookieName(cfg), Value: raw, Path: "/api/v1/auth", HttpOnly: true, Secure: cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode, MaxAge: int(cfg.RefreshTTL.Seconds()), Expires: time.Now().UTC().Add(cfg.RefreshTTL)})
}

func clearRefreshCookie(c *gin.Context, cfg config.Config) {
	http.SetCookie(c.Writer, &http.Cookie{Name: refreshCookieName(cfg), Value: "", Path: "/api/v1/auth", HttpOnly: true, Secure: cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0).UTC()})
}

// Cookie mutations require an exact trusted Origin, including native clients.
// Accepting absent Origin would bypass this CSRF boundary on browser requests.
func trustedOrigin(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		for _, allowed := range cfg.AllowedOrigins {
			if origin != "" && origin == allowed {
				c.Next()
				return
			}
		}
		platform.Fail(c, platform.Err(403, "origin_required", "A trusted request Origin is required."))
	}
}

func RegisterAuthRoutes(r *gin.Engine, service *auth.Service, cfg config.Config) {
	loginLimit := platform.NewLimiter(10, time.Minute)
	registerLimit := platform.NewLimiter(5, time.Hour)
	refreshLimit := platform.NewLimiter(60, time.Minute)
	accountLimit := platform.NewLimiter(10, time.Minute)
	r.GET("/api/v1/auth/config", func(c *gin.Context) {
		platform.OK(c, http.StatusOK, gin.H{"google_enabled": service.GoogleEnabled(), "minimum_age": cfg.MinAge})
	})
	r.POST("/api/v1/auth/register", trustedOrigin(cfg), registerLimit.Middleware(), func(c *gin.Context) {
		var input auth.RegisterInput
		if !platform.Decode(c, &input) {
			return
		}
		result, err := service.Register(c.Request.Context(), input)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		setRefreshCookie(c, cfg, result.RefreshToken)
		platform.OK(c, http.StatusCreated, result)
	})
	r.POST("/api/v1/auth/login", trustedOrigin(cfg), loginLimit.Middleware(), func(c *gin.Context) {
		var input auth.LoginInput
		if !platform.Decode(c, &input) {
			return
		}
		result, err := service.Login(c.Request.Context(), input)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		setRefreshCookie(c, cfg, result.RefreshToken)
		platform.OK(c, http.StatusOK, result)
	})
	r.POST("/api/v1/auth/refresh", trustedOrigin(cfg), refreshLimit.Middleware(), func(c *gin.Context) {
		raw, _ := c.Cookie(refreshCookieName(cfg))
		result, err := service.Refresh(c.Request.Context(), raw)
		if err != nil {
			clearRefreshCookie(c, cfg)
			platform.Fail(c, err)
			return
		}
		setRefreshCookie(c, cfg, result.RefreshToken)
		platform.OK(c, http.StatusOK, result)
	})
	r.POST("/api/v1/auth/logout", trustedOrigin(cfg), refreshLimit.Middleware(), func(c *gin.Context) {
		raw, _ := c.Cookie(refreshCookieName(cfg))
		if err := service.Logout(c.Request.Context(), raw); err != nil {
			platform.Fail(c, err)
			return
		}
		clearRefreshCookie(c, cfg)
		platform.OK(c, http.StatusOK, gin.H{"logged_out": true})
	})
	r.POST("/api/v1/auth/logout-all", trustedOrigin(cfg), service.Middleware(), accountLimit.Middleware(), func(c *gin.Context) {
		if err := service.LogoutAll(c.Request.Context(), auth.CurrentUser(c).ID); err != nil {
			platform.Fail(c, err)
			return
		}
		clearRefreshCookie(c, cfg)
		platform.OK(c, http.StatusOK, gin.H{"logged_out": true})
	})
	users := r.Group("/api/v1/users", service.Middleware())
	users.GET("/me", func(c *gin.Context) { platform.OK(c, http.StatusOK, auth.CurrentUser(c)) })
	users.PATCH("/me", accountLimit.Middleware(), func(c *gin.Context) {
		var input auth.ProfileInput
		if !platform.Decode(c, &input) {
			return
		}
		user, err := service.UpdateProfile(c.Request.Context(), auth.CurrentUser(c).ID, input)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, http.StatusOK, user)
	})
	users.POST("/me/password", trustedOrigin(cfg), accountLimit.Middleware(), func(c *gin.Context) {
		var input auth.PasswordInput
		if !platform.Decode(c, &input) {
			return
		}
		if err := service.ChangePassword(c.Request.Context(), auth.CurrentUser(c).ID, input); err != nil {
			platform.Fail(c, err)
			return
		}
		clearRefreshCookie(c, cfg)
		platform.OK(c, http.StatusOK, gin.H{"password_changed": true, "login_required": true})
	})
	users.DELETE("/me", trustedOrigin(cfg), accountLimit.Middleware(), func(c *gin.Context) {
		var input struct {
			Password string `json:"password"`
		}
		if !platform.Decode(c, &input) {
			return
		}
		if err := service.DeleteAccount(c.Request.Context(), auth.CurrentUser(c).ID, input.Password); err != nil {
			platform.Fail(c, err)
			return
		}
		clearRefreshCookie(c, cfg)
		platform.OK(c, http.StatusOK, gin.H{"deleted": true})
	})
}
