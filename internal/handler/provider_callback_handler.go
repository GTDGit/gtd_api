package handler

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/internal/utils"
)

// ProviderCallbackHandler handles callbacks from PPOB providers
type ProviderCallbackHandler struct {
	callbackSvc      *service.ProviderCallbackService
	alterraPublicKey *rsa.PublicKey
}

// NewProviderCallbackHandler creates a new ProviderCallbackHandler
func NewProviderCallbackHandler(callbackSvc *service.ProviderCallbackService, alterraPublicKeyPEM string) *ProviderCallbackHandler {
	h := &ProviderCallbackHandler{
		callbackSvc: callbackSvc,
	}

	// Parse Alterra public key if provided
	if alterraPublicKeyPEM != "" {
		block, _ := pem.Decode([]byte(alterraPublicKeyPEM))
		if block != nil {
			pub, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err == nil {
				if rsaPub, ok := pub.(*rsa.PublicKey); ok {
					h.alterraPublicKey = rsaPub
					log.Info().Msg("Alterra callback signature verification enabled")
				}
			} else {
				log.Warn().Err(err).Msg("Failed to parse Alterra callback public key")
			}
		} else {
			log.Warn().Msg("Failed to decode Alterra callback public key PEM")
		}
	} else {
		log.Warn().Msg("Alterra callback public key not configured - signature verification disabled")
	}

	return h
}

// verifyAlterraSignature verifies an RSA-SHA256 signature from Alterra
func (h *ProviderCallbackHandler) verifyAlterraSignature(body []byte, signatureB64 string) bool {
	if h.alterraPublicKey == nil {
		return true // Skip verification if no public key configured
	}

	signature, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to decode Alterra callback signature")
		return false
	}

	hash := sha256.Sum256(body)
	err = rsa.VerifyPKCS1v15(h.alterraPublicKey, crypto.SHA256, hash[:], signature)
	if err != nil {
		log.Warn().Err(err).Msg("Alterra callback signature verification failed")
		return false
	}

	return true
}

// HandleKiosbankCallback handles callback from Kiosbank
func (h *ProviderCallbackHandler) HandleKiosbankCallback(c *gin.Context) {
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		log.Error().Err(err).Msg("Failed to parse Kiosbank callback body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	log.Info().Interface("payload", payload).Msg("Received Kiosbank callback")

	// Process the callback
	if err := h.callbackSvc.ProcessKiosbankCallback(c.Request.Context(), payload); err != nil {
		log.Error().Err(err).Msg("Failed to process Kiosbank callback")
		// Still return 200 to acknowledge receipt
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// HandleAlterraCallback handles callback from Alterra
func (h *ProviderCallbackHandler) HandleAlterraCallback(c *gin.Context) {
	// Read raw body for signature verification
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read Alterra callback body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	log.Info().RawJSON("payload", body).Msg("Received Alterra callback")

	// Verify signature if present
	signature := c.GetHeader("X-Signature")
	if signature != "" {
		if !h.verifyAlterraSignature(body, signature) {
			log.Warn().Msg("Alterra callback rejected: invalid signature")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}
		log.Debug().Msg("Alterra callback signature verified")
	} else if h.alterraPublicKey != nil {
		// Signature header missing but we have a public key configured - reject
		log.Warn().Msg("Alterra callback rejected: missing X-Signature header")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing signature"})
		return
	}

	// Parse body into map (body already consumed, use json.Unmarshal directly)
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Error().Err(err).Msg("Failed to parse Alterra callback JSON")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Process the callback
	if err := h.callbackSvc.ProcessAlterraCallback(c.Request.Context(), payload); err != nil {
		log.Error().Err(err).Msg("Failed to process Alterra callback")
		// Still return 200 to acknowledge receipt
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// HandleGenericCallback handles generic provider callback (fallback)
func (h *ProviderCallbackHandler) HandleGenericCallback(c *gin.Context) {
	providerCode := c.Param("provider")

	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		log.Error().Err(err).Str("provider", providerCode).Msg("Failed to parse provider callback body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	log.Info().Str("provider", providerCode).Interface("payload", payload).Msg("Received provider callback")

	if err := h.callbackSvc.ProcessGenericCallback(c.Request.Context(), providerCode, payload); err != nil {
		log.Error().Err(err).Str("provider", providerCode).Msg("Failed to process provider callback")
	}

	utils.Success(c, http.StatusOK, "Callback received", nil)
}
