package httpserver

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"feed-videos-system/backend/internal/domain/auth"
	"feed-videos-system/backend/internal/transport/httpserver/response"
)

const currentUserIDKey = "currentUserID"

func AuthRequired(authService *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := c.GetHeader("Authorization")
		if authorization == "" {
			response.Unauthorized(c, "authorization header is required")
			c.Abort()
			return
		}

		token := strings.TrimPrefix(authorization, "Bearer ")
		if token == authorization {
			response.Unauthorized(c, "bearer token is required")
			c.Abort()
			return
		}

		claims, err := authService.Parse(token)
		if err != nil {
			response.Unauthorized(c, "invalid token")
			c.Abort()
			return
		}

		c.Set(currentUserIDKey, claims.UserID)
		c.Next()
	}
}

func OptionalAuth(authService *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := c.GetHeader("Authorization")
		if authorization == "" {
			c.Next()
			return
		}

		token := strings.TrimPrefix(authorization, "Bearer ")
		if token == authorization {
			c.Next()
			return
		}

		claims, err := authService.Parse(token)
		if err == nil {
			c.Set(currentUserIDKey, claims.UserID)
		}
		c.Next()
	}
}

func CurrentUserID(c *gin.Context) (uint, bool) {
	value, exists := c.Get(currentUserIDKey)
	if !exists {
		return 0, false
	}
	id, ok := value.(uint)
	return id, ok
}

func NoRoute(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
}
