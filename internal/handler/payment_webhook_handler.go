package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/pkg/dana"
	"github.com/GTDGit/gtd_api/pkg/midtrans"
	"github.com/GTDGit/gtd_api/pkg/pakailink"
	"github.com/GTDGit/gtd_api/pkg/xendit"
)

// PaymentWebhookHandler receives provider-side webhooks for payments.
type PaymentWebhookHandler struct {
	paymentRepo      *repository.PaymentRepository
	paymentSvc       *service.PaymentService
	pakailinkSecret  string
	danaClientSecret string
	midtransKey      string
	xenditToken      string
}

func NewPaymentWebhookHandler(
	paymentRepo *repository.PaymentRepository,
	paymentSvc *service.PaymentService,
	pakailinkSecret, danaClientSecret, midtransServerKey, xenditWebhookToken string,
) *PaymentWebhookHandler {
	return &PaymentWebhookHandler{
		paymentRepo:      paymentRepo,
		paymentSvc:       paymentSvc,
		pakailinkSecret:  pakailinkSecret,
		danaClientSecret: danaClientSecret,
		midtransKey:      midtransServerKey,
		xenditToken:      xenditWebhookToken,
	}
}

// ---------------------------------------------------------------------------
// Pakailink — handles both VA and QRIS callbacks. Dual-webhook dedupe:
// settlement type is ACK-only (no state change, no client callback).
// ---------------------------------------------------------------------------

func (h *PaymentWebhookHandler) HandlePakailink(c *gin.Context) {
	body, cb, ok := h.persistRawCallback(c, models.ProviderPakailink)
	if !ok {
		return
	}
	timestamp := c.GetHeader("X-TIMESTAMP")
	signature := c.GetHeader("X-SIGNATURE")
	path := c.Request.URL.Path
	valid := pakailink.VerifyWebhookSignature("POST", path, "", body, timestamp, signature, h.pakailinkSecret)
	if err := h.paymentRepo.UpdatePaymentCallbackSignature(c.Request.Context(), cb.ID, valid); err != nil {
		log.Warn().Err(err).Msg("pakailink webhook: update signature flag")
	}
	if !valid {
		h.markCallbackError(c.Request.Context(), cb.ID, "invalid signature")
		respondSNAP(c, http.StatusUnauthorized, "4010001", "Unauthorized signature")
		return
	}

	var payload pakailink.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		h.markCallbackError(c.Request.Context(), cb.ID, "invalid JSON")
		respondSNAP(c, http.StatusBadRequest, "4000001", "Invalid request body")
		return
	}
	data := payload.TransactionData
	partnerRef := strings.TrimSpace(data.PartnerReferenceNo)
	callbackType := strings.ToLower(strings.TrimSpace(data.CallbackType))

	// Settlement callback: ACK only.
	if callbackType == "settlement" {
		_ = h.paymentRepo.UpdatePaymentCallbackProcessed(c.Request.Context(), cb.ID, true, nil)
		respondSNAP(c, http.StatusOK, "2002500", "Successful")
		return
	}

	event := service.PaymentWebhookEvent{
		Status:     mapPakailinkFlagStatus(data.PaymentFlagStatus),
		PaidAmount: parseAmountString(data.PaidAmount.Value),
		RawPayload: body,
	}
	if err := h.paymentSvc.ApplyWebhook(c.Request.Context(), models.ProviderPakailink, partnerRef, event); err != nil {
		h.markCallbackError(c.Request.Context(), cb.ID, err.Error())
		respondSNAP(c, http.StatusInternalServerError, "5000001", "Processing error")
		return
	}
	_ = h.paymentRepo.UpdatePaymentCallbackProcessed(c.Request.Context(), cb.ID, true, nil)
	respondSNAP(c, http.StatusOK, "2002500", "Successful")
}

// ---------------------------------------------------------------------------
// DANA — SNAP-style webhook for ewallet + QRIS notifications.
// ---------------------------------------------------------------------------

