package repository

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

type QRISMerchantRepository struct {
	db *sqlx.DB
}

func NewQRISMerchantRepository(db *sqlx.DB) *QRISMerchantRepository {
	return &QRISMerchantRepository{db: db}
}

const qrisMerchantColumns = `id, client_id, provider, merchant_name, merchant_city,
    merchant_category_code, nmid, store_id, terminal_id, qris_string, status,
    raw_provider_response, created_at, updated_at`

// GetByStore resolves a merchant from the (provider, store_id) pair echoed back
// by an inbound webhook. This is the identification path: store_id is unique per
// provider (uq_qris_merchant_store).
func (r *QRISMerchantRepository) GetByStore(ctx context.Context, provider models.QRISProvider, storeID string) (*models.QRISMerchant, error) {
	q := `SELECT ` + qrisMerchantColumns + ` FROM qris_merchants WHERE provider = $1 AND store_id = $2 LIMIT 1`
	var m models.QRISMerchant
	if err := r.db.GetContext(ctx, &m, q, provider, storeID); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &m, nil
}
