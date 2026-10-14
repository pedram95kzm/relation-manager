package auth

import (
	"strings"

	"github.com/gin-gonic/gin"
	"relations/internal/platform"
)

const userContextKey = "authenticated_user"

func CurrentUser(c *gin.Context) User {
	value, _ := c.Get(userContextKey)
	user, _ := value.(User)
	return user
}

func (s *Service) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			platform.Fail(c, invalidCredentials())
			return
		}
		user, err := s.Authenticate(c.Request.Context(), parts[1])
		if err != nil {
			platform.Fail(c, err)
			return
		}
		c.Set(userContextKey, user)
		c.Next()
	}
}

func (s *Service) RequireProfile() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.ProfileComplete(CurrentUser(c)) {
			platform.Fail(c, platform.Err(403, "profile_required", "Complete the required eligible age and gender profile first."))
			return
		}
		c.Next()
	}
}

func (s *Service) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if CurrentUser(c).Role != "admin" {
			platform.Fail(c, platform.Err(403, "admin_required", "Administrator access is required."))
			return
		}
		c.Next()
	}
}
