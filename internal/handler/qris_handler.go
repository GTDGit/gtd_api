package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_api/internal/middleware"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/internal/utils"
)

// QRISHandler exposes the client-facing static-QRIS API: register a merchant
// (Nobu Excel-batch onboarding), list/get registrations, and query payment
// history. Auth is the client API key + qris scope.
type QRISHandler struct {
	registrationSvc *service.QRISRegistrationService
	paymentSvc      *service.QRISPaymentService
	batchSvc        *service.QRISBatchService
}

func NewQRISHandler(registrationSvc *service.QRISRegistrationService, paymentSvc *service.QRISPaymentService, batchSvc *service.QRISBatchService) *QRISHandler {
	return &QRISHandler{registrationSvc: registrationSvc, paymentSvc: paymentSvc, batchSvc: batchSvc}
}

// CreateMerchant handles POST /v1/qris/merchants — a client registration intake.
func (h *QRISHandler) CreateMerchant(c *gin.Context) {
	client := middleware.GetClient(c)
	if client == nil {
		utils.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid API key")
		return
	}

	var req service.QRISRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	reg, err := h.registrationSvc.Register(c.Request.Context(), client.ID, req)
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	utils.Success(c, http.StatusCreated, "QRIS merchant registration received", reg)
}

// ListMerchants handles GET /v1/qris/merchants — the client's own registrations.
func (h *QRISHandler) ListMerchants(c *gin.Context) {
	client := middleware.GetClient(c)
	if client == nil {
		utils.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid API key")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	status := c.Query("status")

	items, total, err := h.registrationSvc.List(c.Request.Context(), client.ID, page, limit, status)
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	utils.SuccessWithPagination(c, http.StatusOK, "Registrations retrieved", gin.H{"items": items}, page, limit, total)
}

// GetMerchant handles GET /v1/qris/merchants/:ref — one registration by ref.
func (h *QRISHandler) GetMerchant(c *gin.Context) {
	client := middleware.GetClient(c)
	if client == nil {
		utils.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid API key")
		return
	}
	reg, err := h.registrationSvc.Get(c.Request.Context(), client.ID, c.Param("ref"))
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Registration retrieved", reg)
}

// ListPayments handles GET /v1/qris/payments — the client's standardized QRIS
// payment history, filterable by storeId / subMerchantId / orderId / from / to.
func (h *QRISHandler) ListPayments(c *gin.Context) {
	client := middleware.GetClient(c)
	if client == nil {
		utils.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid API key")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 20
	}

	f := repository.QRISPaymentFilter{
		ClientID:      client.ID,
		StoreID:       strings.TrimSpace(c.Query("storeId")),
		SubMerchantID: strings.TrimSpace(c.Query("subMerchantId")),
		OrderID:       strings.TrimSpace(c.Query("orderId")),
		Limit:         limit,
		Offset:        (page - 1) * limit,
	}
	if v := strings.TrimSpace(c.Query("from")); v != "" {
		if t, err := parseQRISDate(v); err == nil {
			f.From = &t
		} else {
			utils.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "from must be an ISO 8601 date/time")
			return
		}
	}
	if v := strings.TrimSpace(c.Query("to")); v != "" {
		if t, err := parseQRISDate(v); err == nil {
			f.To = &t
		} else {
			utils.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "to must be an ISO 8601 date/time")
			return
		}
	}

	items, total, err := h.paymentSvc.ListPaymentsForClient(c.Request.Context(), f)
	if err != nil {
		var pErr *service.QRISPaymentServiceError
		if errors.As(err, &pErr) {
			utils.Error(c, pErr.HTTPStatus, "ERROR", pErr.Message)
			return
		}
		utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unexpected error")
		return
	}
	utils.SuccessWithPagination(c, http.StatusOK, "Payments retrieved", gin.H{"items": items}, page, limit, total)
}

// parseQRISDate accepts a full RFC3339 timestamp or a bare YYYY-MM-DD (WIB).
func parseQRISDate(v string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	wib := time.FixedZone("WIB", 7*3600)
	return time.ParseInLocation("2006-01-02", v, wib)
}

