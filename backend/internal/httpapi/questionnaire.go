package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"relations/internal/platform"
	"relations/internal/questionnaire"
)

func RegisterQuestionnaireRoutes(r *gin.Engine) {
	r.GET("/api/v1/questionnaires/:module", func(c *gin.Context) {
		version := c.DefaultQuery("version", questionnaire.Version)
		locale := c.DefaultQuery("locale", "fa")
		if !questionnaire.ValidLocale(locale) {
			platform.Fail(c, platform.Err(http.StatusBadRequest, "invalid_locale", "Select Persian or English."))
			return
		}
		definition, err := questionnaire.Get(c.Param("module"), version)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		platform.OK(c, http.StatusOK, questionnaire.Public(definition))
	})
}
