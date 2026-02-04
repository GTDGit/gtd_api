package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware handles Cross-Origin Resource Sharing (CORS) headers.
func CORSMiddleware() gin.HandlerFunc {
	// Allowed origins (no trailing slash; comparison normalizes origin)
	allowedOrigins := map[string]bool{
		"http://localhost:3000":       true,
		"http://127.0.0.1:3000":       true,
		"https://admin.gtd.co.id":     true,
		"https://gtd.co.id":           true,
		"https://www.admin.gtd.co.id": true,
		"https://www.gtd.co.id":       true,
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			// Fallback: some proxies strip Origin; derive from Referer (e.g. https://admin.gtd.co.id/)
			if ref := c.Request.Header.Get("Referer"); ref != "" {
				if u, err := url.Parse(ref); err == nil && u.Scheme != "" && u.Host != "" {
					origin = u.Scheme + "://" + u.Host
				}
			}
		}
		// Normalize: browser may send "https://admin.gtd.co.id/" with trailing slash
		originNorm := strings.TrimSuffix(origin, "/")

		// Check if origin is allowed
		if origin != "" && allowedOrigins[originNorm] {
			// Echo back the exact origin the browser expects (with or without slash)
			c.Header("Access-Control-Allow-Origin", origin)
		}

		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Api-Key, X-Client-Id")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")
		c.Header("Access-Control-Max-Age", "86400")

		// Handle preflight request
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
