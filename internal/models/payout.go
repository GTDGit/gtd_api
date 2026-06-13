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

// PayoutStatus is the lifecycle status persisted on a payout (transfer_status
// enum: Processing/Success/Pending/Failed).
type PayoutStatus string

const (
	PayoutStatusProcessing PayoutStatus = "Processing"
	PayoutStatusSuccess    PayoutStatus = "Success"
	PayoutStatusPending    PayoutStatus = "Pending"
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

// PayoutMethod is the {type, code} selector shared by request and response.
type PayoutMethod struct {
	Type MethodType `json:"type"`
	Code string     `json:"code"`
	Name string     `json:"name,omitempty"`
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

// PayoutInquiryResponse is returned by the inquiry endpoint.
type PayoutInquiryResponse struct {
	InquiryID     string       `json:"inquiryId"`
	PayoutMethod  PayoutMethod `json:"payoutMethod"`
	BankName      string       `json:"bankName,omitempty"`
	AccountNumber string       `json:"accountNumber"`
	AccountName   string       `json:"accountName"`
	ExpiredAt     string       `json:"expiredAt"`
}

// PayoutResponse is the unified payout view returned by create/get endpoints.
type PayoutResponse struct {
	PayoutID      string          `json:"payoutId"`
	ReferenceID   string          `json:"referenceId"`
	Status        string          `json:"status"`
	PayoutMethod  PayoutMethod    `json:"payoutMethod"`
	AccountNumber string          `json:"accountNumber"`
	AccountName   string          `json:"accountName"`
	Amount        int64           `json:"amount"`
	Fee           int64           `json:"fee"`
	TotalAmount   int64           `json:"totalAmount"`
	FeePaidBy     string          `json:"feePaidBy"`
	Customer      *PayoutCustomer `json:"customer,omitempty"`
	Description   string          `json:"description,omitempty"`
	ProviderRef   string          `json:"providerRef,omitempty"`
	CreatedAt     string          `json:"createdAt"`
	CompletedAt   string          `json:"completedAt,omitempty"`
	FailedAt      string          `json:"failedAt,omitempty"`
	FailedReason  string          `json:"failedReason,omitempty"`
	FailedCode    string          `json:"failedCode,omitempty"`
}
