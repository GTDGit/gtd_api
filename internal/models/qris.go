package models

import "time"

// ----------------------------------------------------------------------------
// Static QRIS merchant registry + payments (migration 000064).
// ----------------------------------------------------------------------------

type QRISProvider string

const (
	QRISProviderPakailink QRISProvider = "pakailink"
	QRISProviderNobu      QRISProvider = "nobu"
)

type QRISMerchantStatus string

const (
	QRISMerchantActive   QRISMerchantStatus = "active"
	QRISMerchantInactive QRISMerchantStatus = "inactive"
)

// QRISMerchant mirrors the qris_merchants table. store_id is entered manually
// (it identifies the merchant on inbound webhooks); the descriptive fields are
// parsed from qris_string when present.
type QRISMerchant struct {
	ID                   int                `db:"id" json:"id"`
	ClientID             *int               `db:"client_id" json:"clientId,omitempty"`
	Provider             QRISProvider       `db:"provider" json:"provider"`
	MerchantName         *string            `db:"merchant_name" json:"merchantName,omitempty"`
	MerchantCity         *string            `db:"merchant_city" json:"merchantCity,omitempty"`
	MerchantCategoryCode *string            `db:"merchant_category_code" json:"merchantCategoryCode,omitempty"`
	NMID                 *string            `db:"nmid" json:"nmid,omitempty"`
	StoreID              string             `db:"store_id" json:"storeId"`
	TerminalID           *string            `db:"terminal_id" json:"terminalId,omitempty"`
	QRISString           *string            `db:"qris_string" json:"qrisString,omitempty"`
	Status               QRISMerchantStatus `db:"status" json:"status"`
	RawProviderResponse  NullableRawMessage `db:"raw_provider_response" json:"rawProviderResponse,omitempty"`
	CreatedAt            time.Time          `db:"created_at" json:"createdAt"`
	UpdatedAt            time.Time          `db:"updated_at" json:"updatedAt"`
}

// QRISPayment mirrors the qris_payments table — a successful QRIS payment
// delivered by a provider webhook. Idempotency keys on (provider, reference_no).
type QRISPayment struct {
	ID                 int                `db:"id" json:"id"`
	QRISMerchantID     *int               `db:"qris_merchant_id" json:"qrisMerchantId,omitempty"`
	Provider           QRISProvider       `db:"provider" json:"provider"`
	ReferenceNo        string             `db:"reference_no" json:"referenceNo"`
	PartnerReferenceNo *string            `db:"partner_reference_no" json:"partnerReferenceNo,omitempty"`
	RRN                *string            `db:"rrn" json:"rrn,omitempty"`
	PaymentReferenceNo *string            `db:"payment_reference_no" json:"paymentReferenceNo,omitempty"`
	IssuerID           *string            `db:"issuer_id" json:"issuerId,omitempty"`
	StoreID            string             `db:"store_id" json:"storeId"`
	TerminalID         *string            `db:"terminal_id" json:"terminalId,omitempty"`
	Amount             int64              `db:"amount" json:"amount"`
	FeeAmount          *int64             `db:"fee_amount" json:"feeAmount,omitempty"`
	NettAmount         *int64             `db:"nett_amount" json:"nettAmount,omitempty"`
	PayerName          *string            `db:"payer_name" json:"payerName,omitempty"`
	PayerPhone         *string            `db:"payer_phone" json:"payerPhone,omitempty"`
	Status             string             `db:"status" json:"status"`
	PaidAt             *time.Time         `db:"paid_at" json:"paidAt,omitempty"`
	RawPayload         NullableRawMessage `db:"raw_payload" json:"rawPayload,omitempty"`
	CreatedAt          time.Time          `db:"created_at" json:"createdAt"`
}
