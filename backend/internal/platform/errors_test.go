package platform

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStrictBoundedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, s := range []string{`{"name":"ok","secret":"unexpected"}`, `{"name":"ok"} {}`, `{"name":"first","name":"second"}`, `{"name":"` + strings.Repeat("x", 70000) + `"}`} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", bytes.NewBufferString(s))
		c.Request.Header.Set("Content-Type", "application/json")
		var input struct {
			Name string `json:"name"`
		}
		if Decode(c, &input) || w.Code != 400 {
			t.Error("accepted invalid payload")
		}
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?page=bad", nil)
	if _, _, e := Pagination(c); e == nil {
		t.Fatal("accepted invalid page")
	}
}
