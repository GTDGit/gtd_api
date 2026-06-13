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
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

// PayoutCallbackService delivers final payout state to the client's webhook.
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

// Send returns handled=true when no retry is needed anymore (delivered, no URL,
// or non-final state). It returns an error only on transient delivery failures.
func (s *PayoutCallbackService) Send(ctx context.Context, payout *models.Payout) (bool, error) {
	if payout == nil {
		return true, nil
	}
	if payout.Status != models.PayoutStatusSuccess && payout.Status != models.PayoutStatusFailed {
		return false, nil
	}

	// Prefer the per-request callback URL; fall back to the client's generic one.
	callbackURL := derefString(payout.CallbackURL)
	secret := ""
	if s.clientRepo != nil {
		if client, err := s.clientRepo.GetByID(payout.ClientID); err == nil && client != nil {
			secret = client.CallbackSecret
			if callbackURL == "" {
				callbackURL = client.CallbackURL
			}
		} else if err != nil {
			return false, err
		}
	}
	if callbackURL == "" {
		return true, nil
	}

	payload, event := s.buildPayload(ctx, payout)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GTD-Signature", "sha256="+signPayoutCallback(payload, secret))
	req.Header.Set("X-GTD-Event", event)
	req.Header.Set("X-GTD-Timestamp", formatPayoutTime(time.Now()))
	req.Header.Set("X-GTD-Request-Id", newPayoutCallbackRequestID())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("payout callback returned status %d", resp.StatusCode)
	}
	return true, nil
}

func (s *PayoutCallbackService) buildPayload(ctx context.Context, payout *models.Payout) ([]byte, string) {
	event := "payout.failed"
	if payout.Status == models.PayoutStatusSuccess {
		event = "payout.success"
	}

	bankName := derefString(payout.BankName)
	if payout.MethodType == models.MethodTypeBank && bankName == "" && s.bankRepo != nil {
		if bank, err := s.bankRepo.GetByCode(ctx, payout.ChannelCode); err == nil && bank != nil {
			bankName = bank.Name
		}
	}

	data := map[string]any{
		"payoutId":    payout.PayoutID,
		"referenceId": payout.ReferenceID,
		"status":      string(payout.Status),
		"payoutMethod": map[string]any{
			"type": string(payout.MethodType),
			"code": payout.ChannelCode,
			"name": nonEmptyOrDefault(bankName, payout.ChannelCode),
		},
		"accountNumber": payout.AccountNumber,
		"accountName":   derefString(payout.AccountName),
		"amount":        payout.Amount,
		"fee":           payout.Fee,
		"totalAmount":   payout.TotalAmount,
		"feePaidBy":     string(payout.FeePaidBy),
		"providerRef":   derefString(payout.ProviderRef),
		"createdAt":     formatPayoutTime(payout.CreatedAt),
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
		return []byte("{}"), event
	}
	return raw, event
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
