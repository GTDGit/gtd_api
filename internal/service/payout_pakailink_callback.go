package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
)

// PakailinkCallbackEvent is the parsed Service 44 callback delivered by
// PakaiLink for previously-pending payouts (bank transfers and e-wallet top-ups).
type PakailinkCallbackEvent struct {
	PaymentFlagStatus string // "00" success, "06" failed, "03" pending
	PartnerReference  string // maps to payouts.payout_id
	ReferenceNo       string
	AccountNumber     string
	AccountName       string
	PaidAmount        int64
	FeeAmount         int64
	RawPayload        []byte
}

// ApplyPakailinkCallback updates payout state from a Service 44 callback.
// Returns the matched payout (or nil) so the handler can decide on response.
func (s *PayoutService) ApplyPakailinkCallback(ctx context.Context, ev PakailinkCallbackEvent) (*models.Payout, error) {
	partnerRef := strings.TrimSpace(ev.PartnerReference)
	if partnerRef == "" {
		return nil, errors.New("partnerReferenceNo is required")
	}
	payout, err := s.repo.GetPayoutByPayoutID(ctx, partnerRef)
	if err != nil {
		return nil, err
	}

	payout.ProviderData = mergePayoutProviderData(payout.ProviderData, "callback", json.RawMessage(ev.RawPayload))
	if strings.TrimSpace(ev.ReferenceNo) != "" {
		payout.ProviderRef = stringPtr(ev.ReferenceNo)
	}

	switch strings.TrimSpace(ev.PaymentFlagStatus) {
	case "00":
		if payout.Status != models.PayoutStatusSuccess {
			now := time.Now()
			payout.Status = models.PayoutStatusSuccess
			payout.CompletedAt = &now
			payout.FailedAt = nil
			payout.FailedReason = nil
			payout.FailedCode = nil
		}
	case "06":
		if payout.Status != models.PayoutStatusFailed {
			now := time.Now()
			payout.Status = models.PayoutStatusFailed
			payout.FailedReason = stringPtr("Payout failed by provider")
			payout.FailedCode = stringPtr(ev.PaymentFlagStatus)
			payout.FailedAt = &now
			payout.CompletedAt = nil
		}
	default:
		// "03" pending or anything else — keep current state.
	}

	if err := s.repo.UpdatePayout(ctx, payout); err != nil {
		return payout, err
	}
	if payout.Status == models.PayoutStatusSuccess || payout.Status == models.PayoutStatusFailed {
		s.trySendFinalCallback(ctx, payout)
	}
	return payout, nil
}
