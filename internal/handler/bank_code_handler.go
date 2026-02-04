package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/utils"
)

// BankCodeHandler handles bank code HTTP requests.
type BankCodeHandler struct {
	repo *repository.BankCodeRepository
}

// NewBankCodeHandler creates a new BankCodeHandler.
func NewBankCodeHandler(repo *repository.BankCodeRepository) *BankCodeHandler {
	return &BankCodeHandler{repo: repo}
}

// GetBankCodes returns all active bank codes.
// GET /v1/bank-codes
func (h *BankCodeHandler) GetBankCodes(c *gin.Context) {
	ctx := c.Request.Context()

	banks, err := h.repo.GetAll(ctx)
	if err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve bank codes")
		return
	}

	items := make([]models.BankCodeItem, 0, len(banks))
	for _, b := range banks {
		items = append(items, models.BankCodeItem{
			Name:      b.Name,
			ShortName: b.ShortName,
			Code:      b.Code,
			SwiftCode: b.SwiftCode,
		})
	}

	utils.Success(c, http.StatusOK, "Bank codes retrieved successfully", items)
}
