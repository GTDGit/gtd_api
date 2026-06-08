package ovo

import "encoding/json"

// APIError represents a non-2xx response from the OVO partner API.
type APIError struct {
	HTTPStatus   int             `json:"-"`
	ResponseCode string          `json:"responseCode,omitempty"`
	Message      string          `json:"responseMessage,omitempty"`
	RawResponse  json.RawMessage `json:"-"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.ResponseCode == "" {
		return e.Message
	}
	return e.ResponseCode + ": " + e.Message
}

// Transaction status strings returned by OVO.
// TODO(ovo-docs): confirm exact status vocabulary with OVO partner docs.
const (
	StatusPending = "PENDING"
	StatusSuccess = "SUCCESS"
	StatusFailed  = "FAILED"
	StatusExpired = "EXPIRED"
	StatusVoided  = "VOID"
)

// PushPaymentRequest initiates a push-to-pay transaction.
// TODO(ovo-docs): confirm field names against OVO partner docs.
type PushPaymentRequest struct {
	MerchantID         string `json:"merchantId"`
	AppID              string `json:"appId,omitempty"`
	PartnerReferenceNo string `json:"partnerReferenceNo"`
	// Phone is the customer's OVO-registered MSISDN (e.g. 08xxxxxxxxxx).
	Phone       string `json:"phone"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency,omitempty"`
	Description string `json:"description,omitempty"`
	// ExpiredAt is an RFC3339 timestamp; empty means provider default.
	ExpiredAt       string `json:"expiredAt,omitempty"`
	NotificationURL string `json:"notificationUrl,omitempty"`
}

// PushPaymentResponse is OVO's reply to a push-to-pay request.
type PushPaymentResponse struct {
	ResponseCode       string `json:"responseCode,omitempty"`
	ResponseMessage    string `json:"responseMessage,omitempty"`
	ReferenceNo        string `json:"referenceNo,omitempty"`
	PartnerReferenceNo string `json:"partnerReferenceNo,omitempty"`
	TransactionStatus  string `json:"transactionStatus,omitempty"`
	// Deeplink / mobile URL the customer can open to approve the push, when OVO
	// returns one instead of an in-app silent push.
	Deeplink    string          `json:"deeplink,omitempty"`
	CheckoutURL string          `json:"checkoutUrl,omitempty"`
	RawResponse json.RawMessage `json:"-"`
}

// StatusRequest queries the status of a transaction.
type StatusRequest struct {
	MerchantID         string `json:"merchantId"`
	PartnerReferenceNo string `json:"partnerReferenceNo"`
	ReferenceNo        string `json:"referenceNo,omitempty"`
}

// StatusResponse reports the latest known status of a transaction.
type StatusResponse struct {
	ResponseCode       string          `json:"responseCode,omitempty"`
	ResponseMessage    string          `json:"responseMessage,omitempty"`
	ReferenceNo        string          `json:"referenceNo,omitempty"`
	PartnerReferenceNo string          `json:"partnerReferenceNo,omitempty"`
	TransactionStatus  string          `json:"transactionStatus,omitempty"`
	Amount             int64           `json:"amount,omitempty"`
	RawResponse        json.RawMessage `json:"-"`
}

// VoidRequest cancels a pending transaction.
type VoidRequest struct {
	MerchantID         string `json:"merchantId"`
	PartnerReferenceNo string `json:"partnerReferenceNo"`
	ReferenceNo        string `json:"referenceNo,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

// VoidResponse reports the result of a void/cancel.
type VoidResponse struct {
	ResponseCode    string          `json:"responseCode,omitempty"`
	ResponseMessage string          `json:"responseMessage,omitempty"`
	RawResponse     json.RawMessage `json:"-"`
}

// WebhookPayload is the asynchronous notification OVO POSTs to the merchant on
// completion of a push-to-pay transaction.
// TODO(ovo-docs): confirm notification field names against OVO partner docs.
type WebhookPayload struct {
	ReferenceNo        string `json:"referenceNo,omitempty"`
	PartnerReferenceNo string `json:"partnerReferenceNo,omitempty"`
	TransactionStatus  string `json:"transactionStatus,omitempty"`
	Amount             int64  `json:"amount,omitempty"`
}