type danaWebhookPayload struct {
	OriginalPartnerReferenceNo string             `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo        string             `json:"originalReferenceNo"`
	LatestTransactionStatus    string             `json:"latestTransactionStatus"`
	TransactionStatusDesc      string             `json:"transactionStatusDesc"`
	Amount                     danaWebhookAmount  `json:"amount"`
	AdditionalInfo             json.RawMessage    `json:"additionalInfo,omitempty"`
}

type danaWebhookAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

func (h *PaymentWebhookHandler) HandleDANA(c *gin.Context) {
	body, cb, ok := h.persistRawCallback(c, models.ProviderDanaDirect)
	if !ok {
		return
	}
	timestamp := c.GetHeader("X-TIMESTAMP")
	signature := c.GetHeader("X-SIGNATURE")
	valid := dana.VerifyWebhookSignature("POST", c.Request.URL.Path, "", body, timestamp, signature, h.danaClientSecret)
	if err := h.paymentRepo.UpdatePaymentCallbackSignature(c.Request.Context(), cb.ID, valid); err != nil {
		log.Warn().Err(err).Msg("dana webhook: update signature flag")
	}
	if !valid {
		h.markCallbackError(c.Request.Context(), cb.ID, "invalid signature")
		respondSNAP(c, http.StatusUnauthorized, "4010001", "Unauthorized signature")
		return
	}
	var p danaWebhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		h.markCallbackError(c.Request.Context(), cb.ID, "invalid JSON")
		respondSNAP(c, http.StatusBadRequest, "4000001", "Invalid request body")
		return
	}
	event := service.PaymentWebhookEvent{
		Status:      mapDanaWebhookStatus(p.LatestTransactionStatus),
		ProviderRef: p.OriginalReferenceNo,
		PaidAmount:  parseAmountString(p.Amount.Value),
		RawPayload:  body,
	}
	if err := h.paymentSvc.ApplyWebhook(c.Request.Context(), models.ProviderDanaDirect, strings.TrimSpace(p.OriginalPartnerReferenceNo), event); err != nil {
		h.markCallbackError(c.Request.Context(), cb.ID, err.Error())
		respondSNAP(c, http.StatusInternalServerError, "5000001", "Processing error")
		return
	}
	_ = h.paymentRepo.UpdatePaymentCallbackProcessed(c.Request.Context(), cb.ID, true, nil)
	respondSNAP(c, http.StatusOK, "2002500", "Successful")
}

// ---------------------------------------------------------------------------
// Midtrans — SHA-512 signature_key, plain JSON ACK.
// ---------------------------------------------------------------------------

type midtransWebhookPayload struct {
	OrderID           string `json:"order_id"`
	TransactionID     string `json:"transaction_id"`
	StatusCode        string `json:"status_code"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	GrossAmount       string `json:"gross_amount"`
	PaymentType       string `json:"payment_type"`
	SignatureKey      string `json:"signature_key"`
}

