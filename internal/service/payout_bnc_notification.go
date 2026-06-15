package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
)

// BNCTransferNotificationAmount is the amount object in a BNC transfer notify.
type BNCTransferNotificationAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

// BNCTransferNotification is the inbound BNC transfer-result notification.
type BNCTransferNotification struct {
	ResponseCode               string                        `json:"responseCode"`
	ResponseMessage            string                        `json:"responseMessage"`
	OriginalReferenceNo        string                        `json:"originalReferenceNo"`
	OriginalPartnerReferenceNo string                        `json:"originalPartnerReferenceNo"`
	ReferenceNo                string                        `json:"referenceNo"`
	PartnerReferenceNo         string                        `json:"partnerReferenceNo"`
	ServiceCode                string                        `json:"serviceCode"`
	TransactionDate            string                        `json:"transactionDate"`
	FinishedTime               string                        `json:"finishedTime"`
	BeneficiaryAccountNo       string                        `json:"beneficiaryAccountNo"`
	BeneficiaryBankCode        string                        `json:"beneficiaryBankCode"`
	SourceAccountNo            string                        `json:"sourceAccountNo"`
	LatestTransactionStatus    string                        `json:"latestTransactionStatus"`
	TransactionStatusDesc      string                        `json:"transactionStatusDesc"`
	Amount                     BNCTransferNotificationAmount `json:"amount"`
}

func (n *BNCTransferNotification) PayoutID() string {
	return firstNonEmptyString(n.OriginalPartnerReferenceNo, n.PartnerReferenceNo)
}

func (n *BNCTransferNotification) ProviderRef() string {
	return firstNonEmptyString(n.OriginalReferenceNo, n.ReferenceNo)
}

// ApplyBNCNotification updates payout state from an inbound BNC transfer notify.
func (s *PayoutService) ApplyBNCNotification(ctx context.Context, notification *BNCTransferNotification, rawPayload json.RawMessage) error {
	if notification == nil {
		return newPayoutError(400, "INVALID_NOTIFICATION", "Invalid transfer notification", nil)
	}
	payoutID := strings.TrimSpace(notification.PayoutID())
	if payoutID == "" {
		return newPayoutError(400, "INVALID_NOTIFICATION", "Missing payout identifier", nil)
	}

	payout, err := s.repo.GetPayoutByPayoutID(ctx, payoutID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return newPayoutError(404, "PAYOUT_NOT_FOUND", "Payout not found", err)
		}
		return err
	}

	if providerRef := strings.TrimSpace(notification.ProviderRef()); providerRef != "" {
		payout.ProviderRef = stringPtr(providerRef)
	}
	payout.ProviderData = mergePayoutProviderData(payout.ProviderData, "callback", rawPayload)

	statusCode := strings.ToUpper(strings.TrimSpace(notification.LatestTransactionStatus))
	finalTime := parseBNCNotificationTime(notification.FinishedTime, notification.TransactionDate)

	switch statusCode {
	case "00", "SUCCESS":
		payout.Status = models.PayoutStatusSuccess
		payout.CompletedAt = &finalTime
		payout.FailedAt = nil
		payout.FailedReason = nil
		payout.FailedCode = nil
	case "06", "05", "FAILED", "CANCELLED":
		payout.Status = models.PayoutStatusFailed
		payout.CompletedAt = nil
		payout.FailedAt = &finalTime
		payout.FailedReason = stringPtr(nonEmptyOrDefault(notification.TransactionStatusDesc, "Payout failed"))
		payout.FailedCode = stringPtr(nonEmptyOrDefault(notification.LatestTransactionStatus, notification.ResponseCode))
	case "03", "02", "01", "PENDING", "PAYING", "INITIATED":
		payout.Status = models.PayoutStatusProcessing
	default:
		payout.Status = models.PayoutStatusProcessing
	}

	if err := s.repo.UpdatePayout(ctx, payout); err != nil {
		return err
	}
	if payout.Status == models.PayoutStatusSuccess || payout.Status == models.PayoutStatusFailed {
		s.trySendFinalCallback(ctx, payout)
	}
	return nil
}

func parseBNCNotificationTime(values ...string) time.Time {
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			return ts
		}
	}
	return time.Now()
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
