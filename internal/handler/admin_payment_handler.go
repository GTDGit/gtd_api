package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/internal/utils"
)

// AdminPaymentHandler exposes admin endpoints for managing canonical payment
// methods and their method-provider mappings.
type AdminPaymentHandler struct {
	adminPaymentSvc *service.AdminPaymentService
}

func NewAdminPaymentHandler(adminPaymentSvc *service.AdminPaymentService) *AdminPaymentHandler {
	return &AdminPaymentHandler{adminPaymentSvc: adminPaymentSvc}
}

// ListMethods handles GET /v1/admin/payment-methods — canonical methods
// (de-duplicated by type+code) each with its ordered provider bindings.
func (h *AdminPaymentHandler) ListMethods(c *gin.Context) {
	resp, err := h.adminPaymentSvc.ListMethods(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Successfully", resp)
}

// UpdateMethod handles PUT /v1/admin/payment-methods/:id — edit a canonical
// method's fields.
func (h *AdminPaymentHandler) UpdateMethod(c *gin.Context) {
	id, ok := h.intParam(c, "method")
	if !ok {
		return
	}
	var req service.AdminUpdateMethodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "MISSING_FIELD", "Invalid request body")
		return
	}
	m, err := h.adminPaymentSvc.UpdateMethod(c.Request.Context(), id, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Successfully", m)
}

// ListProviders handles GET /v1/admin/payment-methods/:type/:code/providers —
// the provider bindings for a method, ordered by priority.
func (h *AdminPaymentHandler) ListProviders(c *gin.Context) {
	bindings, err := h.adminPaymentSvc.ListProviders(c.Request.Context(), c.Param("method"), c.Param("code"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Successfully", bindings)
}

// UpdateProviders handles PUT /v1/admin/payment-methods/:type/:code/providers —
// update the ordered bindings (priority, is_active, is_maintenance) for a method.
func (h *AdminPaymentHandler) UpdateProviders(c *gin.Context) {
	var req service.AdminUpdateBindingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "MISSING_FIELD", "Invalid request body")
		return
	}
	bindings, err := h.adminPaymentSvc.UpdateProviders(c.Request.Context(), c.Param("method"), c.Param("code"), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Successfully", bindings)
}

// AvailableProviders handles GET /v1/admin/payment-methods/:type/:code/available-providers
// Returns only providers that have a registered adapter, pass Available(), and are not globally maintained.
func (h *AdminPaymentHandler) AvailableProviders(c *gin.Context) {
	providers, err := h.adminPaymentSvc.AvailableProviders(c.Request.Context(), c.Param("method"), c.Param("code"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Successfully", providers)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (h *AdminPaymentHandler) intParam(c *gin.Context, name string) (int, bool) {
	raw := c.Param(name)
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		utils.Error(c, http.StatusBadRequest, "INVALID_PARAM", name+" must be a positive integer")
		return 0, false
	}
	return id, true
}

func (h *AdminPaymentHandler) handleError(c *gin.Context, err error) {
	var pe *service.PaymentServiceError
	if errors.As(err, &pe) {
		utils.Error(c, pe.HTTPStatus, pe.Code, pe.Message)
		return
	}
	log.Error().Err(err).Str("path", c.FullPath()).Msg("admin payment: unhandled error")
	utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
}