// AdminActivateRequest is the admin activation payload (Nobu's returned IDs).
type AdminActivateRequest struct {
	SubMerchantID string `json:"subMerchantId"`
	StoreID       string `json:"storeId"`
	TerminalID    string `json:"terminalId"`
	QRISString    string `json:"qrisString"` // optional manual paste / override
}

// AdminActivate handles POST /v1/admin/qris/registrations/:id/activate — an
// operator provisions the merchant after Nobu returns its identifiers. Protected
// by admin JWT. Generates the QR string via Nobu (manual paste fallback) and
// fires the qris.merchant.activated webhook to the client.
func (h *QRISHandler) AdminActivate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		utils.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "registration id must be a positive integer")
		return
	}
	var req AdminActivateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	merchant, err := h.registrationSvc.Activate(c.Request.Context(), id, service.QRISActivateInput{
		SubMerchantID: req.SubMerchantID,
		StoreID:       req.StoreID,
		TerminalID:    req.TerminalID,
		QRISString:    req.QRISString,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "QRIS merchant activated", merchant)
}

// AdminListRegistrations handles GET /v1/admin/qris/registrations — all clients'
// registrations (admin view), filterable by status.
func (h *QRISHandler) AdminListRegistrations(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	status := strings.TrimSpace(c.Query("status"))
	items, total, err := h.registrationSvc.AdminList(c.Request.Context(), page, limit, status)
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	utils.SuccessWithPagination(c, http.StatusOK, "Registrations retrieved", gin.H{"items": items}, page, limit, total)
}

// AdminListBatches handles GET /v1/admin/qris/batches — rendered Nobu Excel batches.
func (h *QRISHandler) AdminListBatches(c *gin.Context) {
	if h.batchSvc == nil {
		utils.Error(c, http.StatusServiceUnavailable, "UNAVAILABLE", "batch service is not configured")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	items, total, err := h.batchSvc.ListBatches(c.Request.Context(), page, limit)
	if err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list batches")
		return
	}
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	utils.SuccessWithPagination(c, http.StatusOK, "Batches retrieved", gin.H{"items": items}, page, limit, total)
}

// AdminDownloadBatch handles GET /v1/admin/qris/batches/:id/download — streams
// the rendered Excel file for manual delivery to Nobu's WhatsApp group.
func (h *QRISHandler) AdminDownloadBatch(c *gin.Context) {
	if h.batchSvc == nil {
		utils.Error(c, http.StatusServiceUnavailable, "UNAVAILABLE", "batch service is not configured")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		utils.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "batch id must be a positive integer")
		return
	}
	data, fileName, err := h.batchSvc.GetBatchFile(c.Request.Context(), id)
	if err != nil {
		utils.Error(c, http.StatusNotFound, "NOT_FOUND", "batch file not found")
		return
	}
	c.Header("Content-Disposition", "attachment; filename=\""+fileName+"\"")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

// AdminMarkBatchSent handles POST /v1/admin/qris/batches/:id/sent.
func (h *QRISHandler) AdminMarkBatchSent(c *gin.Context) {
	if h.batchSvc == nil {
		utils.Error(c, http.StatusServiceUnavailable, "UNAVAILABLE", "batch service is not configured")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		utils.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "batch id must be a positive integer")
		return
	}
	if err := h.batchSvc.MarkBatchSent(c.Request.Context(), id); err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to mark batch sent")
		return
	}
	utils.Success(c, http.StatusOK, "Batch marked sent", gin.H{"id": id, "status": "sent"})
}

// writeServiceError maps a QRISRegistrationServiceError to the response envelope.
func (h *QRISHandler) writeServiceError(c *gin.Context, err error) {
	var svcErr *service.QRISRegistrationServiceError
	if errors.As(err, &svcErr) {
		code := svcErr.Code
		if code == "" {
			code = "ERROR"
		}
		utils.Error(c, svcErr.HTTPStatus, code, svcErr.Message)
		return
	}
	utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unexpected error")
}
