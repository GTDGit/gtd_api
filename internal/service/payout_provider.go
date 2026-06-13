package service

import (
	"context"
	"encoding/json"

	"github.com/GTDGit/gtd_api/internal/models"
)

// ----------------------------------------------------------------------------
// PayoutProviderClient is the unified provider-agnostic adapter contract for
// disbursement. It mirrors the payment system's PaymentProviderClient: the
// router holds one adapter per provider, and the selector picks a healthy
// provider per method_type (BANK/EWALLET) with priority-ordered fallback.
//
// Capability is decided by the adapter (Supports), not by the routing table, so
// e.g. dana_direct can advertise EWALLET support only for the DANA wallet while
// still serving BANK transfers, and the selector skips providers that cannot
// serve a given channel.
// ----------------------------------------------------------------------------

// PayoutInquiryInput is the provider-agnostic recipient validation request.
type PayoutInquiryInput struct {
	PartnerRef    string // public inquiry id / partner reference
	MethodType    models.MethodType
	ChannelCode   string // bank code (BANK) or e-wallet code (EWALLET)
	AccountNumber string
	Amount        int64
}

// PayoutInquiryOutput carries the validated recipient name and routing hints.
type PayoutInquiryOutput struct {
	AccountName  string
	BankName     string
	ProviderRef  string
	TransferType models.TransferType // bank routing hint; empty for e-wallet
	Fee          int64               // provider-reported fee, 0 when unknown
	RawResponse  json.RawMessage
}

// PayoutExecInput is the provider-agnostic disbursement request.
type PayoutExecInput struct {
	PartnerRef    string // public payout id
	MethodType    models.MethodType
	ChannelCode   string
	AccountNumber string
	AccountName   string
	Amount        int64 // value to send to the provider (already fee-adjusted)
	TransferType  models.TransferType
	Purpose       string
	Remark        string
	CallbackURL   string
}

// PayoutExecOutput is the provider response to a disbursement submission.
type PayoutExecOutput struct {
	ProviderRef  string
	Status       models.PayoutStatus // best-effort immediate status
	Fee          int64               // provider-reported fee, 0 when unknown
	RawResponse  json.RawMessage
}

// PayoutStatusInput requests the latest status of a submitted payout.
type PayoutStatusInput struct {
	PartnerRef   string
	ProviderRef  string
	MethodType   models.MethodType
	TransferType models.TransferType
}

// PayoutStatusOutput reports the latest known provider status.
type PayoutStatusOutput struct {
	Status       models.PayoutStatus
	ProviderRef  string
	FailedReason string
	FailedCode   string
	RawResponse  json.RawMessage
}

// PayoutProviderClient is implemented by provider-specific adapters.
type PayoutProviderClient interface {
	Code() models.DisbursementProvider
	// Available reports whether the adapter has the configuration/credentials
	// required to serve requests. Used by the selector for health checks.
	Available() bool
	// Supports reports whether this provider can serve the given method_type and
	// channel (bank code or e-wallet code). Drives capability-based fallback.
	Supports(mt models.MethodType, channelCode string) bool
	// SourceAccount returns the (bankCode, accountNo) debited for this payout,
	// for persistence/audit. Either may be empty (e.g. e-wallet balance).
	SourceAccount(mt models.MethodType) (bankCode string, accountNo string)
	Inquiry(ctx context.Context, in *PayoutInquiryInput) (*PayoutInquiryOutput, error)
	Pay(ctx context.Context, in *PayoutExecInput) (*PayoutExecOutput, error)
	Status(ctx context.Context, in *PayoutStatusInput) (*PayoutStatusOutput, error)
}

// PayoutServiceError is the typed error returned across the payout service.
type PayoutServiceError struct {
	HTTPStatus int
	Code       string
	Message    string
	// Retryable marks provider failures where falling through to the next
	// provider in the routing table is safe (the payout was not accepted).
	Retryable bool
	Err       error
}

func (e *PayoutServiceError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Code + ": " + e.Message
	}
	return e.Code + ": " + e.Message + ": " + e.Err.Error()
}

func (e *PayoutServiceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newPayoutError(httpStatus int, code, message string, err error) *PayoutServiceError {
	return &PayoutServiceError{HTTPStatus: httpStatus, Code: code, Message: message, Err: err}
}

func newRetryablePayoutError(httpStatus int, code, message string, err error) *PayoutServiceError {
	return &PayoutServiceError{HTTPStatus: httpStatus, Code: code, Message: message, Retryable: true, Err: err}
}
