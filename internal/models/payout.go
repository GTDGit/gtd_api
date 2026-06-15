package models

import "time"

// Payout (disbursement) domain types. The payout system mirrors the payment
// system: a request carries a payoutMethod {type, code}, an amount and a
// feePaidBy policy; routing picks a provider per method_type (BANK/EWALLET)
// with priority-ordered fallback.

// MethodType distinguishes a bank transfer from an e-wallet top-up.
type MethodType string

const (
	MethodTypeBank    MethodType = "BANK"
	MethodTypeEwallet MethodType = "EWALLET"
)

// TransferType is the internal bank routing hint (intrabank vs interbank). It
// is only meaningful for BANK payouts and drives provider service-code choice.
type TransferType string

const (
	TransferTypeIntrabank TransferType = "INTRABANK"
	TransferTypeInterbank TransferType = "INTERBANK"
)

// PayoutStatus is the lifecycle status persisted on a payout (payout_status
// enum: Processing/Success/Failed). It mirrors the payment lifecycle: a payout
// is created Processing and settles to Success or Failed.
type PayoutStatus string

const (
	PayoutStatusProcessing PayoutStatus = "Processing"
	PayoutStatusSuccess    PayoutStatus = "Success"
	PayoutStatusFailed     PayoutStatus = "Failed"
)

// DisbursementProvider identifies a payout provider (disbursement_provider enum).
type DisbursementProvider string

const (
	DisbursementProviderBCA       DisbursementProvider = "bca_direct"
	DisbursementProviderBRI       DisbursementProvider = "bri_direct"
	DisbursementProviderBNI       DisbursementProvider = "bni_direct"
	DisbursementProviderMandiri   DisbursementProvider = "mandiri_direct"
	DisbursementProviderBNC       DisbursementProvider = "bnc_direct"
	DisbursementProviderPakaiLink DisbursementProvider = "pakailink"
	DisbursementProviderDANA      DisbursementProvider = "dana_direct"
)

// FeePaidBy (defined in method_provider.go) is the fee-bearing policy:
//
//	merchant -> recipient gets the full amount; merchant is charged amount+fee.
//	customer -> recipient gets amount-fee;       merchant is charged amount.

// PayoutRoute binds a method_type to a provider with a priority for fallback.
type PayoutRoute struct {
	ID                 int                  `db:"id"`
	MethodType         MethodType           `db:"method_type"`
	Provider           DisbursementProvider `db:"provider"`
	Priority           int                  `db:"priority"`
	IsActive           bool                 `db:"is_active"`
	IsMaintenance      bool                 `db:"is_maintenance"`
	MaintenanceMessage *string              `db:"maintenance_message"`
	CreatedAt          time.Time            `db:"created_at"`
	UpdatedAt          time.Time            `db:"updated_at"`
}

// PayoutInquiry is a cached recipient validation (name + provider) created by
// the inquiry endpoint and consumed when a payout is executed.
type PayoutInquiry struct {
	ID            int                  `db:"id"`
	InquiryID     string               `db:"inquiry_id"`
	ClientID      int                  `db:"client_id"`
	IsSandbox     bool                 `db:"is_sandbox"`
	MethodType    MethodType           `db:"method_type"`
	ChannelCode   string               `db:"channel_code"` // bank code or e-wallet code
	BankCode      string               `db:"bank_code"`
	BankName      *string              `db:"bank_name"`
	AccountNumber string               `db:"account_number"`
	AccountName   *string              `db:"account_name"`
	TransferType  *TransferType        `db:"transfer_type"`
	Provider      DisbursementProvider `db:"provider"`
	ProviderRef   *string              `db:"provider_ref"`
	ProviderData  NullableRawMessage   `db:"provider_data"`
	ExpiredAt     time.Time            `db:"expired_at"`
	CreatedAt     time.Time            `db:"created_at"`
}

