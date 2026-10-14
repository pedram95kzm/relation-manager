package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"relations/internal/analysis"
	"relations/internal/auth"
	"relations/internal/platform"
)

func RegisterAnalysisRoutes(r *gin.Engine, accounts *auth.Service, analyses *analysis.Service) {
	group := r.Group("/api/v1/analyses", accounts.Middleware())
	group.GET("", func(c *gin.Context) {
		page, size, err := platform.Pagination(c)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		result, err := analyses.List(c.Request.Context(), auth.CurrentUser(c).ID, page, size, c.Query("module"), c.Query("status"))
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, http.StatusOK, result)
	})
	group.GET("/usage", func(c *gin.Context) {
		result, err := analyses.Usage(c.Request.Context(), auth.CurrentUser(c).ID)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, http.StatusOK, result)
	})
	group.POST("", accounts.RequireProfile(), func(c *gin.Context) {
		var input analysis.CreateInput
		if !platform.Decode(c, &input) {
			return
		}
		result, err := analyses.Create(c.Request.Context(), auth.CurrentUser(c).ID, input)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, http.StatusCreated, result)
	})
	group.GET("/:id", func(c *gin.Context) {
		result, err := analyses.Get(c.Request.Context(), auth.CurrentUser(c).ID, c.Param("id"))
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, http.StatusOK, result)
	})
	group.GET("/:id/report", func(c *gin.Context) {
		result, err := analyses.Get(c.Request.Context(), auth.CurrentUser(c).ID, c.Param("id"))
		if err != nil {
			platform.Fail(c, err)
			return
		}
		if result.Report == nil || result.Status == "draft" || result.Status == "processing" {
			platform.Fail(c, platform.Err(409, "report_not_ready", "The analysis report is not ready yet."))
			return
		}
		platform.OK(c, http.StatusOK, result)
	})
	group.PATCH("/:id", accounts.RequireProfile(), func(c *gin.Context) {
		var input analysis.UpdateInput
		if !platform.Decode(c, &input) {
			return
		}
		result, err := analyses.Update(c.Request.Context(), auth.CurrentUser(c).ID, c.Param("id"), input)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, http.StatusOK, result)
	})
	group.POST("/:id/submit", accounts.RequireProfile(), func(c *gin.Context) {
		var input analysis.SubmitInput
		if !platform.Decode(c, &input) {
			return
		}
		result, err := analyses.Submit(c.Request.Context(), auth.CurrentUser(c).ID, c.Param("id"), input)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, http.StatusOK, result)
	})
	group.DELETE("/:id", func(c *gin.Context) {
		if err := analyses.Delete(c.Request.Context(), auth.CurrentUser(c).ID, c.Param("id")); err != nil {
			platform.Fail(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
}
