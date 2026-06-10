package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
)

// PaymentCreateRequest is the unified provider-agnostic create payload.
type PaymentCreateRequest struct {
	Type           models.PaymentType
	Code           string
	BankCode       string // VA bank code, e.g. "014", "451"
	PartnerRef     string // public PaymentID (UUID)
	Amount         int64
	Fee            int64
	TotalAmount    int64
	ExpiredAt      time.Time
	Description    string
	ClientName     string // owning client's name; VA name fallback when customer.name is empty
	CustomerName   string
	CustomerEmail  string
	CustomerPhone  string
	ReturnURL      string
	ScanData       string // CPM QRIS: QR code content scanned from customer's app
	Metadata       map[string]any
}

// PaymentDetailNormalized is the union shape copied into payment.payment_detail.
//
// Each payment type populates only the fields from its own group; every field
// is tagged omitempty so irrelevant fields are omitted from the serialized
// paymentDetail. The field names are shared across all provider adapters so
// the same type produces an identical field set regardless of provider
// (Req 4.5, 4.8). Allowed field sets per type:
//
//	VA      -> bankCode, bankName, vaNumber, accountName, billerCode
//	EWALLET -> checkoutUrl, mobileWebUrl, deeplink, qrCodeUrl
//	QRIS    -> qrString, qrImageUrl
//	RETAIL  -> retailName, paymentCode
type PaymentDetailNormalized struct {
	// VA
	BankCode    string `json:"bankCode,omitempty"`
	BankName    string `json:"bankName,omitempty"`
	VANumber    string `json:"vaNumber,omitempty"`
	AccountName string `json:"accountName,omitempty"`
	// BillerCode is set only for Mandiri Bill Payment (echannel), where the
	// customer pays using biller code + bill key (carried in VANumber).
	BillerCode string `json:"billerCode,omitempty"`
	// EWALLET
	CheckoutURL  string `json:"checkoutUrl,omitempty"`
	MobileWebURL string `json:"mobileWebUrl,omitempty"`
	Deeplink     string `json:"deeplink,omitempty"`
	QRCodeURL    string `json:"qrCodeUrl,omitempty"`
	// QRIS
	QRString   string `json:"qrString,omitempty"`
	QRImageURL string `json:"qrImageUrl,omitempty"`
	// RETAIL
	RetailName  string `json:"retailName,omitempty"`
	PaymentCode string `json:"paymentCode,omitempty"`
}

// PaymentCreateResponse is returned by all provider adapters.
type PaymentCreateResponse struct {
	ProviderRef string
	Normalized  PaymentDetailNormalized
	RawResponse json.RawMessage
}

// PaymentInquiryResult reports the latest known status from a provider.
type PaymentInquiryResult struct {
	Status      models.PaymentStatus
	ProviderRef string
	PaidAmount  int64
	RawResponse json.RawMessage
}

// PaymentCancelResult reports the outcome of a cancel call.
type PaymentCancelResult struct {
	Cancelled   bool
	RawResponse json.RawMessage
}

// PaymentRefundResult reports the outcome of a refund call.
type PaymentRefundResult struct {
	ProviderRef string
	Succeeded   bool
	RawResponse json.RawMessage
}

// PaymentProviderClient is implemented by provider-specific adapters.
type PaymentProviderClient interface {
	Code() models.PaymentProvider
	// Available reports whether the adapter has the configuration/credentials
	// required to serve requests. Used by ProviderSelector for fallback.
	Available() bool
	CreatePayment(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error)
	InquiryPayment(ctx context.Context, payment *models.Payment) (*PaymentInquiryResult, error)
	CancelPayment(ctx context.Context, payment *models.Payment, reason string) (*PaymentCancelResult, error)
}