func (h *PaymentWebhookHandler) HandleMidtrans(c *gin.Context) {
	body, cb, ok := h.persistRawCallback(c, models.ProviderMidtrans)
	if !ok {
		return
	}
	var p midtransWebhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		h.markCallbackError(c.Request.Context(), cb.ID, "invalid JSON")
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid body"})
		return
	}
	valid := midtrans.VerifyWebhookSignature(p.OrderID, p.StatusCode, p.GrossAmount, h.midtransKey, p.SignatureKey)
	if err := h.paymentRepo.UpdatePaymentCallbackSignature(c.Request.Context(), cb.ID, valid); err != nil {
		log.Warn().Err(err).Msg("midtrans webhook: update signature flag")
	}
	if !valid {
		h.markCallbackError(c.Request.Context(), cb.ID, "invalid signature")
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Invalid signature"})
		return
	}
	status := mapMidtransWebhookStatus(p.TransactionStatus, p.FraudStatus)
	event := service.PaymentWebhookEvent{
		Status:      status,
		ProviderRef: p.TransactionID,
		PaidAmount:  parseAmountString(p.GrossAmount),
		RawPayload:  body,
	}
	if err := h.paymentSvc.ApplyWebhook(c.Request.Context(), models.ProviderMidtrans, strings.TrimSpace(p.OrderID), event); err != nil {
		h.markCallbackError(c.Request.Context(), cb.ID, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	_ = h.paymentRepo.UpdatePaymentCallbackProcessed(c.Request.Context(), cb.ID, true, nil)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Xendit — static x-callback-token header; plain JSON ACK.
// ---------------------------------------------------------------------------

type xenditWebhookPayload struct {
	EventType         string          `json:"event"`
	Data              xenditEventData `json:"data"`
	// Raw fields for legacy shapes that embed them at the top level.
	ID                string          `json:"id"`
	ReferenceID       string          `json:"reference_id"`
	Status            string          `json:"status"`
	RequestAmount     int64           `json:"request_amount"`
	ChannelCode       string          `json:"channel_code"`
}

type xenditEventData struct {
	ID            string `json:"id"`
	ReferenceID   string `json:"reference_id"`
	Status        string `json:"status"`
	RequestAmount int64  `json:"request_amount"`
	ChannelCode   string `json:"channel_code"`
}

func (h *PaymentWebhookHandler) HandleXendit(c *gin.Context) {
	body, cb, ok := h.persistRawCallback(c, models.ProviderXendit)
	if !ok {
		return
	}
	token := c.GetHeader("x-callback-token")
	valid := xendit.VerifyWebhookToken(token, h.xenditToken)
	if err := h.paymentRepo.UpdatePaymentCallbackSignature(c.Request.Context(), cb.ID, valid); err != nil {
		log.Warn().Err(err).Msg("xendit webhook: update signature flag")
	}
	if !valid {
		h.markCallbackError(c.Request.Context(), cb.ID, "invalid callback token")
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Invalid token"})
		return
	}
	var p xenditWebhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		h.markCallbackError(c.Request.Context(), cb.ID, "invalid JSON")
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid body"})
		return
	}
	ref := firstPaymentString(p.Data.ReferenceID, p.ReferenceID)
	providerID := firstPaymentString(p.Data.ID, p.ID)
	status := firstPaymentString(p.Data.Status, p.Status)
	amount := p.Data.RequestAmount
	if amount == 0 {
		amount = p.RequestAmount
	}
	event := service.PaymentWebhookEvent{
		Status:      mapXenditWebhookStatus(status),
		ProviderRef: providerID,
		PaidAmount:  amount,
		RawPayload:  body,
	}
	if err := h.paymentSvc.ApplyWebhook(c.Request.Context(), models.ProviderXendit, strings.TrimSpace(ref), event); err != nil {
		h.markCallbackError(c.Request.Context(), cb.ID, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	_ = h.paymentRepo.UpdatePaymentCallbackProcessed(c.Request.Context(), cb.ID, true, nil)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// persistRawCallback reads the body and stores a payment_callbacks row before
// signature verification so audit history is preserved even on rejects.
func (h *PaymentWebhookHandler) persistRawCallback(c *gin.Context, provider models.PaymentProvider) ([]byte, *models.PaymentCallback, bool) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid body"})
		return nil, nil, false
	}
	headersJSON, _ := json.Marshal(c.Request.Header)
	signature := c.GetHeader("X-SIGNATURE")
	if signature == "" {
		signature = c.GetHeader("x-callback-token")
	}
	var sigPtr *string
	if signature != "" {
		s := signature
		sigPtr = &s
	}
	cb := &models.PaymentCallback{
		Provider:         provider,
		Headers:          models.NullableRawMessage(headersJSON),
		Payload:          models.NullableRawMessage(body),
		Signature:        sigPtr,
		IsValidSignature: false,
		IsProcessed:      false,
	}
	if err := h.paymentRepo.CreatePaymentCallback(c.Request.Context(), cb); err != nil {
		log.Error().Err(err).Str("provider", string(provider)).Msg("payment webhook: persist raw")
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to record callback"})
		return nil, nil, false
	}
	return body, cb, true
}

func (h *PaymentWebhookHandler) markCallbackError(ctx context.Context, id int, msg string) {
	m := msg
	_ = h.paymentRepo.UpdatePaymentCallbackProcessed(ctx, id, false, &m)
}

func respondSNAP(c *gin.Context, httpStatus int, code, message string) {
	c.JSON(httpStatus, gin.H{
		"responseCode":    code,
		"responseMessage": message,
		"timestamp":       time.Now().UTC().Format("2006-01-02T15:04:05-07:00"),
	})
}

func parseAmountString(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if idx := strings.Index(s, "."); idx > 0 {
		s = s[:idx]
	}
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func firstPaymentString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Status mapping
// ---------------------------------------------------------------------------

func mapPakailinkFlagStatus(code string) models.PaymentStatus {
	switch strings.TrimSpace(code) {
	case "00", "SUCCESS":
		return models.PaymentStatusPaid
	case "05", "CANCELLED":
		return models.PaymentStatusCancelled
	case "06", "FAILED":
		return models.PaymentStatusFailed
	case "07", "EXPIRED":
		return models.PaymentStatusExpired
	default:
		return models.PaymentStatusPending
	}
}

func mapDanaWebhookStatus(code string) models.PaymentStatus {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "SUCCESS", "PAID":
		return models.PaymentStatusPaid
	case "CANCELLED", "CLOSED":
		return models.PaymentStatusCancelled
	case "FAILED":
		return models.PaymentStatusFailed
	case "EXPIRED":
		return models.PaymentStatusExpired
	case "REFUNDED":
		return models.PaymentStatusRefunded
	default:
		return models.PaymentStatusPending
	}
}

func mapMidtransWebhookStatus(status, fraudStatus string) models.PaymentStatus {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case midtrans.StatusSettlement, midtrans.StatusCapture:
		if strings.EqualFold(fraudStatus, "challenge") {
			return models.PaymentStatusPending
		}
		return models.PaymentStatusPaid
	case midtrans.StatusDeny:
		return models.PaymentStatusFailed
	case midtrans.StatusExpire:
		return models.PaymentStatusExpired
	case midtrans.StatusCancel:
		return models.PaymentStatusCancelled
	case midtrans.StatusRefund:
		return models.PaymentStatusRefunded
	case "partial_refund":
		return models.PaymentStatusPartialRefund
	default:
		return models.PaymentStatusPending
	}
}

func mapXenditWebhookStatus(s string) models.PaymentStatus {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case xendit.StatusSucceeded, "COMPLETED":
		return models.PaymentStatusPaid
	case xendit.StatusExpired:
		return models.PaymentStatusExpired
	case xendit.StatusCanceled, "CANCELLED":
		return models.PaymentStatusCancelled
	case xendit.StatusFailed:
		return models.PaymentStatusFailed
	default:
		return models.PaymentStatusPending
	}
}
