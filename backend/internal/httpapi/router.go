package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"relations/internal/admin"
	"relations/internal/ai"
	"relations/internal/analysis"
	"relations/internal/auth"
	"relations/internal/config"
	"relations/internal/platform"
)

func New(cfg config.Config, db *gorm.DB, log *slog.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	_ = r.SetTrustedProxies(cfg.TrustedProxies)
	r.Use(func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				log.Error("request_panic", "route", c.FullPath())
				platform.Fail(c, platform.Err(500, "internal_error", "The request could not be completed."))
			}
		}()
		c.Next()
	}, func(c *gin.Context) {
		start := time.Now()
		id := uuid.NewString()
		c.Header("X-Request-ID", id)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Cache-Control", "no-store")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if cfg.Environment == "production" {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
		log.Info("http_request", "request_id", id, "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(), "latency_ms", time.Since(start).Milliseconds())
	})
	r.Use(func(c *gin.Context) {
		if c.Request.ContentLength > 65536 {
			platform.Fail(c, platform.Err(413, "request_too_large", "Request body exceeds the supported size."))
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
		c.Next()
	})
	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := origin == ""
		for _, o := range cfg.AllowedOrigins {
			if origin == o {
				allowed = true
				c.Header("Access-Control-Allow-Origin", o)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Credentials", "true")
			}
		}
		if !allowed {
			platform.Fail(c, platform.Err(403, "origin_denied", "This request origin is not allowed."))
			return
		}
		if c.Request.Method == http.MethodOptions {
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
	r.GET("/api/v1/health/live", func(c *gin.Context) { platform.OK(c, 200, gin.H{"status": "alive"}) })
	r.GET("/api/v1/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		sql, e := db.DB()
		if e == nil {
			e = sql.PingContext(ctx)
		}
		if e != nil {
			platform.Fail(c, platform.Err(503, "not_ready", "Service is not ready."))
			return
		}
		platform.OK(c, 200, gin.H{"status": "ready"})
	})
	accounts := auth.New(cfg, db)
	RegisterAuthRoutes(r, accounts, cfg)
	RegisterGoogleRoutes(r, accounts, cfg)
	RegisterQuestionnaireRoutes(r)
	generator := ai.New(cfg, db)
	analyses := analysis.New(cfg, db, generator)
	RegisterAnalysisRoutes(r, accounts, analyses)
	RegisterAdminRoutes(r, accounts, admin.New(cfg, db, generator), cfg)
	r.NoRoute(func(c *gin.Context) { platform.Fail(c, platform.Err(404, "not_found", "Resource not found.")) })
	return r
}
