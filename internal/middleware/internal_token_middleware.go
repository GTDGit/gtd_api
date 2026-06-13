package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// InternalToken guards service-to-service endpoints (the gateway calls these to
// reach provider clients that only live in the api service). It checks a shared
// secret in the X-Internal-Token header using a constant-time comparison.
func InternalToken(expected string) gin.HandlerFunc {
	expected = strings.TrimSpace(expected)
	return func(c *gin.Context) {
		if expected == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"code":    "INTERNAL_DISABLED",
				"message": "internal endpoints are not configured",
			})
			return
		}
		got := strings.TrimSpace(c.GetHeader("X-Internal-Token"))
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "UNAUTHORIZED",
				"message": "invalid internal token",
			})
			return
		}
		c.Next()
	}
}
