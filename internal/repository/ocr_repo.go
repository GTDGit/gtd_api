package repository

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

// OCRRepository handles OCR record database operations
type OCRRepository struct {
	db *sqlx.DB
}

// NewOCRRepository creates a new OCR repository
func NewOCRRepository(db *sqlx.DB) *OCRRepository {
	return &OCRRepository{db: db}
}

// Create inserts a new OCR record
func (r *OCRRepository) Create(ctx context.Context, record *models.OCRRecord) error {
	query := `
		INSERT INTO ocr_records (
			id, client_id, doc_type, nik, npwp, npwp_raw, sim_number,
			full_name, place_of_birth, date_of_birth, gender, blood_type,
			address, administrative_code, religion, marital_status, occupation,
			nationality, valid_until, valid_from, published_in, published_on,
			publisher, npwp_format, tax_payer_type, sim_type, height,
			file_urls, raw_text, processing_time_ms
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30
		) RETURNING created_at
	`

	return r.db.QueryRowContext(ctx, query,
		record.ID,
		record.ClientID,
		record.DocType,
		record.NIK,
		record.NPWP,
		record.NPWPRaw,
		record.SIMNumber,
		record.FullName,
		record.PlaceOfBirth,
		record.DateOfBirth,
		record.Gender,
		record.BloodType,
		record.Address,
		record.AdministrativeCode,
		record.Religion,
		record.MaritalStatus,
		record.Occupation,
		record.Nationality,
		record.ValidUntil,
		record.ValidFrom,
		record.PublishedIn,
		record.PublishedOn,
		record.Publisher,
		record.NPWPFormat,
		record.TaxPayerType,
		record.SIMType,
		record.Height,
		record.FileURLs,
		record.RawText,
		record.ProcessingTimeMs,
	).Scan(&record.CreatedAt)
}

// GetByID retrieves an OCR record by ID
func (r *OCRRepository) GetByID(ctx context.Context, id string) (*models.OCRRecord, error) {
	query := `
		SELECT id, client_id, doc_type, nik, npwp, npwp_raw, sim_number,
			full_name, place_of_birth, date_of_birth, gender, blood_type,
			address, administrative_code, religion, marital_status, occupation,
			nationality, valid_until, valid_from, published_in, published_on,
			publisher, npwp_format, tax_payer_type, sim_type, height,
			file_urls, processing_time_ms, created_at
		FROM ocr_records
		WHERE id = $1
	`

	var record models.OCRRecord
	err := r.db.QueryRowxContext(ctx, query, id).StructScan(&record)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// GetByIDAndClientID retrieves an OCR record by ID ensuring client ownership
func (r *OCRRepository) GetByIDAndClientID(ctx context.Context, id string, clientID int) (*models.OCRRecord, error) {
	query := `
		SELECT id, client_id, doc_type, nik, npwp, npwp_raw, sim_number,
			full_name, place_of_birth, date_of_birth, gender, blood_type,
			address, administrative_code, religion, marital_status, occupation,
			nationality, valid_until, valid_from, published_in, published_on,
			publisher, npwp_format, tax_payer_type, sim_type, height,
			file_urls, processing_time_ms, created_at
		FROM ocr_records
		WHERE id = $1 AND client_id = $2
	`

	var record models.OCRRecord
	err := r.db.QueryRowxContext(ctx, query, id, clientID).StructScan(&record)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// GetByNIK retrieves OCR records by NIK
func (r *OCRRepository) GetByNIK(ctx context.Context, nik string, clientID int) ([]*models.OCRRecord, error) {
	query := `
		SELECT id, client_id, doc_type, nik, npwp, npwp_raw, sim_number,
			full_name, place_of_birth, date_of_birth, gender, blood_type,
			address, administrative_code, religion, marital_status, occupation,
			nationality, valid_until, valid_from, published_in, published_on,
			publisher, npwp_format, tax_payer_type, sim_type, height,
			file_urls, processing_time_ms, created_at
		FROM ocr_records
		WHERE nik = $1 AND client_id = $2
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryxContext(ctx, query, nik, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*models.OCRRecord
	for rows.Next() {
		var record models.OCRRecord
		if err := rows.StructScan(&record); err != nil {
			return nil, err
		}
		records = append(records, &record)
	}
	return records, rows.Err()
}