// Payout is a disbursement record.
type Payout struct {
	ID                  int                  `db:"id"`
	PayoutID            string               `db:"payout_id"`
	ReferenceID         string               `db:"reference_id"`
	ClientID            int                  `db:"client_id"`
	IsSandbox           bool                 `db:"is_sandbox"`
	MethodType          MethodType           `db:"method_type"`
	ChannelCode         string               `db:"channel_code"`
	TransferType        *TransferType        `db:"transfer_type"`
	Provider            DisbursementProvider `db:"provider"`
	BankCode            string               `db:"bank_code"`
	BankName            *string              `db:"bank_name"`
	AccountNumber       string               `db:"account_number"`
	AccountName         *string              `db:"account_name"`
	SourceBankCode      *string              `db:"source_bank_code"`
	SourceAccountNumber *string              `db:"source_account_number"`
	Amount              int64                `db:"amount"`       // recipient-facing payout amount
	Fee                 int64                `db:"fee"`          // provider/admin fee
	SendAmount          int64                `db:"send_amount"`  // value actually sent to the provider
	TotalAmount         int64                `db:"total_amount"` // amount debited from the merchant
	FeePaidBy           FeePaidBy            `db:"fee_paid_by"`
	Status              PayoutStatus         `db:"status"`
	FailedReason        *string              `db:"failed_reason"`
	FailedCode          *string              `db:"failed_code"`
	PurposeCode         *string              `db:"purpose_code"`
	Remark              *string              `db:"remark"`
	Description         *string              `db:"description"`
	CustomerName        *string              `db:"customer_name"`
	CustomerEmail       *string              `db:"customer_email"`
	CustomerPhone       *string              `db:"customer_phone"`
	InquiryRowID        *int                 `db:"inquiry_id"`
	ProviderRef         *string              `db:"provider_ref"`
	ProviderData        NullableRawMessage   `db:"provider_data"`
	CallbackURL         *string              `db:"callback_url"`
	CallbackSent        bool                 `db:"callback_sent"`
	CallbackSentAt      *time.Time           `db:"callback_sent_at"`
	CallbackAttempts    int                  `db:"callback_attempts"`
	CreatedAt           time.Time            `db:"created_at"`
	CompletedAt         *time.Time           `db:"completed_at"`
	FailedAt            *time.Time           `db:"failed_at"`
	UpdatedAt           time.Time            `db:"updated_at"`
}

// PayoutCallback records an inbound provider callback for a payout.
type PayoutCallback struct {
	ID               int                  `db:"id"`
	Provider         DisbursementProvider `db:"provider"`
	ProviderRef      *string              `db:"provider_ref"`
	Headers          NullableRawMessage   `db:"headers"`
	Payload          NullableRawMessage   `db:"payload"`
	Signature        *string              `db:"signature"`
	IsValidSignature bool                 `db:"is_valid_signature"`
	PayoutID         *string              `db:"payout_id"`
	Status           *string              `db:"status"`
	IsProcessed      bool                 `db:"is_processed"`
	ProcessedAt      *time.Time           `db:"processed_at"`
	ProcessError     *string              `db:"process_error"`
	CreatedAt        time.Time            `db:"created_at"`
}

// ----------------------------------------------------------------------------
// API request/response shapes (camelCase JSON), mirroring the payment system.
// ----------------------------------------------------------------------------

// PayoutMethodRef is the {type, code} selector shared by request and response,
// mirroring the payment PaymentMethodResponse ({type, code}).
type PayoutMethodRef struct {
	Type MethodType `json:"type"`
	Code string     `json:"code"`
}

// PayoutCustomer is the optional recipient descriptor.
type PayoutCustomer struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// PayoutURL carries the per-request callback URL (no return URL for payouts).
type PayoutURL struct {
	Callback string `json:"callback,omitempty"`
}

// PayoutAmount is the nested monetary breakdown, mirroring the payment
// AmountResponse ({subtotal, fee, total}).
type PayoutAmount struct {
	Subtotal int64 `json:"subtotal"` // recipient-facing payout amount
	Fee      int64 `json:"fee"`
	Total    int64 `json:"total"` // amount debited from the merchant
}

