package handler

import (
    "time"

    "github.com/gin-gonic/gin"

    "github.com/GTDGit/gtd_api/internal/utils"
    "github.com/GTDGit/gtd_api/pkg/digiflazz"
)

var startTime = time.Now()

// HealthHandler provides health endpoint.
type HealthHandler struct {
    digiflazz *digiflazz.Client
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(digiflazz *digiflazz.Client) *HealthHandler {
    return &HealthHandler{digiflazz: digiflazz}
}

// GetHealth responds with service and Digiflazz status.
func (h *HealthHandler) GetHealth(c *gin.Context) {
    balance, err := h.digiflazz.GetBalance(c.Request.Context())

    digiStatus := "connected"
    var digiBalance int
    if err != nil {
        digiStatus = "disconnected"
    } else {
        digiBalance = balance.Deposit
    }

    utils.Success(c, 200, "Service is healthy", gin.H{
        "status":  "healthy",
        "version": "1.0.0",
        "uptime":  int(time.Since(startTime).Seconds()),
        "digiflazz": gin.H{
            "status":  digiStatus,
            "balance": digiBalance,
        },
    })
}
