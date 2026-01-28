package handler

import (
    "encoding/json"
    "io"

    "github.com/gin-gonic/gin"
    "github.com/rs/zerolog/log"

    "github.com/GTDGit/gtd_api/pkg/digiflazz"
)

// WebhookHandler handles incoming webhooks (e.g., from Digiflazz).
type WebhookHandler struct {
    callbackService interface{ ProcessDigiflazzCallback(payload *digiflazz.CallbackPayload) error }
    webhookSecret   string
}

// NewWebhookHandler constructs a WebhookHandler.
func NewWebhookHandler(callbackService interface{ ProcessDigiflazzCallback(payload *digiflazz.CallbackPayload) error }, webhookSecret string) *WebhookHandler {
    return &WebhookHandler{callbackService: callbackService, webhookSecret: webhookSecret}
}

// HandleDigiflazzCallback handles POST /webhook/digiflazz
func (h *WebhookHandler) HandleDigiflazzCallback(c *gin.Context) {
    // 1. Read body
    body, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.JSON(400, gin.H{"error": "Invalid body"})
        return
    }

    // 2. (Optional) Verify signature if provided by Digiflazz
    // signature := c.GetHeader("X-Hub-Signature")

    // 3. Parse payload
    var payload digiflazz.CallbackPayload
    if err := json.Unmarshal(body, &payload); err != nil {
        c.JSON(400, gin.H{"error": "Invalid JSON"})
        return
    }

    // 4. Process callback
    if err := h.callbackService.ProcessDigiflazzCallback(&payload); err != nil {
        log.Error().Err(err).Msg("Failed to process Digiflazz callback")
        c.JSON(500, gin.H{"error": "Processing failed"})
        return
    }

    c.JSON(200, gin.H{"received": true})
}
