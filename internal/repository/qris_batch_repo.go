package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

// QRISBatchRepository persists rendered Nobu Excel batches (migration 000067).
type QRISBatchRepository struct {
	db *sqlx.DB
}

func NewQRISBatchRepository(db *sqlx.DB) *QRISBatchRepository {
	return &QRISBatchRepository{db: db}
}

const qrisBatchColumns = `id, batch_date, batch_seq, period_label, file_storage_key,
    file_name, registration_count, status, created_at`

// Create inserts a batch row and returns it with id/timestamps.
func (r *QRISBatchRepository) Create(ctx context.Context, b *models.QRISNobuBatch) (*models.QRISNobuBatch, error) {
	q := `INSERT INTO qris_nobu_batches
	        (batch_date, batch_seq, period_label, file_storage_key, file_name,
	         registration_count, status)
	      VALUES ($1,$2,$3,$4,$5,$6,COALESCE(NULLIF($7,''),'generated'))
	      RETURNING ` + qrisBatchColumns
	var out models.QRISNobuBatch
	if err := r.db.GetContext(ctx, &out, q,
		b.BatchDate, b.BatchSeq, b.PeriodLabel, b.FileStorageKey, b.FileName,
		b.RegistrationCount, string(b.Status),
	); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetBySlot returns the batch for a (date, seq) slot, or (nil, nil) if none.
func (r *QRISBatchRepository) GetBySlot(ctx context.Context, day string, seq int) (*models.QRISNobuBatch, error) {
	q := `SELECT ` + qrisBatchColumns + `
	      FROM qris_nobu_batches WHERE batch_date = $1 AND batch_seq = $2 LIMIT 1`
	var b models.QRISNobuBatch
	if err := r.db.GetContext(ctx, &b, q, day, seq); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

// GetByID loads a batch by primary key.
func (r *QRISBatchRepository) GetByID(ctx context.Context, id int) (*models.QRISNobuBatch, error) {
	q := `SELECT ` + qrisBatchColumns + ` FROM qris_nobu_batches WHERE id = $1`
	var b models.QRISNobuBatch
	if err := r.db.GetContext(ctx, &b, q, id); err != nil {
		return nil, err
	}
	return &b, nil
}

// List returns batches newest first, with total count.
func (r *QRISBatchRepository) List(ctx context.Context, limit, offset int) ([]models.QRISNobuBatch, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM qris_nobu_batches`); err != nil {
		return nil, 0, err
	}
	q := `SELECT ` + qrisBatchColumns + ` FROM qris_nobu_batches
	      ORDER BY batch_date DESC, batch_seq DESC LIMIT $1 OFFSET $2`
	var items []models.QRISNobuBatch
	if err := r.db.SelectContext(ctx, &items, q, limit, offset); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// MarkSent flips a batch to "sent" once delivered to Nobu.
func (r *QRISBatchRepository) MarkSent(ctx context.Context, id int) error {
	_, err := r.db.ExecContext(ctx, `UPDATE qris_nobu_batches SET status = 'sent' WHERE id = $1`, id)
	return err
}
