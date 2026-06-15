package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

const (
	qrisCallbackMaxAttempts     = 5
	qrisCallbackSignatureHeader = "X-GTD-Signature"
)

// QRISCallbackService delivers HMAC-signed QRIS webhooks (merchant activation,
// payment success) to the client's configured callback URL and persists each
// attempt to qris_callbacks. It mirrors PaymentCallbackService.
type QRISCallbackService struct {
	callbackRepo *repository.QRISCallbackRepository
	clientRepo   *repository.ClientRepository
	httpClient   *http.Client
}

func NewQRISCallbackService(callbackRepo *repository.QRISCallbackRepository, clientRepo *repository.ClientRepository) *QRISCallbackService {
	return &QRISCallbackService{
		callbackRepo: callbackRepo,
		clientRepo:   clientRepo,
		httpClient:   &http.Client{Timeout: 20 * time.Second},
	}
}

// qrisCallbackEnvelope is the merchant-facing webhook body. `data` is event-
// specific (a merchant on activation, a payment on payment.success).
type qrisCallbackEnvelope struct {
	Event string            `json:"event"`
	Data  any               `json:"data"`
	Meta  qrisCallbackMeta  `json:"meta"`
}

type qrisCallbackMeta struct {
	RequestID string `json:"requestId"`
	Timestamp string `json:"timestamp"`
}

// Enqueue builds the payload, creates a pending row, and attempts a first send.
// The worker retries failed rows. A missing client / callback URL is a no-op.
func (s *QRISCallbackService) Enqueue(ctx context.Context, clientID int, event string, merchantID, paymentID *int, data any) {
	if s == nil || s.callbackRepo == nil {
		return
	}
	client, err := s.clientRepo.GetByID(clientID)
	if err != nil {
		log.Warn().Err(err).Int("clientId", clientID).Msg("qris callback: load client")
		return
	}
	url := strings.TrimSpace(client.CallbackURL)
	if url == "" {
		log.Info().Int("clientId", clientID).Str("event", event).Msg("qris callback: client has no callback URL; skipping")
		return
	}

	payload := buildQRISCallbackPayload(event, data)
	row := &models.QRISCallback{
		ClientID:       clientID,
		QRISMerchantID: merchantID,
		QRISPaymentID:  paymentID,
		Event:          event,
		TargetURL:      url,
		Payload:        models.NullableRawMessage(payload),
		Status:         models.QRISCallbackPending,
		MaxAttempts:    qrisCallbackMaxAttempts,
		NextRetryAt:    time.Now(),
	}
	created, err := s.callbackRepo.Create(ctx, row)
	if err != nil {
		log.Warn().Err(err).Int("clientId", clientID).Str("event", event).Msg("qris callback: insert row")
		return
	}
	s.attemptDelivery(ctx, created, client.CallbackSecret)
}

// RetryDue scans qris_callbacks for due pending rows and re-attempts each one.
func (s *QRISCallbackService) RetryDue(ctx context.Context, limit int) error {
	if s == nil || s.callbackRepo == nil {
		return nil
	}
	rows, err := s.callbackRepo.ListDue(ctx, limit)
	if err != nil {
		return err
	}
	for i := range rows {
		row := &rows[i]
		client, err := s.clientRepo.GetByID(row.ClientID)
		if err != nil {
			continue
		}
		s.attemptDelivery(ctx, row, client.CallbackSecret)
	}
	return nil
}

// attemptDelivery sends one webhook attempt and records the outcome.
func (s *QRISCallbackService) attemptDelivery(ctx context.Context, row *models.QRISCallback, secret string) {
	payload := []byte(row.Payload)
	signature := hmacHexSHA256(payload, secret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, row.TargetURL, bytes.NewReader(payload))
	if err != nil {
		next := qrisNextRetry(row.Attempts + 1)
		_ = s.callbackRepo.MarkFailure(ctx, row.ID, nil, err.Error(), nextOrNil(next, row.Attempts+1))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(qrisCallbackSignatureHeader, "sha256="+signature)
	req.Header.Set("X-GTD-Event", row.Event)
	req.Header.Set("X-GTD-Timestamp", formatPaymentTime(time.Now()))
	req.Header.Set("X-GTD-Request-Id", uuid.New().String())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		next := qrisNextRetry(row.Attempts + 1)
		_ = s.callbackRepo.MarkFailure(ctx, row.ID, nil, err.Error(), nextOrNil(next, row.Attempts+1))
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := s.callbackRepo.MarkDelivered(ctx, row.ID, resp.StatusCode); err != nil {
			log.Warn().Err(err).Int("id", row.ID).Msg("qris callback: mark delivered")
		}
		return
	}

	sc := resp.StatusCode
	next := qrisNextRetry(row.Attempts + 1)
	_ = s.callbackRepo.MarkFailure(ctx, row.ID, &sc, "non-2xx response", nextOrNil(next, row.Attempts+1))
}

// qrisNextRetry applies the backoff schedule 30s / 1m / 5m / 30m / 2h.
func qrisNextRetry(attempt int) time.Time {
	intervals := []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		5 * time.Minute,
		30 * time.Minute,
		2 * time.Hour,
	}
	if attempt <= 0 || attempt > len(intervals) {
		return time.Time{}
	}
	return time.Now().Add(intervals[attempt-1])
}

// nextOrNil returns the retry time only while attempts remain under the cap.
func nextOrNil(next time.Time, attempt int) *time.Time {
	if next.IsZero() || attempt >= qrisCallbackMaxAttempts {
		return nil
	}
	return &next
}

func buildQRISCallbackPayload(event string, data any) []byte {
	out := qrisCallbackEnvelope{
		Event: event,
		Data:  data,
		Meta: qrisCallbackMeta{
			RequestID: uuid.New().String(),
			Timestamp: formatPaymentTime(time.Now()),
		},
	}
	b, _ := json.Marshal(out)
	return b
}
