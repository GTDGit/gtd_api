package repository

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

type ReconciliationRepository struct {
	db *sqlx.DB
}

func NewReconciliationRepository(db *sqlx.DB) *ReconciliationRepository {
	return &ReconciliationRepository{db: db}
}

const reconciliationColumns = `id, payment_id, provider, reason, webhook_status, inquiry_status,
    webhook_amount, inquiry_amount, expected_amount, webhook_payload, inquiry_payload,
    status, resolved_status, resolved_by, resolution_note, created_at, updated_at, resolved_at`

// UpsertOpen inserts a new open reconciliation row, or updates the existing open
// row for the same payment (dedupe via the uq_recon_open partial unique index).
// Repeated mismatching webhooks therefore refresh one row instead of piling up.
func (r *ReconciliationRepository) UpsertOpen(ctx context.Context, rec *models.PaymentReconciliation) error {
	const q = `INSERT INTO payment_reconciliations (
        payment_id, provider, reason, webhook_status, inquiry_status,
        webhook_amount, inquiry_amount, expected_amount, webhook_payload, inquiry_payload, status
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'open')
    ON CONFLICT (payment_id) WHERE status = 'open'
    DO UPDATE SET
        provider = EXCLUDED.provider,
        reason = EXCLUDED.reason,
        webhook_status = EXCLUDED.webhook_status,
        inquiry_status = EXCLUDED.inquiry_status,
        webhook_amount = EXCLUDED.webhook_amount,
        inquiry_amount = EXCLUDED.inquiry_amount,
        expected_amount = EXCLUDED.expected_amount,
        webhook_payload = EXCLUDED.webhook_payload,
        inquiry_payload = EXCLUDED.inquiry_payload,
        updated_at = NOW()
    RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, q,
		rec.PaymentID, rec.Provider, rec.Reason, rec.WebhookStatus, rec.InquiryStatus,
		rec.WebhookAmount, rec.InquiryAmount, rec.ExpectedAmount,
		nullableReconJSON(rec.WebhookPayload), nullableReconJSON(rec.InquiryPayload),
	).Scan(&rec.ID, &rec.CreatedAt, &rec.UpdatedAt)
}

// GetOpenByPaymentID returns the open reconciliation for a payment, or
// sql.ErrNoRows when none exists.
func (r *ReconciliationRepository) GetOpenByPaymentID(ctx context.Context, paymentID string) (*models.PaymentReconciliation, error) {
	const q = `SELECT ` + reconciliationColumns + `
        FROM payment_reconciliations
        WHERE payment_id = $1 AND status = 'open'
        LIMIT 1`
	var rec models.PaymentReconciliation
	if err := r.db.GetContext(ctx, &rec, q, paymentID); err != nil {
		return nil, err
	}
	return &rec, nil
}

// ResolveByID marks a specific reconciliation row resolved.
func (r *ReconciliationRepository) ResolveByID(ctx context.Context, id int64, resolvedStatus, resolvedBy, note string) error {
	const q = `UPDATE payment_reconciliations SET
        status = 'resolved',
        resolved_status = $2,
        resolved_by = $3,
        resolution_note = NULLIF($4, ''),
        resolved_at = NOW(),
        updated_at = NOW()
    WHERE id = $1 AND status = 'open'`
	res, err := r.db.ExecContext(ctx, q, id, resolvedStatus, resolvedBy, note)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ResolveOpenByPaymentID resolves the open reconciliation (if any) for a
// payment. Returns false when there was no open row — a no-op, not an error —
// so callers can invoke it unconditionally after a confirmed transition.
func (r *ReconciliationRepository) ResolveOpenByPaymentID(ctx context.Context, paymentID, resolvedStatus, resolvedBy, note string) (bool, error) {
	const q = `UPDATE payment_reconciliations SET
        status = 'resolved',
        resolved_status = $2,
        resolved_by = $3,
        resolution_note = NULLIF($4, ''),
        resolved_at = NOW(),
        updated_at = NOW()
    WHERE payment_id = $1 AND status = 'open'`
	res, err := r.db.ExecContext(ctx, q, paymentID, resolvedStatus, resolvedBy, note)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func nullableReconJSON(v models.NullableRawMessage) any {
	if len(v) == 0 {
		return nil
	}
	return []byte(v)
}
