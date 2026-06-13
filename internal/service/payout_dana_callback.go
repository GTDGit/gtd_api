package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
)

// DanaCallbackEvent is the parsed DANA Transfer-to-Bank Notify (Service 43)
// delivered for previously-pending bank payouts.
type DanaCallbackEvent struct {
	PartnerReference string // originalPartnerReferenceNo -> payouts.payout_id
	ReferenceNo      string // originalReferenceNo
	Status           string // latestTransactionStatus (DANA DisbStatus code)
	StatusDesc       string
	RawPayload       []byte
}

// ApplyDanaCallback updates payout state from a DANA disbursement notify.
// Returns the matched payout (or nil) so the handler can decide on response.
func (s *PayoutService) ApplyDanaCallback(ctx context.Context, ev DanaCallbackEvent) (*models.Payout, error) {
	partnerRef := strings.TrimSpace(ev.PartnerReference)
	if partnerRef == "" {
		return nil, errors.New("originalPartnerReferenceNo is required")
	}
	payout, err := s.repo.GetPayoutByPayoutID(ctx, partnerRef)
	if err != nil {
		return nil, err
	}

	payout.ProviderData = mergePayoutProviderData(payout.ProviderData, "callback", json.RawMessage(ev.RawPayload))
	if strings.TrimSpace(ev.ReferenceNo) != "" {
		payout.ProviderRef = stringPtr(ev.ReferenceNo)
	}

	switch danaStatusFromCode(ev.Status) {
	case models.PayoutStatusSuccess:
		if payout.Status != models.PayoutStatusSuccess {
			now := time.Now()
			payout.Status = models.PayoutStatusSuccess
			payout.CompletedAt = &now
			payout.FailedAt = nil
			payout.FailedReason = nil
			payout.FailedCode = nil
		}
	case models.PayoutStatusFailed:
		if payout.Status != models.PayoutStatusFailed {
			now := time.Now()
			payout.Status = models.PayoutStatusFailed
			reason := strings.TrimSpace(ev.StatusDesc)
			if reason == "" {
				reason = "Payout failed by provider"
			}
			payout.FailedReason = stringPtr(reason)
			payout.FailedCode = stringPtr(strings.TrimSpace(ev.Status))
			payout.FailedAt = &now
			payout.CompletedAt = nil
		}
	default:
		// pending/processing — keep current state.
	}

	if err := s.repo.UpdatePayout(ctx, payout); err != nil {
		return payout, err
	}
	if payout.Status == models.PayoutStatusSuccess || payout.Status == models.PayoutStatusFailed {
		s.trySendFinalCallback(ctx, payout)
	}
	return payout, nil
}
