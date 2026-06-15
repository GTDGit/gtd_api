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
	ScopeQRIS         = models.ScopeQRIS
)

// RequireScope enforces that the authenticated client carries the given scope.
// Must be chained after AuthMiddleware.Handle so the client is in context.
func RequireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		client := GetClient(c)
		if client == nil {
			utils.Error(c, 401, "UNAUTHORIZED", "Missing or invalid API key")
			c.Abort()
			return
		}

		if !slices.Contains(client.Scopes, scope) {
			utils.Error(c, 403, "FORBIDDEN", "API key is not allowed to perform this action (required scope: "+scope+")")
			c.Abort()
			return
		}

		c.Next()
	}
}
