package platform

import (
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string                    { return e.Code }
func Err(status int, code, message string) *Error { return &Error{status, code, message} }
func Fail(c *gin.Context, err error) {
	var e *Error
	if !errors.As(err, &e) {
		e = Err(500, "internal_error", "The request could not be completed.")
	}
	c.AbortWithStatusJSON(e.Status, gin.H{"error": e})
}
func OK(c *gin.Context, status int, data interface{}) { c.JSON(status, gin.H{"data": data}) }
func Decode(c *gin.Context, target interface{}) bool {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		Fail(c, Err(415, "unsupported_media_type", "Use application/json."))
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 65536))
	if err != nil || StrictJSON(body, target) != nil {
		Fail(c, Err(400, "invalid_request", "Invalid request fields or body size."))
		return false
	}
	return true
}
