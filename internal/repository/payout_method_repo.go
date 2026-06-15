package repository

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

// PayoutMethodRepository is the data access layer for the payout_methods
// catalog (per BANK/EWALLET channel config: name, fee, min/max amount).
type PayoutMethodRepository struct {
	db *sqlx.DB
}

func NewPayoutMethodRepository(db *sqlx.DB) *PayoutMethodRepository {
	return &PayoutMethodRepository{db: db}
}

const payoutMethodColumns = `id, method_type, code, name, fee_type, fee_flat, fee_percent,
	fee_min, fee_max, min_amount, max_amount, logo_url, display_order,
	is_active, is_maintenance, maintenance_message, created_at, updated_at`

// GetMethod returns a single catalog row by (method_type, code).
func (r *PayoutMethodRepository) GetMethod(ctx context.Context, mt models.MethodType, code string) (*models.PayoutMethodCatalog, error) {
	q := `SELECT ` + payoutMethodColumns + ` FROM payout_methods WHERE method_type = $1 AND code = $2`
	var m models.PayoutMethodCatalog
	if err := r.db.GetContext(ctx, &m, q, mt, code); err != nil {
		return nil, err
	}
	return &m, nil
}

// ListActive returns active catalog rows ordered for display.
func (r *PayoutMethodRepository) ListActive(ctx context.Context) ([]models.PayoutMethodCatalog, error) {
	q := `SELECT ` + payoutMethodColumns + ` FROM payout_methods
		WHERE is_active = true
		ORDER BY method_type, display_order ASC, id ASC`
	rows := []models.PayoutMethodCatalog{}
	if err := r.db.SelectContext(ctx, &rows, q); err != nil {
		return nil, err
	}
	return rows, nil
}

// ListAll returns every catalog row (admin view).
func (r *PayoutMethodRepository) ListAll(ctx context.Context) ([]models.PayoutMethodCatalog, error) {
	q := `SELECT ` + payoutMethodColumns + ` FROM payout_methods
		ORDER BY method_type, display_order ASC, id ASC`
	rows := []models.PayoutMethodCatalog{}
	if err := r.db.SelectContext(ctx, &rows, q); err != nil {
		return nil, err
	}
	return rows, nil
}

// GetByID returns a single catalog row by primary key (admin view).
func (r *PayoutMethodRepository) GetByID(ctx context.Context, id int) (*models.PayoutMethodCatalog, error) {
	q := `SELECT ` + payoutMethodColumns + ` FROM payout_methods WHERE id = $1`
	var m models.PayoutMethodCatalog
	if err := r.db.GetContext(ctx, &m, q, id); err != nil {
		return nil, err
	}
	return &m, nil
}

// Update mutates the editable fields of a catalog row.
func (r *PayoutMethodRepository) Update(ctx context.Context, m *models.PayoutMethodCatalog) error {
	const q = `
		UPDATE payout_methods
		SET name = $2,
		    fee_type = $3,
		    fee_flat = $4,
		    fee_percent = $5,
		    fee_min = $6,
		    fee_max = $7,
		    min_amount = $8,
		    max_amount = $9,
		    logo_url = $10,
		    display_order = $11,
		    is_active = $12,
		    is_maintenance = $13,
		    maintenance_message = $14,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`
	return r.db.QueryRowContext(ctx, q,
		m.ID, m.Name, m.FeeType, m.FeeFlat, m.FeePercent, m.FeeMin, m.FeeMax,
		m.MinAmount, m.MaxAmount, m.LogoURL, m.DisplayOrder,
		m.IsActive, m.IsMaintenance, m.MaintenanceMessage,
	).Scan(&m.UpdatedAt)
}
