package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_api/internal/middleware"
	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/internal/utils"
)

// PayoutHandler exposes the client-facing payout (disbursement) endpoints.
type PayoutHandler struct {
	payoutService *service.PayoutService
}

func NewPayoutHandler(payoutService *service.PayoutService) *PayoutHandler {
	return &PayoutHandler{payoutService: payoutService}
}

// CreateInquiry validates a recipient (bank account or e-wallet) before payout.
func (h *PayoutHandler) CreateInquiry(c *gin.Context) {
	var req service.PayoutInquiryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "MISSING_FIELD", "Invalid request body")
		return
	}

	client := middleware.GetClient(c)
	if client == nil {
		utils.Error(c, http.StatusUnauthorized, "INVALID_TOKEN", "Unauthorized")
		return
	}

	resp, err := h.payoutService.Inquiry(c.Request.Context(), &req, client, middleware.IsSandbox(c))
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Inquiry successful", resp)
}

// CreatePayout submits a disbursement.
func (h *PayoutHandler) CreatePayout(c *gin.Context) {
	var req service.CreatePayoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "MISSING_FIELD", "Invalid request body")
		return
	}

	client := middleware.GetClient(c)
	if client == nil {
		utils.Error(c, http.StatusUnauthorized, "INVALID_TOKEN", "Unauthorized")
		return
	}

	resp, err := h.payoutService.Create(c.Request.Context(), &req, client, middleware.IsSandbox(c))
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusCreated, "Payout submitted", resp)
}

// GetPayout returns a payout by its public id.
func (h *PayoutHandler) GetPayout(c *gin.Context) {
	resp, err := h.payoutService.GetPayout(c.Request.Context(), c.Param("payoutId"), c.GetInt("client_id"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payout retrieved", resp)
}

// ListMethods returns the available payout channels (banks + e-wallets).
func (h *PayoutHandler) ListMethods(c *gin.Context) {
	resp, err := h.payoutService.ListMethods(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payout methods retrieved", resp)
}

func (h *PayoutHandler) handleError(c *gin.Context, err error) {
	var payoutErr *service.PayoutServiceError
	if errors.As(err, &payoutErr) {
		utils.Error(c, payoutErr.HTTPStatus, payoutErr.Code, payoutErr.Message)
		return
	}
	utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
}
