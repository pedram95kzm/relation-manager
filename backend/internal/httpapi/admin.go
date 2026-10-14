package httpapi

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"relations/internal/admin"
	"relations/internal/ai"
	"relations/internal/auth"
	"relations/internal/config"
	"relations/internal/platform"
)

func RegisterAdminRoutes(r *gin.Engine, accounts *auth.Service, service *admin.Service, cfg config.Config) {
	group := r.Group("/api/v1/admin", accounts.Middleware(), accounts.RequireAdmin())
	mutations := platform.NewLimiter(30, time.Minute)
	tests := platform.NewLimiter(3, time.Minute)
	group.GET("/analytics", func(c *gin.Context) {
		days := 30
		if value := c.Query("days"); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				platform.Fail(c, platform.Err(400, "invalid_range", "Analytics days must be between 1 and 90."))
				return
			}
			days = parsed
		}
		result, err := service.Analytics(c.Request.Context(), days)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, 200, result)
	})
	group.GET("/users", func(c *gin.Context) {
		page, size, err := platform.Pagination(c)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		result, err := service.Users(c.Request.Context(), c.Query("q"), page, size)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, 200, result)
	})
	group.PATCH("/users/:id", trustedOrigin(cfg), mutations.Middleware(), func(c *gin.Context) {
		var input admin.UserInput
		if !platform.Decode(c, &input) {
			return
		}
		result, err := service.UpdateUser(c.Request.Context(), auth.CurrentUser(c).ID, c.Param("id"), input)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, 200, result)
	})
	group.DELETE("/users/:id", trustedOrigin(cfg), mutations.Middleware(), func(c *gin.Context) {
		var input admin.DeleteUserInput
		if !platform.Decode(c, &input) {
			return
		}
		if err := service.DeleteUser(c.Request.Context(), auth.CurrentUser(c).ID, c.Param("id"), input); err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, 200, gin.H{"deleted": true})
	})
	group.GET("/provider", func(c *gin.Context) {
		result, err := service.Provider().GetProvider(c.Request.Context())
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, 200, result)
	})
	group.PUT("/provider", trustedOrigin(cfg), mutations.Middleware(), func(c *gin.Context) {
		var input ai.ProviderInput
		if !platform.Decode(c, &input) {
			return
		}
		result, err := service.Provider().SetProvider(c.Request.Context(), auth.CurrentUser(c).ID, input)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, 200, result)
	})
	group.POST("/provider/test", trustedOrigin(cfg), tests.Middleware(), func(c *gin.Context) {
		var input *struct{}
		if !platform.Decode(c, &input) {
			return
		}
		if input == nil {
			platform.Fail(c, platform.Err(400, "invalid_request", "Use an empty JSON object for the connection check."))
			return
		}
		result, err := service.Provider().TestProvider(c.Request.Context(), auth.CurrentUser(c).ID)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, 200, result)
	})
	group.POST("/provider/preview", trustedOrigin(cfg), mutations.Middleware(), func(c *gin.Context) {
		var input ai.PreviewInput
		if !platform.Decode(c, &input) {
			return
		}
		result, err := service.Provider().Preview(c.Request.Context(), auth.CurrentUser(c).ID, input)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, 200, result)
	})
	group.GET("/limits", func(c *gin.Context) {
		result, err := service.GetLimits(c.Request.Context())
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, 200, result)
	})
	group.PUT("/limits", trustedOrigin(cfg), mutations.Middleware(), func(c *gin.Context) {
		var input admin.Limits
		if !platform.Decode(c, &input) {
			return
		}
		result, err := service.SetLimits(c.Request.Context(), auth.CurrentUser(c).ID, input)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, 200, result)
	})
	group.GET("/audit", func(c *gin.Context) {
		page, size, err := platform.Pagination(c)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		result, err := service.Audit(c.Request.Context(), page, size)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, 200, result)
	})
}
