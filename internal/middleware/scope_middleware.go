package middleware

import (
	"slices"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/utils"
)

// Re-export scope identifiers from models for ergonomic call sites in routing.
const (
	ScopePPOB         = models.ScopePPOB
	ScopePayment      = models.ScopePayment
	ScopeDisbursement = models.ScopeDisbursement
)

// RequireScope enforces that the authenticated client carries the given scope.
// Must be chained after AuthMiddleware.Handle so the client is in context.
func RequireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		client := GetClient(c)
		if client == nil {
			utils.Error(c, 401, "INVALID_TOKEN", "Authentication required")
			c.Abort()
			return
		}

		if !slices.Contains(client.Scopes, scope) {
			utils.Error(c, 403, "INSUFFICIENT_SCOPE", "API key does not have required scope: "+scope)
			c.Abort()
			return
		}

		c.Next()
	}
}
