package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/pkg/pakailink"
)

// DisbursementWebhookHandler receives provider-side callbacks for transfers.
type DisbursementWebhookHandler struct {
	transferRepo    *repository.TransferRepository
	transferSvc     *service.TransferService
	pakailinkSecret string
}

// NewDisbursementWebhookHandler constructs a DisbursementWebhookHandler.
func NewDisbursementWebhookHandler(
	transferRepo *repository.TransferRepository,
	transferSvc *service.TransferService,
	pakailinkSecret string,
) *DisbursementWebhookHandler {
	return &DisbursementWebhookHandler{
		transferRepo:    transferRepo,
		transferSvc:     transferSvc,
		pakailinkSecret: pakailinkSecret,
	}
}

// HandlePakailink processes a PakaiLink Service 44 callback for previously
// pending bank transfers. Verifies the symmetric signature, persists the raw
// callback for audit, and updates the underlying transfer.
func (h *DisbursementWebhookHandler) HandlePakailink(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"responseCode": "4004400", "responseMessage": "Invalid body"})
		return
	}

	timestamp := c.GetHeader("X-TIMESTAMP")
	signature := c.GetHeader("X-SIGNATURE")
	path := c.Request.URL.Path

	cb := &models.TransferCallback{
		Provider:         models.DisbursementProviderPakaiLink,
		Headers:          headersAsJSON(c.Request.Header),
		Payload:          models.NullableRawMessage(body),
		Signature:        nilIfEmpty(signature),
		IsValidSignature: false,
		IsProcessed:      false,
	}
	if err := h.transferRepo.CreateTransferCallback(c.Request.Context(), cb); err != nil {
		log.Error().Err(err).Msg("pakailink disbursement webhook: persist raw callback")
		c.JSON(http.StatusInternalServerError, gin.H{"responseCode": "5004400", "responseMessage": "Failed to record callback"})
		return
	}

	valid := pakailink.VerifyWebhookSignature("POST", path, "", body, timestamp, signature, h.pakailinkSecret)
	if err := h.transferRepo.UpdateTransferCallbackSignature(c.Request.Context(), cb.ID, valid); err != nil {
		log.Warn().Err(err).Msg("pakailink disbursement webhook: update signature flag")
	}
	if !valid {
		h.markError(c.Request.Context(), cb.ID, "invalid signature")
		c.JSON(http.StatusUnauthorized, gin.H{"responseCode": "4014400", "responseMessage": "Unauthorized signature"})
		return
	}

	var payload pakailink.DisbursementCallback
	if err := json.Unmarshal(body, &payload); err != nil {
		h.markError(c.Request.Context(), cb.ID, "invalid JSON")
		c.JSON(http.StatusBadRequest, gin.H{"responseCode": "4004400", "responseMessage": "Invalid request body"})
		return
	}
	data := payload.TransactionData
	partnerRef := strings.TrimSpace(data.PartnerReferenceNo)
	if partnerRef == "" {
		h.markError(c.Request.Context(), cb.ID, "missing partnerReferenceNo")
		c.JSON(http.StatusBadRequest, gin.H{"responseCode": "4004400", "responseMessage": "Missing partnerReferenceNo"})
		return
	}

	paid, _ := pakailink.ParseWebhookAmount(data.PaidAmount)
	fee, _ := pakailink.ParseWebhookAmount(data.FeeAmount)

	transfer, err := h.transferSvc.ApplyPakailinkCallback(c.Request.Context(), service.PakailinkCallbackEvent{
		PaymentFlagStatus: data.PaymentFlagStatus,
		PartnerReference:  partnerRef,
		ReferenceNo:       data.ReferenceNo,
		AccountNumber:     data.AccountNumber,
		AccountName:       data.AccountName,
		PaidAmount:        paid,
		FeeAmount:         fee,
		RawPayload:        body,
	})
	if err != nil {
		// Unknown reference is still a valid 200 response per SNAP, so the
		// provider stops retrying — but we record the error.
		if errors.Is(err, sql.ErrNoRows) {
			h.markError(c.Request.Context(), cb.ID, "transfer not found")
			c.JSON(http.StatusOK, gin.H{"responseCode": "2004400", "responseMessage": "Successful"})
			return
		}
		h.markError(c.Request.Context(), cb.ID, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"responseCode": "5004400", "responseMessage": "Processing error"})
		return
	}

	transferIDPtr := transfer.TransferID
	statusStr := string(transfer.Status)
	if err := h.markProcessed(c.Request.Context(), cb.ID, &transferIDPtr, &statusStr); err != nil {
		log.Warn().Err(err).Msg("pakailink disbursement webhook: mark processed")
	}

	c.JSON(http.StatusOK, gin.H{"responseCode": "2004400", "responseMessage": "Successful"})
}

func (h *DisbursementWebhookHandler) markError(ctx context.Context, id int, msg string) {
	m := msg
	_ = h.transferRepo.UpdateTransferCallbackProcessed(ctx, id, false, &m)
}

func (h *DisbursementWebhookHandler) markProcessed(ctx context.Context, id int, transferID, status *string) error {
	_ = transferID
	_ = status
	return h.transferRepo.UpdateTransferCallbackProcessed(ctx, id, true, nil)
}

func headersAsJSON(h http.Header) models.NullableRawMessage {
	b, err := json.Marshal(h)
	if err != nil {
		return nil
	}
	return models.NullableRawMessage(b)
}

func nilIfEmpty(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}
