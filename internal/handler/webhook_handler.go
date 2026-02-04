package handler

import (
	"encoding/json"
	"io"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/pkg/digiflazz"
)

// WebhookHandler handles incoming webhooks (e.g., from Digiflazz).
type WebhookHandler struct {
	callbackService interface {
		ProcessDigiflazzCallback(payload *digiflazz.CallbackPayload) error
	}
	webhookSecret string
	debug         bool
}

// NewWebhookHandler constructs a WebhookHandler.
func NewWebhookHandler(callbackService interface {
	ProcessDigiflazzCallback(payload *digiflazz.CallbackPayload) error
}, webhookSecret string) *WebhookHandler {
	return &WebhookHandler{
		callbackService: callbackService,
		webhookSecret:   webhookSecret,
		debug:           os.Getenv("ENV") == "development",
	}
}

// HandleDigiflazzCallback handles POST /webhook/digiflazz
func (h *WebhookHandler) HandleDigiflazzCallback(c *gin.Context) {
	// 1. Read body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid body"})
		return
	}

	// Debug logging for development
	if h.debug {
		log.Debug().
			Str("event", c.GetHeader("X-Digiflazz-Event")).
			Str("user_agent", c.GetHeader("User-Agent")).
			Str("signature", c.GetHeader("X-Hub-Signature")).
			RawJSON("raw_body", body).
			Msg("[DIGIFLAZZ WEBHOOK] Incoming callback")
	}

	// 2. (Optional) Verify signature if provided by Digiflazz
	// signature := c.GetHeader("X-Hub-Signature")

	// 3. Parse payload - Digiflazz wraps callback in "data" field
	var wrapper struct {
		Data digiflazz.CallbackPayload `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		log.Error().Err(err).Str("raw_body", string(body)).Msg("Failed to parse Digiflazz callback JSON")
		c.JSON(400, gin.H{"error": "Invalid JSON"})
		return
	}

	// Debug logging parsed payload
	if h.debug {
		log.Debug().
			Str("ref_id", wrapper.Data.RefID).
			Str("customer_no", wrapper.Data.CustomerNo).
			Str("buyer_sku_code", wrapper.Data.BuyerSkuCode).
			Str("status", wrapper.Data.Status).
			Str("rc", wrapper.Data.RC).
			Str("sn", wrapper.Data.SN).
			Str("message", wrapper.Data.Message).
			Int("price", wrapper.Data.Price).
			Msg("[DIGIFLAZZ WEBHOOK] Parsed callback data")
	}

	// 4. Process callback
	if err := h.callbackService.ProcessDigiflazzCallback(&wrapper.Data); err != nil {
		log.Error().Err(err).Msg("Failed to process Digiflazz callback")
		c.JSON(500, gin.H{"error": "Processing failed"})
		return
	}

	c.JSON(200, gin.H{"received": true})
}
