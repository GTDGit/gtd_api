package xendit

import "encoding/json"

type APIError struct {
	HTTPStatus int
	ErrorCode  string `json:"error_code"`
	Message    string `json:"message"`
	Errors     []any  `json:"errors,omitempty"`
	RawResponse json.RawMessage
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.ErrorCode == "" {
		return e.Message
	}
	return e.ErrorCode + ": " + e.Message
}

const (
	ChannelAlfamart  = "ALFAMART"
	ChannelIndomaret = "INDOMARET"

	StatusAccepting = "ACCEPTING_PAYMENTS"
	StatusSucceeded = "SUCCEEDED"
	StatusExpired   = "EXPIRED"
	StatusFailed    = "FAILED"
	StatusCanceled  = "CANCELED"

	EventCapture  = "payment.capture"
	EventFailure  = "payment.failure"
	EventAuthorization = "payment.authorization"
)

type PaymentRequestChannelProperties struct {
	PayerName   string `json:"payer_name,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	PaymentCode string `json:"payment_code,omitempty"`
	// QRIS-specific fields returned by Xendit
	QRString   string `json:"qr_string,omitempty"`
	QRImageURL string `json:"qr_image_url,omitempty"`
	// Ewallet-specific fields
	AccountMobileNumber string `json:"account_mobile_number,omitempty"` // OVO requires phone number (format 62xxx)
	SuccessReturnURL    string `json:"success_return_url,omitempty"`    // Redirect URL on success
	FailureReturnURL    string `json:"failure_return_url,omitempty"`    // Redirect URL on failure
	CancelReturnURL     string `json:"cancel_return_url,omitempty"`     // Redirect URL on cancel
}

type PaymentRequestCreate struct {
	ReferenceID       string                          `json:"reference_id"`
	Type              string                          `json:"type"`
	Country           string                          `json:"country"`
	Currency          string                          `json:"currency"`
	ChannelCode       string                          `json:"channel_code"`
	RequestAmount     int64                           `json:"request_amount"`
	ChannelProperties PaymentRequestChannelProperties `json:"channel_properties"`
	CaptureMethod     string                          `json:"capture_method,omitempty"`
	Description       string                          `json:"description,omitempty"`
	Metadata          map[string]any                  `json:"metadata,omitempty"`
}

type PaymentRequest struct {
	BusinessID        string                          `json:"business_id"`
	ReferenceID       string                          `json:"reference_id"`
	PaymentRequestID  string                          `json:"payment_request_id"`
	LatestPaymentID   string                          `json:"latest_payment_id,omitempty"`
	Type              string                          `json:"type"`
	Country           string                          `json:"country"`
	Currency          string                          `json:"currency"`
	RequestAmount     int64                           `json:"request_amount"`
	CaptureMethod     string                          `json:"capture_method"`
	ChannelCode       string                          `json:"channel_code"`
	ChannelProperties PaymentRequestChannelProperties `json:"channel_properties"`
	Status            string                          `json:"status"`
	Description       string                          `json:"description"`
	Created           string                          `json:"created"`
	Updated           string                          `json:"updated"`
	FailureCode       string                          `json:"failure_code,omitempty"`
	Actions           []any                           `json:"actions,omitempty"`
	RawResponse       json.RawMessage                 `json:"-"`
}

type RefundCreate struct {
	Amount      int64          `json:"amount"`
	ReferenceID string         `json:"reference_id,omitempty"`
	Reason      string         `json:"reason"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type Refund struct {
	RefundID         string          `json:"refund_id"`
	PaymentRequestID string          `json:"payment_request_id,omitempty"`
	PaymentID        string          `json:"payment_id,omitempty"`
	ReferenceID      string          `json:"reference_id,omitempty"`
	Currency         string          `json:"currency"`
	Amount           int64           `json:"amount"`
	Status           string          `json:"status"`
	Reason           string          `json:"reason"`
	Created          string          `json:"created"`
	Updated          string          `json:"updated"`
	RawResponse      json.RawMessage `json:"-"`
}

type WebhookPayload struct {
	Event      string         `json:"event"`
	BusinessID string         `json:"business_id"`
	Created    string         `json:"created"`
	Data       WebhookData    `json:"data"`
	RawBody    []byte         `json:"-"`
}

type WebhookData struct {
	PaymentID        string         `json:"payment_id"`
	PaymentRequestID string         `json:"payment_request_id"`
	ReferenceID      string         `json:"reference_id"`
	Status           string         `json:"status"`
	RequestAmount    int64          `json:"request_amount"`
	ChannelCode      string         `json:"channel_code"`
	Country          string         `json:"country"`
	Currency         string         `json:"currency"`
	Description      string         `json:"description,omitempty"`
	FailureCode      string         `json:"failure_code,omitempty"`
	Created          string         `json:"created"`
	Updated          string         `json:"updated"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// ---------------------------------------------------------------------------
// Legacy Virtual Account API (/callback_virtual_accounts).
//
// This account serves VA through the legacy Fixed-VA endpoint, not the v3
// /v3/payment_requests flow. Closed single-use VAs are created with an
// expected_amount and expiration; Xendit POSTs a FixedVirtualAccountPaid
// webhook (no event/status field — its arrival means PAID) keyed by external_id.
// ---------------------------------------------------------------------------

// VirtualAccountCreate is the request body for POST /callback_virtual_accounts.
type VirtualAccountCreate struct {
	ExternalID     string `json:"external_id"`
	BankCode       string `json:"bank_code"`
	Name           string `json:"name"`
	IsClosed       bool   `json:"is_closed"`
	IsSingleUse    bool   `json:"is_single_use"`
	ExpectedAmount int64  `json:"expected_amount,omitempty"`
	ExpirationDate string `json:"expiration_date,omitempty"` // ISO-8601 UTC
}

// VirtualAccount is the legacy Fixed-VA object returned by create/get.
// status is a lifecycle value (PENDING/ACTIVE/INACTIVE) — NOT a payment status.
type VirtualAccount struct {
	ID             string          `json:"id"`
	OwnerID        string          `json:"owner_id"`
	ExternalID     string          `json:"external_id"`
	MerchantCode   string          `json:"merchant_code"`
	AccountNumber  string          `json:"account_number"`
	BankCode       string          `json:"bank_code"`
	Name           string          `json:"name"`
	IsClosed       bool            `json:"is_closed"`
	IsSingleUse    bool            `json:"is_single_use"`
	ExpectedAmount int64           `json:"expected_amount"`
	ExpirationDate string          `json:"expiration_date"`
	Status         string          `json:"status"`
	Currency       string          `json:"currency"`
	Country        string          `json:"country"`
	RawResponse    json.RawMessage `json:"-"`
}

// VirtualAccountPaidWebhook models the Fixed-VA payment notification. The
// arrival of this payload means the VA was paid; there is no status field.
type VirtualAccountPaidWebhook struct {
	ID                         string `json:"id"`                  // payment id
	PaymentID                  string `json:"payment_id"`          // bank-side payment id
	CallbackVirtualAccountID   string `json:"callback_virtual_account_id"`
	ExternalID                 string `json:"external_id"`         // our PaymentID
	MerchantCode               string `json:"merchant_code"`
	AccountNumber              string `json:"account_number"`
	BankCode                   string `json:"bank_code"`
	Amount                     int64  `json:"amount"`
	TransactionTimestamp       string `json:"transaction_timestamp"`
}
