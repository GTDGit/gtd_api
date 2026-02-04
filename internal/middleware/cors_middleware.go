package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// Allowed hosts for CORS (origin is checked by host to cover all URL variants).
var allowedHosts = map[string]bool{
	"localhost:3000":      true,
	"127.0.0.1:3000":      true,
	"admin.gtd.co.id":     true,
	"gtd.co.id":           true,
	"www.admin.gtd.co.id": true,
	"www.gtd.co.id":       true,
}

// originHost returns the host part of origin or referer URL, or empty if invalid.
// Strips default ports (:443, :80) so "admin.gtd.co.id:443" matches "admin.gtd.co.id".
func originHost(raw string) string {
	raw = strings.TrimSpace(strings.TrimSuffix(raw, "/"))
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Host)
	if strings.HasSuffix(host, ":443") || strings.HasSuffix(host, ":80") {
		host, _, _ = strings.Cut(host, ":")
	}
	return host
}

// CORSMiddleware handles Cross-Origin Resource Sharing (CORS) headers.
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			if ref := c.Request.Header.Get("Referer"); ref != "" {
				if u, err := url.Parse(ref); err == nil && u.Scheme != "" && u.Host != "" {
					origin = u.Scheme + "://" + u.Host
				}
			}
		}
		origin = strings.TrimSpace(strings.TrimSuffix(origin, "/"))
		host := originHost(origin)

		if host != "" && allowedHosts[host] {
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
