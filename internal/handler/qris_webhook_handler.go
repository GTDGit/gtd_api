package handler

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/pkg/pakailink"
)

// QRISWebhookHandler receives static-QRIS payment notifications from Pakailink
// (Service 52). Nobu QRIS notifications use the connector pattern instead (token
// + HMAC), handled by NobuConnectorHandler.
type QRISWebhookHandler struct {
	qrisPaymentSvc *service.QRISPaymentService
	pakailinkPub   *rsa.PublicKey
}

func NewQRISWebhookHandler(
	qrisPaymentSvc *service.QRISPaymentService,
	pakailinkPub *rsa.PublicKey,
) *QRISWebhookHandler {
	return &QRISWebhookHandler{
		qrisPaymentSvc: qrisPaymentSvc,
		pakailinkPub:   pakailinkPub,
	}
}

// HandlePakailink verifies the RSA signature, then records the successful QRIS
// payment. The merchant is identified by storeId (falling back to merchantId)
// echoed in additionalInfo, matched against qris_merchants.store_id.
func (h *QRISWebhookHandler) HandlePakailink(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondSNAP(c, http.StatusBadRequest, "4005200", "Invalid body")
		return
	}

	timestamp := c.GetHeader("X-TIMESTAMP")
	signature := c.GetHeader("X-SIGNATURE")
	pathCandidates := []string{
		c.Request.URL.Path,
		"https://" + c.Request.Host + c.Request.URL.Path,
	}

	if !pakailink.VerifyWebhookSignature("POST", pathCandidates, body, timestamp, signature, h.pakailinkPub) {
		log.Warn().Msg("pakailink qris webhook: invalid signature")
		respondSNAP(c, http.StatusUnauthorized, "4015200", "Unauthorized signature")
		return
	}

	var payload pakailink.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		respondSNAP(c, http.StatusBadRequest, "4005200", "Invalid request body")
		return
	}

	data := payload.ResolveTransactionData()

	// settlement callbacks are ACK-only (dedupe parity with payment webhook).
	if strings.EqualFold(strings.TrimSpace(data.CallbackType), "settlement") {
		respondSNAP(c, http.StatusOK, "2005200", "Successful")
		return
	}

	// Only record successful payments. Pakailink QRIS success flag is "00".
	status := strings.TrimSpace(data.PaymentFlagStatus)
	if status != "" && status != pakailink.StatusSuccess && !strings.EqualFold(status, "success") {
		respondSNAP(c, http.StatusOK, "2005200", "Successful")
		return
	}

	storeID := firstNonEmpty(
		additionalInfoString(data.AdditionalInfo, "storeId"),
		additionalInfoString(data.AdditionalInfo, "storeID"),
		additionalInfoString(data.AdditionalInfo, "merchantId"),
		additionalInfoString(data.AdditionalInfo, "merchantID"),
	)
	referenceNo := firstNonEmpty(data.OriginalReferenceNo, data.PartnerReferenceNo)
	amount, _ := pakailink.ParseWebhookAmount(data.PaidAmount)

	event := service.QRISPaymentEvent{
		Provider:           models.QRISProviderPakailink,
		StoreID:            storeID,
		ReferenceNo:        referenceNo,
		PartnerReferenceNo: data.PartnerReferenceNo,
		RRN:                additionalInfoString(data.AdditionalInfo, "rrn"),
		PaymentReferenceNo: additionalInfoString(data.AdditionalInfo, "paymentReferenceNo"),
		IssuerID:           additionalInfoString(data.AdditionalInfo, "issuer"),
		TerminalID:         additionalInfoString(data.AdditionalInfo, "terminalId"),
		Amount:             amount,
		PayerName:          additionalInfoString(data.AdditionalInfo, "payor"),
		PaidAt:             timePtr(time.Now()),
		RawPayload:         body,
	}

	_, _, err = h.qrisPaymentSvc.RecordQRISPayment(c.Request.Context(), event)
	if err != nil {
		var svcErr *service.QRISPaymentServiceError
		if errors.As(err, &svcErr) && svcErr.HTTPStatus == http.StatusNotFound {
			log.Warn().Str("store_id", storeID).Str("reference_no", referenceNo).
				Msg("pakailink qris webhook for unknown merchant; acknowledged")
			respondSNAP(c, http.StatusOK, "2005200", "Successful")
			return
		}
		log.Error().Err(err).Msg("pakailink qris webhook: record payment")
		respondSNAP(c, http.StatusInternalServerError, "5005200", "Processing error")
		return
	}

	respondSNAP(c, http.StatusOK, "2005200", "Successful")
}

func additionalInfoString(info map[string]any, key string) string {
	if info == nil {
		return ""
	}
	if v, ok := info[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func timePtr(t time.Time) *time.Time { return &t }
