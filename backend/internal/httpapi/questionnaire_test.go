package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"relations/internal/questionnaire"
)

func TestPublicQuestionnaireVersionAndLocale(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterQuestionnaireRoutes(r)
	for _, path := range []string{"/api/v1/questionnaires/self", "/api/v1/questionnaires/compatibility?version=1.0.0&locale=en", "/api/v1/questionnaires/conflict?locale=fa"} {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		var body struct {
			Data questionnaire.PublicDefinition `json:"data"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Data.Version != "1.0.0" || len(body.Data.Questions) == 0 {
			t.Fatal(path, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), `"reverse"`) || strings.Contains(response.Body.String(), `"references"`) {
			t.Fatal("public endpoint leaks backend scoring metadata")
		}
	}
	for path, expected := range map[string]int{"/api/v1/questionnaires/unknown": 404, "/api/v1/questionnaires/self?version=unsupported": 404, "/api/v1/questionnaires/self?version=": 404, "/api/v1/questionnaires/self?locale=de": 400} {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != expected {
			t.Fatal(path, response.Code, response.Body.String())
		}
	}
}