// PayoutInquiryResponse is returned by the inquiry endpoint.
type PayoutInquiryResponse struct {
	ID            string          `json:"id"`
	PayoutMethod  PayoutMethodRef `json:"payoutMethod"`
	AccountNumber string          `json:"accountNumber"`
	AccountName   string          `json:"accountName"`
	ExpiredAt     string          `json:"expiredAt"`
}

// PayoutResponse is the unified payout view returned by create/get endpoints.
// It mirrors PaymentResponse: id (UUID v4), nested amount, customer object.
type PayoutResponse struct {
	ID            string          `json:"id"`
	ReferenceID   string          `json:"referenceId"`
	PayoutMethod  PayoutMethodRef `json:"payoutMethod"`
	AccountNumber string          `json:"accountNumber"`
	AccountName   string          `json:"accountName"`
	Amount        PayoutAmount    `json:"amount"`
	FeePaidBy     string          `json:"feePaidBy"`
	Status        string          `json:"status"`
	Customer      *PayoutCustomer `json:"customer,omitempty"`
	Description   string          `json:"description,omitempty"`
	CreatedAt     string          `json:"createdAt"`
	CompletedAt   string          `json:"completedAt,omitempty"`
	FailedAt      string          `json:"failedAt,omitempty"`
	FailedReason  string          `json:"failedReason,omitempty"`
	FailedCode    string          `json:"failedCode,omitempty"`
}

// PayoutMethodCatalog is a row of the payout_methods catalog: a BANK/EWALLET
// channel with its name, fee config, and per-channel amount limits. It mirrors
// payment_methods and is the source of per-channel minimum payout amounts.
type PayoutMethodCatalog struct {
	ID                 int        `db:"id" json:"id"`
	MethodType         MethodType `db:"method_type" json:"methodType"`
	Code               string     `db:"code" json:"code"`
	Name               string     `db:"name" json:"name"`
	FeeType            string     `db:"fee_type" json:"feeType"`
	FeeFlat            int        `db:"fee_flat" json:"feeFlat"`
	FeePercent         float64    `db:"fee_percent" json:"feePercent"`
	FeeMin             int        `db:"fee_min" json:"feeMin"`
	FeeMax             int        `db:"fee_max" json:"feeMax"`
	MinAmount          int        `db:"min_amount" json:"minAmount"`
	MaxAmount          int        `db:"max_amount" json:"maxAmount"`
	LogoURL            *string    `db:"logo_url" json:"logoUrl,omitempty"`
	DisplayOrder       int        `db:"display_order" json:"displayOrder"`
	IsActive           bool       `db:"is_active" json:"isActive"`
	IsMaintenance      bool       `db:"is_maintenance" json:"isMaintenance"`
	MaintenanceMessage *string    `db:"maintenance_message" json:"maintenanceMessage,omitempty"`
	CreatedAt          time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt          time.Time  `db:"updated_at" json:"updatedAt"`
}

// PayoutMethodEntry is a single channel on the public list endpoint.
type PayoutMethodEntry struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	FeeType       string `json:"feeType"`
	FeeFlat       int    `json:"feeFlat"`
	FeePercent    float64 `json:"feePercent"`
	FeeMin        int    `json:"feeMin"`
	FeeMax        int    `json:"feeMax"`
	MinAmount     int    `json:"minAmount"`
	MaxAmount     int    `json:"maxAmount"`
	LogoURL       string `json:"logoUrl,omitempty"`
	IsMaintenance bool   `json:"isMaintenance"`
}

// PayoutMethodsResponse groups available payout channels for the list endpoint.
type PayoutMethodsResponse struct {
	Bank    []PayoutMethodEntry `json:"bank"`
	Ewallet []PayoutMethodEntry `json:"ewallet"`
}
