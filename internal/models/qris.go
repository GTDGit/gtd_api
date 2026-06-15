package models

import "time"

// ----------------------------------------------------------------------------
// Static QRIS merchant registry + payments (migration 000064).
// ----------------------------------------------------------------------------

type QRISProvider string

const (
	QRISProviderNobu QRISProvider = "nobu"
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
	SubMerchantID        *string            `db:"sub_merchant_id" json:"subMerchantId,omitempty"`  // Nobu MID (000067)
	RegistrationID       *int               `db:"registration_id" json:"registrationId,omitempty"` // back-link (000067)
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

// ----------------------------------------------------------------------------
// QRIS document portal (migration 000065). A bundle is one shareable link for
// one merchant's onboarding documents; it owns N files. Each file lives in
// private object storage and is only ever streamed through a token-validating
// handler. doc_type values: ktp | selfie_ktp | business_location | extra.
// ----------------------------------------------------------------------------

type QRISDocType string

const (
	QRISDocKTP              QRISDocType = "ktp"
	QRISDocSelfieKTP        QRISDocType = "selfie_ktp"
	QRISDocBusinessLocation QRISDocType = "business_location"
	QRISDocExtra            QRISDocType = "extra"
)

type QRISDocBundleStatus string

const (
	QRISDocBundleActive  QRISDocBundleStatus = "active"
	QRISDocBundleRevoked QRISDocBundleStatus = "revoked"
)

// QRISDocBundle mirrors qris_doc_bundles.
type QRISDocBundle struct {
	ID             int                 `db:"id" json:"id"`
	Token          string              `db:"token" json:"token"`
	MerchantName   string              `db:"merchant_name" json:"merchantName"`
	QRISMerchantID *int                `db:"qris_merchant_id" json:"qrisMerchantId,omitempty"`
	Status         QRISDocBundleStatus `db:"status" json:"status"`
	Note           *string             `db:"note" json:"note,omitempty"`
	CreatedBy      *string             `db:"created_by" json:"createdBy,omitempty"`
	ConfirmedAt    *time.Time          `db:"confirmed_at" json:"confirmedAt,omitempty"`
	ExpiresAt      *time.Time          `db:"expires_at" json:"expiresAt,omitempty"`
	CreatedAt      time.Time           `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time           `db:"updated_at" json:"updatedAt"`
}

// QRISDocFile mirrors qris_doc_files.
type QRISDocFile struct {
	ID          int         `db:"id" json:"id"`
	BundleID    int         `db:"bundle_id" json:"bundleId"`
	Token       string      `db:"token" json:"token"`
	DocType     QRISDocType `db:"doc_type" json:"docType"`
	FileName    string      `db:"file_name" json:"fileName"`
	ContentType string      `db:"content_type" json:"contentType"`
	SizeBytes   int64       `db:"size_bytes" json:"sizeBytes"`
	StorageKey  string      `db:"storage_key" json:"-"` // private key; never exposed
	Checksum    *string     `db:"checksum" json:"checksum,omitempty"`
	CreatedAt   time.Time   `db:"created_at" json:"createdAt"`
}

// ----------------------------------------------------------------------------
// QRIS Nobu registration intake + Excel batch (migration 000067).
// ----------------------------------------------------------------------------

type QRISRegistrationStatus string

const (
	QRISRegPendingBatch QRISRegistrationStatus = "pending_batch" // awaiting inclusion in a Nobu Excel batch
	QRISRegSubmitted    QRISRegistrationStatus = "submitted"     // included in a batch sent to Nobu
	QRISRegActivated    QRISRegistrationStatus = "activated"     // Nobu provisioned; merchant created
	QRISRegRejected     QRISRegistrationStatus = "rejected"      // Nobu declined
)

// QRISType is the kind of QR a merchant requests. Exposed to clients as the
// English enum static|dynamic|both (the Nobu Excel form uses its own labels,
// mapped at batch-render time).
type QRISType string

const (
	QRISTypeStatic  QRISType = "static"
	QRISTypeDynamic QRISType = "dynamic"
	QRISTypeBoth    QRISType = "both"
)

// Valid reports whether t is one of the supported QRIS types.
func (t QRISType) Valid() bool {
	switch t {
	case QRISTypeStatic, QRISTypeDynamic, QRISTypeBoth:
		return true
	default:
		return false
	}
}

// QRISRegistration mirrors qris_registrations — a client's request to onboard a
// static-QRIS merchant. Nobu has no register API, so every field of the Nobu
// Excel form is captured here for batch rendering.
type QRISRegistration struct {
	ID              int    `db:"id" json:"-"`                              // internal SERIAL; never exposed
	RegistrationID  string `db:"registration_id" json:"id"`               // public UUID v4 (client-facing `id`)
	ClientID        *int   `db:"client_id" json:"clientId,omitempty"`
	RegistrationRef string `db:"registration_ref" json:"referenceId"`     // client idempotency key

	OwnerFullName string `db:"owner_full_name" json:"ownerFullName"`
	OwnerNIK      string `db:"owner_nik" json:"ownerNik"`
	OwnerPhone    string `db:"owner_phone" json:"ownerPhone"`
	Email         string `db:"email" json:"email"`

	BusinessName     string  `db:"business_name" json:"businessName"`
	MCC              string  `db:"mcc" json:"mcc"`
	AddressStreet    string  `db:"address_street" json:"addressStreet"`
	AddressRT        *string `db:"address_rt" json:"addressRt,omitempty"`
	AddressRW        *string `db:"address_rw" json:"addressRw,omitempty"`
	AddressKelurahan *string `db:"address_kelurahan" json:"addressKelurahan,omitempty"`
	AddressKecamatan *string `db:"address_kecamatan" json:"addressKecamatan,omitempty"`
	City             string  `db:"city" json:"city"`
	PostalCode       *string `db:"postal_code" json:"postalCode,omitempty"`
	HasPhysicalStore bool    `db:"has_physical_store" json:"hasPhysicalStore"`

	OmzetCategory string   `db:"omzet_category" json:"omzetCategory"`
	QRISType      QRISType `db:"qris_type" json:"qrisType"`
	RiskCategory  string   `db:"risk_category" json:"riskCategory"`

	Website              *string `db:"website" json:"website,omitempty"`
	EstimatedSalesVolume *int64  `db:"estimated_sales_volume" json:"estimatedSalesVolume,omitempty"`
	EstimatedTxCount     *int    `db:"estimated_tx_count" json:"estimatedTxCount,omitempty"`

	DocBundleID    *int `db:"doc_bundle_id" json:"docBundleId,omitempty"`
	BatchID        *int `db:"batch_id" json:"batchId,omitempty"`
	QRISMerchantID *int `db:"qris_merchant_id" json:"qrisMerchantId,omitempty"`

	DocPortalURL   *string `db:"doc_portal_url" json:"docPortalUrl,omitempty"`
	DocPortalToken *string `db:"doc_portal_token" json:"docPortalToken,omitempty"`

	Status    QRISRegistrationStatus `db:"status" json:"status"`
	Note      *string                `db:"note" json:"note,omitempty"`
	CreatedAt time.Time              `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time              `db:"updated_at" json:"updatedAt"`
}

type QRISBatchStatus string

const (
	QRISBatchGenerated QRISBatchStatus = "generated"
	QRISBatchSent      QRISBatchStatus = "sent"
)

// QRISNobuBatch mirrors qris_nobu_batches — one rendered Excel file for a slot.
type QRISNobuBatch struct {
	ID                int             `db:"id" json:"id"`
	BatchDate         time.Time       `db:"batch_date" json:"batchDate"`
	BatchSeq          int             `db:"batch_seq" json:"batchSeq"`
	PeriodLabel       *string         `db:"period_label" json:"periodLabel,omitempty"`
	FileStorageKey    string          `db:"file_storage_key" json:"-"`
	FileName          string          `db:"file_name" json:"fileName"`
	RegistrationCount int             `db:"registration_count" json:"registrationCount"`
	Status            QRISBatchStatus `db:"status" json:"status"`
	CreatedAt         time.Time       `db:"created_at" json:"createdAt"`
}

// ----------------------------------------------------------------------------
// QRIS outbound client webhook (migration 000067).
// ----------------------------------------------------------------------------

type QRISCallbackStatus string

const (
	QRISCallbackPending QRISCallbackStatus = "pending"
	QRISCallbackSuccess QRISCallbackStatus = "success"
	QRISCallbackFailed  QRISCallbackStatus = "failed"
)

// QRIS webhook event names sent to clients.
const (
	QRISEventMerchantActivated = "qris.merchant.activated"
	QRISEventPaymentSuccess    = "qris.payment.success"
)

// QRISCallback mirrors qris_callbacks — one outbound webhook delivery attempt
// record, retried by the callback worker.
type QRISCallback struct {
	ID             int                `db:"id" json:"id"`
	ClientID       int                `db:"client_id" json:"clientId"`
	QRISMerchantID *int               `db:"qris_merchant_id" json:"qrisMerchantId,omitempty"`
	QRISPaymentID  *int               `db:"qris_payment_id" json:"qrisPaymentId,omitempty"`
	Event          string             `db:"event" json:"event"`
	TargetURL      string             `db:"target_url" json:"targetUrl"`
	Payload        NullableRawMessage `db:"payload" json:"payload"`
	Status         QRISCallbackStatus `db:"status" json:"status"`
	Attempts       int                `db:"attempts" json:"attempts"`
	MaxAttempts    int                `db:"max_attempts" json:"maxAttempts"`
	NextRetryAt    time.Time          `db:"next_retry_at" json:"nextRetryAt"`
	LastStatusCode *int               `db:"last_status_code" json:"lastStatusCode,omitempty"`
	LastError      *string            `db:"last_error" json:"lastError,omitempty"`
	DeliveredAt    *time.Time         `db:"delivered_at" json:"deliveredAt,omitempty"`
	CreatedAt      time.Time          `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time          `db:"updated_at" json:"updatedAt"`
}
