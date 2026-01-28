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
            m.handleAuthError(c, "INVALID_TOKEN", "Missing or invalid authorization header")
            return
        }
        token := strings.TrimPrefix(authHeader, "Bearer ")

        // 2. Validate API key (live or sandbox)
        client, isSandbox, err := m.authService.ValidateAPIKey(token)
        if err != nil || client == nil {
            m.handleAuthError(c, "INVALID_TOKEN", "Invalid API token")
            return
        }

        // 3. Check if client is active
        if !client.IsActive {
            m.handleAuthError(c, "INVALID_CLIENT", "Client is not active")
            return
        }

        // 4. Validate Client ID header
        clientID := c.GetHeader("X-Client-Id")
        if !m.authService.ValidateClientID(client, clientID) {
            m.handleAuthError(c, "INVALID_CLIENT", "Client ID mismatch")
            return
        }

        // 5. Validate IP whitelist
        clientIP := c.ClientIP()
        if !m.authService.IsIPAllowed(client, clientIP) {
            m.handleAuthError(c, "INVALID_IP", "Request from unauthorized IP address")
            return
        }

        // 6. Set context values
        c.Set("client", client)
        c.Set("is_sandbox", isSandbox)
        c.Set("client_id", client.ID)

        c.Next()
    }
}

func (m *AuthMiddleware) handleAuthError(c *gin.Context, code, message string) {
    // Apply rate limit for invalid auth attempts
    ip := c.ClientIP()
    if !m.rateLimiter.Allow(ip) {
        utils.Error(c, 429, "TOO_MANY_REQUESTS", "Too many invalid authentication attempts")
        c.Abort()
        return
    }

    utils.Error(c, 401, code, message)
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
