package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/pkg/digiflazz"
)

// CallbackService handles outgoing callbacks to client systems and processing
// of incoming Digiflazz callbacks.
type CallbackService struct {
	clientRepo   *repository.ClientRepository
	callbackRepo *repository.CallbackRepository
	httpClient   *http.Client
}

// NewCallbackService constructs a CallbackService with a default HTTP client.
func NewCallbackService(clientRepo *repository.ClientRepository, callbackRepo *repository.CallbackRepository) *CallbackService {
	return &CallbackService{
		clientRepo:   clientRepo,
		callbackRepo: callbackRepo,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// SendCallback sends an HTTP POST webhook to the client's callback URL and logs the attempt.
// It schedules retries when delivery is not successful.
func (s *CallbackService) SendCallback(trx *models.Transaction, event string) error {
	if trx == nil {
		return nil
	}
	client, err := s.clientRepo.GetByID(trx.ClientID)
	if err != nil || client == nil || client.CallbackURL == "" {
		return err
	}

	payload := buildCallbackPayload(trx, event)
	signature := generateSignature(payload, client.CallbackSecret)

	req, err := http.NewRequest(http.MethodPost, client.CallbackURL, bytes.NewReader(payload))
	if err != nil {
		log.Error().Err(err).Msg("failed to create callback request")
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Callback-Token", client.CallbackSecret)
	req.Header.Set("X-Callback-Signature", "sha256="+signature)
	req.Header.Set("X-GTD-Event", event)
	req.Header.Set("X-GTD-Timestamp", time.Now().Format(time.RFC3339))

	resp, err := s.httpClient.Do(req)

	// read response body (best effort)
	var statusCode *int
	var respBody *string
	if resp != nil {
		defer resp.Body.Close()
		sc := resp.StatusCode
		statusCode = &sc
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)
		if bodyStr != "" {
			respBody = &bodyStr
		}
	}

	// Log attempt
	logEntry := &models.CallbackLog{
		TransactionID: trx.ID,
		ClientID:      client.ID,
		Event:         event,
		Payload:       json.RawMessage(payload),
		Attempt:       1,
		HTTPStatus:    statusCode,
		ResponseBody:  respBody,
		IsDelivered:   err == nil && resp != nil && resp.StatusCode == http.StatusOK,
	}
	if !logEntry.IsDelivered {
		next := s.getNextRetryTime(1)
		if !next.IsZero() {
			logEntry.NextRetryAt = &next
		}
	}
	if err := s.callbackRepo.CreateCallbackLog(logEntry); err != nil {
		log.Error().Err(err).Msg("failed to create callback log")
	}

	// Schedule retry automatically handled by worker via RetryPendingCallbacks
	return nil
}

// getNextRetryTime returns next retry time based on attempt number.
// Retry intervals: 30s, 1m, 5m, 30m, 2h
func (s *CallbackService) getNextRetryTime(attempt int) time.Time {
	intervals := []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		5 * time.Minute,
		30 * time.Minute,
		2 * time.Hour,
	}
	if attempt >= len(intervals) {
		return time.Time{}
	}
	return time.Now().Add(intervals[attempt])
}

// RetryPendingCallbacks retries undelivered callbacks.
func (s *CallbackService) RetryPendingCallbacks() error {
	callbacks, err := s.callbackRepo.GetPendingCallbacks()
	if err != nil {
		return err
	}
	for i := range callbacks {
		cb := &callbacks[i]
		client, err := s.clientRepo.GetByID(cb.ClientID)
		if err != nil || client == nil || client.CallbackURL == "" {
			continue
		}
		req, err := http.NewRequest(http.MethodPost, client.CallbackURL, bytes.NewReader(cb.Payload))
		if err != nil {
			continue
		}
		// Recompute signature (payload unchanged)
		sig := generateSignature([]byte(cb.Payload), client.CallbackSecret)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Callback-Token", client.CallbackSecret)
		req.Header.Set("X-Callback-Signature", "sha256="+sig)
		req.Header.Set("X-GTD-Event", cb.Event)
		req.Header.Set("X-GTD-Timestamp", time.Now().Format(time.RFC3339))

		resp, err := s.httpClient.Do(req)
		var statusCode *int
		var respBody *string
		if resp != nil {
			defer resp.Body.Close()
			sc := resp.StatusCode
			statusCode = &sc
			b, _ := io.ReadAll(resp.Body)
			bs := string(b)
			if bs != "" {
				respBody = &bs
			}
		}

		cb.Attempt++
		cb.HTTPStatus = statusCode
		cb.ResponseBody = respBody
		delivered := err == nil && resp != nil && resp.StatusCode == http.StatusOK
		cb.IsDelivered = delivered
		if !delivered {
			next := s.getNextRetryTime(cb.Attempt)
			if next.IsZero() {
				// No more retries
				cb.NextRetryAt = nil
			} else {
				cb.NextRetryAt = &next
			}
		} else {
			cb.NextRetryAt = nil
		}

		if err := s.callbackRepo.UpdateCallbackLog(cb); err != nil {
			log.Error().Err(err).Msg("failed to update callback log")
		}
	}
	return nil
}

// ProcessDigiflazzCallback persists Digiflazz callback and updates transaction state.
func (s *CallbackService) ProcessDigiflazzCallback(payload *digiflazz.CallbackPayload) error {
	if payload == nil {
		return nil
	}
	// Persist raw callback
	raw, _ := json.Marshal(payload)
	cb := &models.DigiflazzCallback{
		DigiRefID: payload.RefID,
		Payload:   json.RawMessage(raw),
		RC:        &payload.RC,
		Status:    &payload.Status,
		SerialNumber: func() *string {
			if payload.SN == "" {
				return nil
			}
			v := payload.SN
			return &v
		}(),
		IsProcessed: false,
	}
	if err := s.callbackRepo.CreateDigiflazzCallback(cb); err != nil {
		log.Error().Err(err).Msg("failed to store digiflazz callback")
	}
	// In full implementation, we would locate the transaction by digi_ref_id and
	// update its status accordingly, then send a callback to the client. That
	// orchestration is intentionally simplified here and delegated to a worker
	// that reads unprocessed digiflazz_callbacks and reconciles transactions.
	return nil
}

// buildCallbackPayload constructs the JSON payload sent to clients.
func buildCallbackPayload(trx *models.Transaction, event string) []byte {
	type dataPayload struct {
		TransactionID string      `json:"transactionId"`
		ReferenceID   string      `json:"referenceId,omitempty"`
		Status        string      `json:"status"`
		Type          string      `json:"type,omitempty"`
		CustomerNo    string      `json:"customerNo,omitempty"`
		CustomerName  *string     `json:"customerName,omitempty"`
		SkuCode       string      `json:"skuCode,omitempty"`
		Amount        *int        `json:"amount,omitempty"`
		Admin         int         `json:"admin,omitempty"`
		Period        *string     `json:"period,omitempty"`
		SerialNumber  *string     `json:"serialNumber,omitempty"`
		Description   interface{} `json:"description,omitempty"`
		FailedReason  *string     `json:"failedReason,omitempty"`
		RetryCount    int         `json:"retryCount,omitempty"`
		NextRetryAt   *time.Time  `json:"nextRetryAt,omitempty"`
		ProcessedAt   *time.Time  `json:"processedAt,omitempty"`
		CreatedAt     time.Time   `json:"createdAt"`
	}
	type payload struct {
		Event     string      `json:"event"`
		Data      dataPayload `json:"data"`
		Timestamp string      `json:"timestamp"`
	}
	var desc any
	if len(trx.Description) > 0 {
		_ = json.Unmarshal(trx.Description, &desc)
	}
	p := payload{
		Event: event,
		Data: dataPayload{
			TransactionID: trx.TransactionID,
			ReferenceID:   trx.ReferenceID,
			Status:        string(trx.Status),
			Type:          string(trx.Type),
			CustomerNo:    trx.CustomerNo,
			CustomerName:  trx.CustomerName,
			SkuCode:       trx.SkuCode,
			Amount:        trx.Amount,
			Admin:         trx.Admin,
			Period:        trx.Period,
			SerialNumber:  trx.SerialNumber,
			Description:   desc,
			FailedReason:  trx.FailedReason,
			RetryCount:    trx.RetryCount,
			NextRetryAt:   trx.NextRetryAt,
			ProcessedAt:   trx.ProcessedAt,
			CreatedAt:     trx.CreatedAt,
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	b, _ := json.Marshal(p)
	return b
}

// generateSignature creates HMAC-SHA256 signature of payload using secret.
func generateSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
