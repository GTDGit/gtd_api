package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

// PayoutCallbackService delivers payout lifecycle events to the client's webhook.
// Signature: X-GTD-Signature: sha256=<hex(hmac-sha256(body, secret))>.
type PayoutCallbackService struct {
	clientRepo *repository.ClientRepository
	bankRepo   *repository.BankCodeRepository
	httpClient *http.Client
}

func NewPayoutCallbackService(clientRepo *repository.ClientRepository, bankRepo *repository.BankCodeRepository) *PayoutCallbackService {
	return &PayoutCallbackService{
		clientRepo: clientRepo,
		bankRepo:   bankRepo,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

// Send delivers the final-state webhook (payout.success / payout.failed). It
// returns handled=true when no retry is needed anymore (delivered, no URL, or
// non-final state). It returns an error only on transient delivery failures.
func (s *PayoutCallbackService) Send(ctx context.Context, payout *models.Payout) (bool, error) {
	if payout == nil {
		return true, nil
	}
	if payout.Status != models.PayoutStatusSuccess && payout.Status != models.PayoutStatusFailed {
		return false, nil
	}
	if err := s.deliver(ctx, payout, payoutEventName(payout.Status)); err != nil {
		if isNoCallbackTarget(err) {
			return true, nil
		}
		return false, err
	}
	return true, nil
}

// SendProcessing delivers the payout.processing webhook immediately after a
// payout is accepted. It is best-effort: failures are not retried because the
// final-state callback (success/failed) is the authoritative delivery.
func (s *PayoutCallbackService) SendProcessing(ctx context.Context, payout *models.Payout) error {
	if payout == nil {
		return nil
	}
	if err := s.deliver(ctx, payout, "payout.processing"); err != nil {
		if isNoCallbackTarget(err) {
			return nil
		}
		return err
	}
	return nil
}

// errNoCallbackTarget signals that no callback URL is configured, so there is
// nothing to deliver and no reason to retry.
var errNoCallbackTarget = fmt.Errorf("no callback target")

func isNoCallbackTarget(err error) bool { return err == errNoCallbackTarget }

// deliver resolves the callback URL + secret and POSTs the signed event payload.
func (s *PayoutCallbackService) deliver(ctx context.Context, payout *models.Payout, event string) error {
	callbackURL := derefString(payout.CallbackURL)
	secret := ""
	if s.clientRepo != nil {
		if client, err := s.clientRepo.GetByID(payout.ClientID); err == nil && client != nil {
			secret = client.CallbackSecret
			if callbackURL == "" {
				callbackURL = client.CallbackURL
			}
		} else if err != nil {
			return err
		}
	}
	if callbackURL == "" {
		return errNoCallbackTarget
	}

	payload := s.buildPayload(payout, event)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GTD-Signature", "sha256="+signPayoutCallback(payload, secret))
	req.Header.Set("X-GTD-Event", event)
	req.Header.Set("X-GTD-Timestamp", formatPayoutTime(time.Now()))
	req.Header.Set("X-GTD-Request-Id", newPayoutCallbackRequestID())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("payout callback returned status %d", resp.StatusCode)
	}
	return nil
}

// buildPayload renders the webhook body. The data block mirrors the public
// PayoutResponse (id, nested amount, no providerRef) so clients consume one
// consistent shape across the API and webhooks.
func (s *PayoutCallbackService) buildPayload(payout *models.Payout, event string) []byte {
	data := map[string]any{
		"id":          payout.PayoutID,
		"referenceId": payout.ReferenceID,
		"status":      string(payout.Status),
		"payoutMethod": map[string]any{
			"type": string(payout.MethodType),
			"code": payout.ChannelCode,
		},
		"accountNumber": payout.AccountNumber,
		"accountName":   derefString(payout.AccountName),
		"amount": map[string]any{
			"subtotal": payout.Amount,
			"fee":      payout.Fee,
			"total":    payout.TotalAmount,
		},
		"feePaidBy": string(payout.FeePaidBy),
		"createdAt": formatPayoutTime(payout.CreatedAt),
	}
	if desc := derefString(payout.Description); desc != "" {
		data["description"] = desc
	}
	if payout.CompletedAt != nil {
		data["completedAt"] = formatPayoutTime(*payout.CompletedAt)
	}
	if payout.FailedAt != nil {
		data["failedAt"] = formatPayoutTime(*payout.FailedAt)
	}
	if payout.Status == models.PayoutStatusFailed {
		data["failedReason"] = derefString(payout.FailedReason)
		data["failedCode"] = derefString(payout.FailedCode)
	}

	payload := map[string]any{
		"event":     event,
		"data":      data,
		"timestamp": formatPayoutTime(time.Now()),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

// payoutEventName maps a payout status to its webhook event name
// (payout.success / payout.failed / payout.processing).
func payoutEventName(status models.PayoutStatus) string {
	return "payout." + strings.ToLower(string(status))
}

func signPayoutCallback(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func newPayoutCallbackRequestID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return "po_cb_" + hex.EncodeToString(buf)
}
