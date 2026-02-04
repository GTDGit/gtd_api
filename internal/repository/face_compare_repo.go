package repository

import (
	"context"
	"encoding/json"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

// FaceCompareRepository handles face comparison database operations
type FaceCompareRepository struct {
	db *sqlx.DB
}

// NewFaceCompareRepository creates a new face compare repository
func NewFaceCompareRepository(db *sqlx.DB) *FaceCompareRepository {
	return &FaceCompareRepository{db: db}
}

// Create inserts a new face comparison record
func (r *FaceCompareRepository) Create(ctx context.Context, fc *models.FaceCompare) error {
	// Serialize bounding boxes to JSON
	var sourceBBox, targetBBox []byte
	var err error

	if fc.SourceBoundingBox != nil {
		sourceBBox, err = json.Marshal(fc.SourceBoundingBox)
		if err != nil {
			return err
		}
	}

	if fc.TargetBoundingBox != nil {
		targetBBox, err = json.Marshal(fc.TargetBoundingBox)
		if err != nil {
			return err
		}
	}

	query := `
		INSERT INTO face_comparisons (
			client_id, source_type, source_url, target_type, target_url,
			matched, similarity, threshold,
			source_detected, source_confidence, source_bounding_box,
			target_detected, target_confidence, target_bounding_box,
			processing_time_ms, aws_request_id, error_code, error_message
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11,
			$12, $13, $14,
			$15, $16, $17, $18
		) RETURNING id::text, created_at`

	return r.db.QueryRowContext(ctx, query,
		fc.ClientID,
		fc.SourceType,
		fc.SourceURL,
		fc.TargetType,
		fc.TargetURL,
		fc.Matched,
		fc.Similarity,
		fc.Threshold,
		fc.SourceDetected,
		fc.SourceConfidence,
		sourceBBox,
		fc.TargetDetected,
		fc.TargetConfidence,
		targetBBox,
		fc.ProcessingTimeMs,
		fc.AWSRequestID,
		nullString(fc.ErrorCode),
		nullString(fc.ErrorMessage),
	).Scan(&fc.ID, &fc.CreatedAt)
}

// GetByID retrieves a face comparison record by ID
func (r *FaceCompareRepository) GetByID(ctx context.Context, id string) (*models.FaceCompare, error) {
	query := `
		SELECT id::text, client_id, source_type, source_url, target_type, target_url,
		       matched, similarity, threshold,
		       source_detected, source_confidence, source_bounding_box,
		       target_detected, target_confidence, target_bounding_box,
		       processing_time_ms, COALESCE(aws_request_id, ''), COALESCE(error_code, ''), COALESCE(error_message, ''),
		       created_at
		FROM face_comparisons
		WHERE id::text = $1`

	var fc models.FaceCompare
	var sourceBBox, targetBBox []byte

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&fc.ID,
		&fc.ClientID,
		&fc.SourceType,
		&fc.SourceURL,
		&fc.TargetType,
		&fc.TargetURL,
		&fc.Matched,
		&fc.Similarity,
		&fc.Threshold,
		&fc.SourceDetected,
		&fc.SourceConfidence,
		&sourceBBox,
		&fc.TargetDetected,
		&fc.TargetConfidence,
		&targetBBox,
		&fc.ProcessingTimeMs,
		&fc.AWSRequestID,
		&fc.ErrorCode,
		&fc.ErrorMessage,
		&fc.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Deserialize bounding boxes
	if len(sourceBBox) > 0 {
		var bbox models.BoundingBox
		if err := json.Unmarshal(sourceBBox, &bbox); err == nil {
			fc.SourceBoundingBox = &bbox
		}
	}

	if len(targetBBox) > 0 {
		var bbox models.BoundingBox
		if err := json.Unmarshal(targetBBox, &bbox); err == nil {
			fc.TargetBoundingBox = &bbox
		}
	}

	return &fc, nil
}

// GetByIDAndClientID retrieves a face comparison record by ID and client ID
func (r *FaceCompareRepository) GetByIDAndClientID(ctx context.Context, id string, clientID int) (*models.FaceCompare, error) {
	query := `
		SELECT id::text, client_id, source_type, source_url, target_type, target_url,
		       matched, similarity, threshold,
		       source_detected, source_confidence, source_bounding_box,
		       target_detected, target_confidence, target_bounding_box,
		       processing_time_ms, COALESCE(aws_request_id, ''), COALESCE(error_code, ''), COALESCE(error_message, ''),
		       created_at
		FROM face_comparisons
		WHERE id::text = $1 AND client_id = $2`

	var fc models.FaceCompare
	var sourceBBox, targetBBox []byte

	err := r.db.QueryRowContext(ctx, query, id, clientID).Scan(
		&fc.ID,
		&fc.ClientID,
		&fc.SourceType,
		&fc.SourceURL,
		&fc.TargetType,
		&fc.TargetURL,
		&fc.Matched,
		&fc.Similarity,
		&fc.Threshold,
		&fc.SourceDetected,
		&fc.SourceConfidence,
		&sourceBBox,
		&fc.TargetDetected,
		&fc.TargetConfidence,
		&targetBBox,
		&fc.ProcessingTimeMs,
		&fc.AWSRequestID,
		&fc.ErrorCode,
		&fc.ErrorMessage,
		&fc.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Deserialize bounding boxes
	if len(sourceBBox) > 0 {
		var bbox models.BoundingBox
		if err := json.Unmarshal(sourceBBox, &bbox); err == nil {
			fc.SourceBoundingBox = &bbox
		}
	}

	if len(targetBBox) > 0 {
		var bbox models.BoundingBox
		if err := json.Unmarshal(targetBBox, &bbox); err == nil {
			fc.TargetBoundingBox = &bbox
		}
	}

	return &fc, nil
}

// nullString converts empty string to nil
func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
