package service

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/GTDGit/gtd_api/pkg/bnc"
	"github.com/GTDGit/gtd_api/pkg/dana"
	"github.com/GTDGit/gtd_api/pkg/pakailink"
)

// snapErrorInfo is the provider-agnostic view of a SNAP API error, extracted
// from whichever concrete *APIError type a provider package returns.
type snapErrorInfo struct {
	HTTPStatus      int
	ResponseCode    string
	ResponseMessage string
	RawResponse     json.RawMessage
}

// extractSNAPError normalizes the various provider *APIError types into a
// single shape. ok=false means the error is not a recognized SNAP API error
// (e.g. a transport/timeout error).
func extractSNAPError(err error) (snapErrorInfo, bool) {
	var bncErr *bnc.APIError
	if errors.As(err, &bncErr) {
		return snapErrorInfo{bncErr.HTTPStatus, bncErr.ResponseCode, bncErr.ResponseMessage, bncErr.RawResponse}, true
	}
	var plErr *pakailink.APIError
	if errors.As(err, &plErr) {
		return snapErrorInfo{plErr.HTTPStatus, plErr.ResponseCode, plErr.ResponseMessage, plErr.RawResponse}, true
	}
	var danaErr *dana.APIError
	if errors.As(err, &danaErr) {
		return snapErrorInfo{danaErr.HTTPStatus, danaErr.ResponseCode, danaErr.ResponseMessage, danaErr.RawResponse}, true
	}
	return snapErrorInfo{}, false
}

// mapPayoutInquiryError converts a provider inquiry error into a typed payout
// error. Account-not-found is a 404 client error; provider/transport failures
// are marked retryable so the selector can fall through to the next provider.
func mapPayoutInquiryError(err error) *PayoutServiceError {
	info, ok := extractSNAPError(err)
	if !ok {
		return newRetryablePayoutError(503, "PROVIDER_UNAVAILABLE", "Payout provider is temporarily unavailable", err)
	}
	switch {
	case strings.HasPrefix(info.ResponseCode, "404"):
		return newPayoutError(404, "ACCOUNT_NOT_FOUND", "Account not found", err)
	case strings.HasPrefix(info.ResponseCode, "400"):
		return newPayoutError(400, "INVALID_REQUEST", nonEmptyOrDefault(info.ResponseMessage, "Invalid inquiry request"), err)
	case strings.HasPrefix(info.ResponseCode, "401"):
		return newRetryablePayoutError(503, "PROVIDER_UNAVAILABLE", "Payout provider unavailable", err)
	default:
		// 500/503 and unknown: retryable so we can try the next provider.
		return newRetryablePayoutError(503, "BANK_UNAVAILABLE", "Bank is temporarily unavailable", err)
	}
}

// mapPayoutSubmitError converts a provider submit error into a typed payout
// error. Definitive client rejections (insufficient balance, amount limits,
// inactive account) are non-retryable; uncertain/transport errors keep the
// payout pending rather than failing it.
func mapPayoutSubmitError(err error) *PayoutServiceError {
	info, ok := extractSNAPError(err)
	if !ok {
		return newRetryablePayoutError(503, "PROVIDER_UNAVAILABLE", "Payout provider is temporarily unavailable", err)
	}
	switch {
	case strings.HasPrefix(info.ResponseCode, "404"):
		return newPayoutError(404, "ACCOUNT_NOT_FOUND", "Account not found", err)
	case strings.Contains(strings.ToLower(info.ResponseMessage), "insufficient"):
		return newPayoutError(400, "INSUFFICIENT_BALANCE", "Disbursement balance is insufficient", err)
	case strings.HasPrefix(info.ResponseCode, "409"):
		return newPayoutError(400, "DUPLICATE_REFERENCE_ID", "Reference ID already exists", err)
	case strings.HasPrefix(info.ResponseCode, "400"), strings.HasPrefix(info.ResponseCode, "403"):
		return newPayoutError(400, "INVALID_REQUEST", nonEmptyOrDefault(info.ResponseMessage, "Invalid payout request"), err)
	default:
		// 5xx / unknown: retryable for provider fallback.
		return newRetryablePayoutError(503, "PROVIDER_UNAVAILABLE", "Payout provider unavailable", err)
	}
}

// isUncertainPayoutError reports whether a submit error left the payout in an
// indeterminate state (the provider may still process it). Such payouts are
// kept Pending and reconciled via status polling rather than failed.
func isUncertainPayoutError(err error) bool {
	info, ok := extractSNAPError(err)
	if !ok {
		return true // transport/timeout: cannot know if accepted
	}
	// 5xx responses to a submit are indeterminate.
	return strings.HasPrefix(info.ResponseCode, "500") ||
		strings.HasPrefix(info.ResponseCode, "504") ||
		info.HTTPStatus >= 500
}
