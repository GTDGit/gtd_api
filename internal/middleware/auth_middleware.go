package middleware

import (
    "strings"

    "github.com/gin-gonic/gin"

    "github.com/GTDGit/gtd_api/internal/models"
    "github.com/GTDGit/gtd_api/internal/service"
    "github.com/GTDGit/gtd_api/internal/utils"
)

// AuthMiddleware handles API key authentication, client validation, and IP checks.
type AuthMiddleware struct {
    authService *service.AuthService
    rateLimiter *InvalidAuthRateLimiter
}

// NewAuthMiddleware constructs a new AuthMiddleware.
func NewAuthMiddleware(authService *service.AuthService) *AuthMiddleware {
    return &AuthMiddleware{
        authService: authService,
        rateLimiter: NewInvalidAuthRateLimiter(),
    }
}

// Handle returns a Gin middleware function that enforces authentication.
func (m *AuthMiddleware) Handle() gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. Extract Bearer token
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
            m.handleAuthError(c, 401, "UNAUTHORIZED", "Missing or invalid API key")
            return
        }
        token := strings.TrimPrefix(authHeader, "Bearer ")

        // 2. Validate API key (live or sandbox)
        client, isSandbox, err := m.authService.ValidateAPIKey(token)
        if err != nil || client == nil {
            m.handleAuthError(c, 401, "UNAUTHORIZED", "Missing or invalid API key")
            return
        }

        // 3. Check if client is active
        if !client.IsActive {
            m.handleAuthError(c, 403, "FORBIDDEN", "API key is not allowed to perform this action")
            return
        }

        // 4. Validate Client ID header
        clientID := c.GetHeader("X-Client-Id")
        if !m.authService.ValidateClientID(client, clientID) {
            m.handleAuthError(c, 401, "UNAUTHORIZED", "Missing or invalid API key")
            return
        }

        // 5. Validate IP whitelist
        clientIP := c.ClientIP()
        if !m.authService.IsIPAllowed(client, clientIP) {
            m.handleAuthError(c, 403, "IP_NOT_ALLOWED", "Request from unauthorized IP address")
            return
        }

        // 6. Set context values
        c.Set("client", client)
        c.Set("is_sandbox", isSandbox)
        c.Set("client_id", client.ID)

        c.Next()
    }
}

func (m *AuthMiddleware) handleAuthError(c *gin.Context, httpStatus int, code, message string) {
    // Apply rate limit for invalid auth attempts
    ip := c.ClientIP()
    if !m.rateLimiter.Allow(ip) {
        utils.Error(c, 429, "RATE_LIMITED", "Too many requests, please try again later")
        c.Abort()
        return
    }

    utils.Error(c, httpStatus, code, message)
    c.Abort()
}

// GetClient returns the authenticated client from context.
func GetClient(c *gin.Context) *models.Client {
    client, _ := c.Get("client")
    if client == nil {
        return nil
    }
    return client.(*models.Client)
}

// IsSandbox indicates whether the request is in sandbox mode.
func IsSandbox(c *gin.Context) bool {
    isSandbox, _ := c.Get("is_sandbox")
    if isSandbox == nil {
        return false
    }
    return isSandbox.(bool)
}
