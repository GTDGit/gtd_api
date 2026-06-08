package models

import "time"

// ----------------------------------------------------------------------------
// Fee bearer — who pays the transaction fee.
//
// Promoted from the legacy metadata._feePaidBy key to a first-class column on
// the payments table (see migration 000050_add_fee_paid_by). The bearer drives
// amount.total: merchant -> total = subtotal, customer -> total = subtotal+fee.
// ----------------------------------------------------------------------------

type FeePaidBy string

const (
	FeePaidByMerchant FeePaidBy = "merchant"
	FeePaidByCustomer FeePaidBy = "customer"
)

// ----------------------------------------------------------------------------
// MethodProviderBinding mirrors the payment_method_providers table — the
// Method_Provider_Mapping that binds one canonical payment method to one
// provider with an explicit priority and health flags (see migration
// 000051_add_payment_method_providers).
// ----------------------------------------------------------------------------

type MethodProviderBinding struct {
	ID                 int             `db:"id" json:"id"`
	PaymentMethodID    int             `db:"payment_method_id" json:"paymentMethodId"`
	Provider           PaymentProvider `db:"provider" json:"provider"`
	Priority           int             `db:"priority" json:"priority"`
	IsActive           bool            `db:"is_active" json:"isActive"`
	IsMaintenance      bool            `db:"is_maintenance" json:"isMaintenance"`
	MaintenanceMessage *string         `db:"maintenance_message" json:"maintenanceMessage,omitempty"`
	ProviderBankCode   *string         `db:"provider_bank_code" json:"providerBankCode,omitempty"`
	ProviderChannel    *string         `db:"provider_channel" json:"providerChannel,omitempty"`
	CreatedAt          time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt          time.Time       `db:"updated_at" json:"updatedAt"`
}

// ----------------------------------------------------------------------------
// MethodGroup is the logical payment method (type + code) plus its ordered
// providers. It is the unit returned by ProviderSelector.Resolve and exposed
// (de-duplicated by type+code) on the method list endpoint.
// ----------------------------------------------------------------------------

type MethodGroup struct {
	Type      PaymentType             `json:"type"`
	Code      string                  `json:"code"`
	Display   PaymentMethodDisplay    `json:"display"`
	Providers []MethodProviderBinding `json:"providers"`
}

// PaymentMethodDisplay holds the canonical, provider-agnostic presentation and
// limit/fee data for a payment method (one per type+code).
type PaymentMethodDisplay struct {
	Name               string             `json:"name"`
	LogoURL            *string            `json:"logoUrl,omitempty"`
	FeeType            FeeType            `json:"feeType"`
	FeeFlat            int                `json:"feeFlat"`
	FeePercent         float64            `json:"feePercent"`
	FeeMin             int                `json:"feeMin"`
	FeeMax             int                `json:"feeMax"`
	MinAmount          int                `json:"minAmount"`
	MaxAmount          int                `json:"maxAmount"`
	ExpiredDuration    int                `json:"expiredDuration"`
	DisplayOrder       int                `json:"displayOrder"`
	PaymentInstruction NullableRawMessage `json:"paymentInstruction,omitempty"`
}
