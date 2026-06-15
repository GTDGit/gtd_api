package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

// QRISCallbackRepository persists outbound client webhook delivery attempts for
// QRIS events (migration 000067). Mirrors the payment_callback retry model.
type QRISCallbackRepository struct {
	db *sqlx.DB
}

func NewQRISCallbackRepository(db *sqlx.DB) *QRISCallbackRepository {
	return &QRISCallbackRepository{db: db}
}

const qrisCallbackColumns = `id, client_id, qris_merchant_id, qris_payment_id, event,
    target_url, payload, status, attempts, max_attempts, next_retry_at,
    last_status_code, last_error, delivered_at, created_at, updated_at`

// Create inserts a pending callback row and returns it with id/timestamps.
func (r *QRISCallbackRepository) Create(ctx context.Context, cb *models.QRISCallback) (*models.QRISCallback, error) {
	q := `INSERT INTO qris_callbacks
	        (client_id, qris_merchant_id, qris_payment_id, event, target_url, payload,
	         status, attempts, max_attempts, next_retry_at)
	      VALUES ($1,$2,$3,$4,$5,$6,
	              COALESCE(NULLIF($7,''),'pending'),$8,COALESCE(NULLIF($9,0),5),$10)
	      RETURNING ` + qrisCallbackColumns
	maxAttempts := cb.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	next := cb.NextRetryAt
	if next.IsZero() {
		next = time.Now()
	}
	var out models.QRISCallback
	if err := r.db.GetContext(ctx, &out, q,
		cb.ClientID, cb.QRISMerchantID, cb.QRISPaymentID, cb.Event, cb.TargetURL,
		nullableQRISJSON(cb.Payload), string(cb.Status), cb.Attempts, maxAttempts, next,
	); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListDue returns pending callbacks whose next_retry_at has elapsed, oldest first.
func (r *QRISCallbackRepository) ListDue(ctx context.Context, limit int) ([]models.QRISCallback, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT ` + qrisCallbackColumns + `
	      FROM qris_callbacks
	      WHERE status = 'pending' AND next_retry_at <= now()
	      ORDER BY next_retry_at ASC, id ASC
	      LIMIT $1`
	var items []models.QRISCallback
	if err := r.db.SelectContext(ctx, &items, q, limit); err != nil {
		return nil, err
	}
	return items, nil
}

// MarkDelivered flips a callback to success after a 2xx response.
func (r *QRISCallbackRepository) MarkDelivered(ctx context.Context, id, statusCode int) error {
	q := `UPDATE qris_callbacks SET
	        status = 'success', attempts = attempts + 1, last_status_code = $2,
	        last_error = NULL, delivered_at = now(), next_retry_at = now(), updated_at = now()
	      WHERE id = $1`
	_, err := r.db.ExecContext(ctx, q, id, statusCode)
	return err
}

// MarkFailure records a failed attempt. When attempts reach max_attempts the row
// is flipped to failed; otherwise it stays pending with the next retry time.
func (r *QRISCallbackRepository) MarkFailure(ctx context.Context, id int, statusCode *int, errMsg string, nextRetry *time.Time) error {
	q := `UPDATE qris_callbacks SET
	        attempts = attempts + 1,
	        last_status_code = $2,
	        last_error = $3,
	        status = CASE WHEN attempts + 1 >= max_attempts THEN 'failed' ELSE 'pending' END,
	        next_retry_at = COALESCE($4, next_retry_at),
	        updated_at = now()
	      WHERE id = $1`
	_, err := r.db.ExecContext(ctx, q, id, statusCode, errMsg, nextRetry)
	return err
}
