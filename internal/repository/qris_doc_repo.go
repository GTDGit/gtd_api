package repository

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

// QRISDocRepository persists QRIS onboarding-document bundles and their files
// (migration 000065). Bytes live in object storage; rows here hold metadata and
// the private storage key.
type QRISDocRepository struct {
	db *sqlx.DB
}

func NewQRISDocRepository(db *sqlx.DB) *QRISDocRepository {
	return &QRISDocRepository{db: db}
}

// CreateBundle inserts a bundle and returns it with the DB-generated token/id.
func (r *QRISDocRepository) CreateBundle(ctx context.Context, b *models.QRISDocBundle) (*models.QRISDocBundle, error) {
	q := `INSERT INTO qris_doc_bundles (merchant_name, qris_merchant_id, status, note, created_by)
	      VALUES ($1, $2, COALESCE(NULLIF($3,''),'active'), $4, $5)
	      RETURNING id, token, merchant_name, qris_merchant_id, status, note, created_by,
	                confirmed_at, expires_at, created_at, updated_at`
	var out models.QRISDocBundle
	if err := r.db.GetContext(ctx, &out, q,
		b.MerchantName, b.QRISMerchantID, string(b.Status), b.Note, b.CreatedBy,
	); err != nil {
		return nil, err
	}
	return &out, nil
}

// AddFile inserts one file row under a bundle and returns it with token/id set.
func (r *QRISDocRepository) AddFile(ctx context.Context, f *models.QRISDocFile) (*models.QRISDocFile, error) {
	q := `INSERT INTO qris_doc_files (bundle_id, doc_type, file_name, content_type, size_bytes, storage_key, checksum)
	      VALUES ($1, $2, $3, $4, $5, $6, $7)
	      RETURNING id, bundle_id, token, doc_type, file_name, content_type, size_bytes, storage_key, checksum, created_at`
	var out models.QRISDocFile
	if err := r.db.GetContext(ctx, &out, q,
		f.BundleID, string(f.DocType), f.FileName, f.ContentType, f.SizeBytes, f.StorageKey, f.Checksum,
	); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetBundleByID loads a bundle by primary key.
func (r *QRISDocRepository) GetBundleByID(ctx context.Context, id int) (*models.QRISDocBundle, error) {
	q := `SELECT id, token, merchant_name, qris_merchant_id, status, note, created_by,
	             confirmed_at, expires_at, created_at, updated_at
	      FROM qris_doc_bundles WHERE id = $1`
	var b models.QRISDocBundle
	if err := r.db.GetContext(ctx, &b, q, id); err != nil {
		return nil, err
	}
	return &b, nil
}

// ListFiles returns all files of a bundle, oldest first.
func (r *QRISDocRepository) ListFiles(ctx context.Context, bundleID int) ([]models.QRISDocFile, error) {
	q := `SELECT id, bundle_id, token, doc_type, file_name, content_type, size_bytes, storage_key, checksum, created_at
	      FROM qris_doc_files WHERE bundle_id = $1 ORDER BY created_at ASC, id ASC`
	var files []models.QRISDocFile
	if err := r.db.SelectContext(ctx, &files, q, bundleID); err != nil {
		return nil, err
	}
	return files, nil
}

// LogAccess records an access-log row for PDP auditability. Best-effort: callers
// generally ignore the error.
func (r *QRISDocRepository) LogAccess(ctx context.Context, bundleID, fileID *int, action, ip, userAgent, detail string) error {
	q := `INSERT INTO qris_doc_access_logs (bundle_id, file_id, action, ip, user_agent, detail)
	      VALUES ($1, $2, $3, NULLIF($4,''), NULLIF($5,''), NULLIF($6,''))`
	_, err := r.db.ExecContext(ctx, q, bundleID, fileID, action, ip, userAgent, detail)
	return err
}
