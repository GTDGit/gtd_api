package models

import (
    "encoding/json"
    "time"
)

type TransactionType string
type TransactionStatus string

const (
    TrxTypePrepaid TransactionType = "prepaid"
    TrxTypeInquiry TransactionType = "inquiry"
    TrxTypePayment TransactionType = "payment"
)

const (
    StatusProcessing TransactionStatus = "Processing"
    StatusSuccess    TransactionStatus = "Success"
    StatusPending    TransactionStatus = "Pending"
    StatusFailed     TransactionStatus = "Failed"
)

// Transaction captures the lifecycle information for a customer transaction.
// Many fields are optional to accommodate different transaction types.
type Transaction struct {
    ID            int               `db:"id" json:"-"`
    TransactionID string            `db:"transaction_id" json:"transactionId"`
    ReferenceID   string            `db:"reference_id" json:"referenceId"`
    ClientID      int               `db:"client_id" json:"-"`
    ProductID     int               `db:"product_id" json:"-"`
    SkuID         *int              `db:"sku_id" json:"-"`
    SkuCode       string            `db:"-" json:"skuCode,omitempty"`
    IsSandbox     bool              `db:"is_sandbox" json:"-"`
    CustomerNo    string            `db:"customer_no" json:"customerNo"`
    CustomerName  *string           `db:"customer_name" json:"customerName,omitempty"`
    Type          TransactionType   `db:"type" json:"type"`
    Status        TransactionStatus `db:"status" json:"status"`
    SerialNumber  *string           `db:"serial_number" json:"serialNumber,omitempty"`
    Amount        *int              `db:"amount" json:"amount,omitempty"`
    Admin         int               `db:"admin" json:"admin,omitempty"`
    Period        *string           `db:"period" json:"period,omitempty"`
    Description   json.RawMessage   `db:"description" json:"description,omitempty"`
    FailedReason  *string           `db:"failed_reason" json:"failedReason,omitempty"`
    RetryCount    int               `db:"retry_count" json:"retryCount,omitempty"`
    NextRetryAt   *time.Time        `db:"next_retry_at" json:"nextRetryAt,omitempty"`
    ExpiredAt     *time.Time        `db:"expired_at" json:"expiredAt,omitempty"`
    InquiryID     *int              `db:"inquiry_id" json:"-"`
    DigiRefID     *string           `db:"digi_ref_id" json:"-"`
    CreatedAt     time.Time         `db:"created_at" json:"createdAt"`
    ProcessedAt   *time.Time        `db:"processed_at" json:"processedAt,omitempty"`
    UpdatedAt     time.Time         `db:"updated_at" json:"-"`
}
