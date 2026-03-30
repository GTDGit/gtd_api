package models

import "time"

type PaymentCallback struct {
	ID               int                `db:"id"`
	Provider         string             `db:"provider"`
	ProviderRef      *string            `db:"provider_ref"`
	Headers          NullableRawMessage `db:"headers"`
	Payload          NullableRawMessage `db:"payload"`
	Signature        *string            `db:"signature"`
	IsValidSignature bool               `db:"is_valid_signature"`
	PaymentID        *string            `db:"payment_id"`
	Status           *string            `db:"status"`
	PaidAmount       *int64             `db:"paid_amount"`
	IsProcessed      bool               `db:"is_processed"`
	ProcessedAt      *time.Time         `db:"processed_at"`
	ProcessError     *string            `db:"process_error"`
	CreatedAt        time.Time          `db:"created_at"`
}
