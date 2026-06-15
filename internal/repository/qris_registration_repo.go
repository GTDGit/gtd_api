package repository

import (
	"context"
	"strconv"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

// QRISRegistrationRepository persists client static-QRIS onboarding requests
// (migration 000067). A registration is the intake form Nobu requires; it is
// later rendered into an Excel batch and, once Nobu provisions the merchant,
// activated into a qris_merchants row.
type QRISRegistrationRepository struct {
	db *sqlx.DB
}

func NewQRISRegistrationRepository(db *sqlx.DB) *QRISRegistrationRepository {
	return &QRISRegistrationRepository{db: db}
}

const qrisRegistrationColumns = `id, registration_id, client_id, registration_ref, owner_full_name, owner_nik,
    owner_phone, email, business_name, mcc, address_street, address_rt, address_rw,
    address_kelurahan, address_kecamatan, city, postal_code, has_physical_store,
    omzet_category, qris_type, risk_category, website, estimated_sales_volume,
    estimated_tx_count, doc_bundle_id, batch_id, qris_merchant_id, status, note,
    doc_portal_url, doc_portal_token, created_at, updated_at`

// Create inserts a registration and returns it with id/timestamps populated.
func (r *QRISRegistrationRepository) Create(ctx context.Context, reg *models.QRISRegistration) (*models.QRISRegistration, error) {
	q := `INSERT INTO qris_registrations
	        (registration_id, client_id, registration_ref, owner_full_name, owner_nik, owner_phone, email,
	         business_name, mcc, address_street, address_rt, address_rw, address_kelurahan,
	         address_kecamatan, city, postal_code, has_physical_store, omzet_category,
	         qris_type, risk_category, website, estimated_sales_volume, estimated_tx_count,
	         doc_bundle_id, status, note)
	      VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,
	              COALESCE(NULLIF($25,''),'pending_batch'),$26)
	      RETURNING ` + qrisRegistrationColumns
	var out models.QRISRegistration
	if err := r.db.GetContext(ctx, &out, q,
		reg.RegistrationID, reg.ClientID, reg.RegistrationRef, reg.OwnerFullName, reg.OwnerNIK, reg.OwnerPhone, reg.Email,
		reg.BusinessName, reg.MCC, reg.AddressStreet, reg.AddressRT, reg.AddressRW, reg.AddressKelurahan,
		reg.AddressKecamatan, reg.City, reg.PostalCode, reg.HasPhysicalStore, reg.OmzetCategory,
		string(reg.QRISType), reg.RiskCategory, reg.Website, reg.EstimatedSalesVolume, reg.EstimatedTxCount,
		reg.DocBundleID, string(reg.Status), reg.Note,
	); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetByRef loads a registration scoped to a client by its idempotency reference.
func (r *QRISRegistrationRepository) GetByRef(ctx context.Context, clientID int, ref string) (*models.QRISRegistration, error) {
	q := `SELECT ` + qrisRegistrationColumns + `
	      FROM qris_registrations WHERE client_id = $1 AND registration_ref = $2 LIMIT 1`
	var reg models.QRISRegistration
	if err := r.db.GetContext(ctx, &reg, q, clientID, ref); err != nil {
		return nil, err
	}
	return &reg, nil
}

// GetByRegistrationID loads a registration scoped to a client by its public
// UUID (the client-facing `id`). This is the lookup behind GET /v1/qris/merchants/{id}.
func (r *QRISRegistrationRepository) GetByRegistrationID(ctx context.Context, clientID int, registrationID string) (*models.QRISRegistration, error) {
	q := `SELECT ` + qrisRegistrationColumns + `
	      FROM qris_registrations WHERE client_id = $1 AND registration_id = $2 LIMIT 1`
	var reg models.QRISRegistration
	if err := r.db.GetContext(ctx, &reg, q, clientID, registrationID); err != nil {
		return nil, err
	}
	return &reg, nil
}

// GetByID loads a registration by primary key (admin / activation path).
func (r *QRISRegistrationRepository) GetByID(ctx context.Context, id int) (*models.QRISRegistration, error) {
	q := `SELECT ` + qrisRegistrationColumns + ` FROM qris_registrations WHERE id = $1`
	var reg models.QRISRegistration
	if err := r.db.GetContext(ctx, &reg, q, id); err != nil {
		return nil, err
	}
	return &reg, nil
}

// QRISRegistrationFilter narrows a list query.
type QRISRegistrationFilter struct {
	ClientID *int
	Status   string
	Limit    int
	Offset   int
}

// List returns registrations matching the filter, newest first, with total count.
func (r *QRISRegistrationRepository) List(ctx context.Context, f QRISRegistrationFilter) ([]models.QRISRegistration, int, error) {
	where := ` WHERE 1=1`
	args := []any{}
	n := 1
	if f.ClientID != nil {
		where += ` AND client_id = $` + strconv.Itoa(n)
		args = append(args, *f.ClientID)
		n++
	}
	if f.Status != "" {
		where += ` AND status = $` + strconv.Itoa(n)
		args = append(args, f.Status)
		n++
	}

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM qris_registrations`+where, args...); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	q := `SELECT ` + qrisRegistrationColumns + ` FROM qris_registrations` + where +
		` ORDER BY created_at DESC, id DESC LIMIT $` + strconv.Itoa(n) + ` OFFSET $` + strconv.Itoa(n+1)
	args = append(args, limit, f.Offset)

	var items []models.QRISRegistration
	if err := r.db.SelectContext(ctx, &items, q, args...); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListPendingForBatch returns all pending_batch registrations ordered oldest
// first — the set the batch worker accumulates into one Excel file.
func (r *QRISRegistrationRepository) ListPendingForBatch(ctx context.Context) ([]models.QRISRegistration, error) {
	q := `SELECT ` + qrisRegistrationColumns + `
	      FROM qris_registrations WHERE status = 'pending_batch'
	      ORDER BY created_at ASC, id ASC`
	var items []models.QRISRegistration
	if err := r.db.SelectContext(ctx, &items, q); err != nil {
		return nil, err
	}
	return items, nil
}

// MarkSubmitted moves a set of registrations into a batch (pending_batch ->
// submitted) within one transaction. Returns how many rows changed.
func (r *QRISRegistrationRepository) MarkSubmitted(ctx context.Context, ids []int, batchID int) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	q, args, err := sqlx.In(
		`UPDATE qris_registrations SET status = 'submitted', batch_id = ?, updated_at = now()
		 WHERE id IN (?) AND status = 'pending_batch'`, batchID, ids)
	if err != nil {
		return 0, err
	}
	q = r.db.Rebind(q)
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SetStatus updates the lifecycle status (+ optional note) of one registration.
func (r *QRISRegistrationRepository) SetStatus(ctx context.Context, id int, status models.QRISRegistrationStatus, note *string) error {
	q := `UPDATE qris_registrations SET status = $2, note = COALESCE($3, note), updated_at = now() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, q, id, string(status), note)
	return err
}

// Activate links a registration to its created merchant and flips it to activated.
func (r *QRISRegistrationRepository) Activate(ctx context.Context, id, merchantID int) error {
	q := `UPDATE qris_registrations SET status = 'activated', qris_merchant_id = $2, updated_at = now() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, q, id, merchantID)
	return err
}

// SetDocPortal records the file-delivery portal bundle URL + token for a
// registration (set after a best-effort upload at intake). Either value may be
// empty if the portal is disabled or the upload failed.
func (r *QRISRegistrationRepository) SetDocPortal(ctx context.Context, id int, url, token string) error {
	q := `UPDATE qris_registrations
	      SET doc_portal_url = NULLIF($2,''), doc_portal_token = NULLIF($3,''), updated_at = now()
	      WHERE id = $1`
	_, err := r.db.ExecContext(ctx, q, id, url, token)
	return err
}
