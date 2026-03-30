package repository

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

type PaymentRepository struct {
	db *sqlx.DB
}

func NewPaymentRepository(db *sqlx.DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

func nullablePaymentJSON(v models.NullableRawMessage) any {
	if len(v) == 0 {
		return nil
	}
	return []byte(v)
}

func (r *PaymentRepository) CreatePaymentCallback(ctx context.Context, callback *models.PaymentCallback) error {
	const q = `
		INSERT INTO payment_callbacks (
			provider, provider_ref, headers, payload, signature, is_valid_signature,
			payment_id, status, paid_amount, is_processed, processed_at, process_error
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12
		)
		RETURNING id, created_at`

	return r.db.QueryRowContext(
		ctx,
		q,
		callback.Provider,
		callback.ProviderRef,
		nullablePaymentJSON(callback.Headers),
		nullablePaymentJSON(callback.Payload),
		callback.Signature,
		callback.IsValidSignature,
		callback.PaymentID,
		callback.Status,
		callback.PaidAmount,
		callback.IsProcessed,
		callback.ProcessedAt,
		callback.ProcessError,
	).Scan(&callback.ID, &callback.CreatedAt)
}

func (r *PaymentRepository) UpdatePaymentCallbackProcessed(ctx context.Context, callbackID int, isProcessed bool, processError *string) error {
	const q = `
		UPDATE payment_callbacks
		SET is_processed = $2,
		    processed_at = CASE WHEN $2 THEN NOW() ELSE NULL END,
		    process_error = $3
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, q, callbackID, isProcessed, processError)
	return err
}

func (r *PaymentRepository) UpdatePaymentCallbackSignature(ctx context.Context, callbackID int, isValidSignature bool) error {
	const q = `
		UPDATE payment_callbacks
		SET is_valid_signature = $2
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, q, callbackID, isValidSignature)
	return err
}
