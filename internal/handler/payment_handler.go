package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_api/internal/middleware"
	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/internal/utils"
)

type PaymentHandler struct {
	paymentService *service.PaymentService
}

func NewPaymentHandler(paymentService *service.PaymentService) *PaymentHandler {
	return &PaymentHandler{paymentService: paymentService}
}

// ListMethods returns active payment methods grouped by type.
func (h *PaymentHandler) ListMethods(c *gin.Context) {
	resp, err := h.paymentService.ListMethods(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payment methods retrieved", resp)
}

// CreatePayment creates a pending payment and calls the provider.
func (h *PaymentHandler) CreatePayment(c *gin.Context) {
	var req service.CreatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "MISSING_FIELD", "Invalid request body")
		return
	}
	client := middleware.GetClient(c)
	if client == nil {
		utils.Error(c, http.StatusUnauthorized, "INVALID_TOKEN", "Unauthorized")
		return
	}
	resp, err := h.paymentService.CreatePayment(c.Request.Context(), &req, client, middleware.IsSandbox(c))
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusCreated, "Payment created", resp)
}

// GetPayment returns a single payment, refreshing from the provider if stale.
func (h *PaymentHandler) GetPayment(c *gin.Context) {
	client := middleware.GetClient(c)
	if client == nil {
		utils.Error(c, http.StatusUnauthorized, "INVALID_TOKEN", "Unauthorized")
		return
	}
	resp, err := h.paymentService.GetPayment(c.Request.Context(), c.Param("paymentId"), client.ID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payment retrieved", resp)
}

// CancelPayment cancels a pending payment.
func (h *PaymentHandler) CancelPayment(c *gin.Context) {
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	client := middleware.GetClient(c)
	if client == nil {
		utils.Error(c, http.StatusUnauthorized, "INVALID_TOKEN", "Unauthorized")
		return
	}
	resp, err := h.paymentService.CancelPayment(c.Request.Context(), c.Param("paymentId"), client.ID, body.Reason)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payment cancelled", resp)
}

// RefundPayment issues a refund against a paid payment.
func (h *PaymentHandler) RefundPayment(c *gin.Context) {
	var req service.CreateRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "MISSING_FIELD", "Invalid request body")
		return
	}
	client := middleware.GetClient(c)
	if client == nil {
		utils.Error(c, http.StatusUnauthorized, "INVALID_TOKEN", "Unauthorized")
		return
	}
	refund, err := h.paymentService.RefundPayment(c.Request.Context(), c.Param("paymentId"), client.ID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusCreated, "Refund submitted", refund)
}

func (h *PaymentHandler) handleError(c *gin.Context, err error) {
	var pe *service.PaymentServiceError
	if errors.As(err, &pe) {
		utils.Error(c, pe.HTTPStatus, pe.Code, pe.Message)
		return
	}
	utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
}
