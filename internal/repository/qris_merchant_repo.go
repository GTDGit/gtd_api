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
    sub_merchant_id, registration_id, raw_provider_response, created_at, updated_at`

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

// GetByID loads a merchant by primary key.
func (r *QRISMerchantRepository) GetByID(ctx context.Context, id int) (*models.QRISMerchant, error) {
	q := `SELECT ` + qrisMerchantColumns + ` FROM qris_merchants WHERE id = $1`
	var m models.QRISMerchant
	if err := r.db.GetContext(ctx, &m, q, id); err != nil {
		return nil, err
	}
	return &m, nil
}

// Create inserts a merchant (used on activation) and returns it with id/timestamps.
func (r *QRISMerchantRepository) Create(ctx context.Context, m *models.QRISMerchant) (*models.QRISMerchant, error) {
	q := `INSERT INTO qris_merchants
	        (client_id, provider, merchant_name, merchant_city, merchant_category_code,
	         nmid, store_id, terminal_id, qris_string, status, sub_merchant_id,
	         registration_id, raw_provider_response)
	      VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,COALESCE(NULLIF($10,''),'active'),$11,$12,$13)
	      RETURNING ` + qrisMerchantColumns
	var out models.QRISMerchant
	if err := r.db.GetContext(ctx, &out, q,
		m.ClientID, m.Provider, m.MerchantName, m.MerchantCity, m.MerchantCategoryCode,
		m.NMID, m.StoreID, m.TerminalID, m.QRISString, string(m.Status), m.SubMerchantID,
		m.RegistrationID, m.RawProviderResponse,
	); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateQRISString sets the QR string + parsed descriptive fields after generate
// or manual paste, and returns the refreshed row.
func (r *QRISMerchantRepository) UpdateQRISString(ctx context.Context, m *models.QRISMerchant) (*models.QRISMerchant, error) {
	q := `UPDATE qris_merchants SET
	        qris_string = $2, merchant_name = $3, merchant_city = $4,
	        merchant_category_code = $5, nmid = $6, terminal_id = $7,
	        raw_provider_response = COALESCE($8, raw_provider_response),
	        updated_at = now()
	      WHERE id = $1
	      RETURNING ` + qrisMerchantColumns
	var out models.QRISMerchant
	if err := r.db.GetContext(ctx, &out, q,
		m.ID, m.QRISString, m.MerchantName, m.MerchantCity, m.MerchantCategoryCode,
		m.NMID, m.TerminalID, m.RawProviderResponse,
	); err != nil {
		return nil, err
	}
	return &out, nil
}
