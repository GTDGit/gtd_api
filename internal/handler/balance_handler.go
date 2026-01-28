package handler

import (
    "github.com/gin-gonic/gin"

    "github.com/GTDGit/gtd_api/internal/utils"
    "github.com/GTDGit/gtd_api/pkg/digiflazz"
)

// BalanceHandler exposes balance endpoints.
type BalanceHandler struct {
    digiflazz *digiflazz.Client
}

// NewBalanceHandler constructs a BalanceHandler.
func NewBalanceHandler(digiflazz *digiflazz.Client) *BalanceHandler {
    return &BalanceHandler{digiflazz: digiflazz}
}

// GetBalance returns Digiflazz deposit balance.
func (h *BalanceHandler) GetBalance(c *gin.Context) {
    balance, err := h.digiflazz.GetBalance(c.Request.Context())
    if err != nil {
        utils.Error(c, 500, "INTERNAL_ERROR", "Failed to get balance")
        return
    }
    utils.Success(c, 200, "Balance retrieved", gin.H{
        "deposit": balance.Deposit,
    })
}
