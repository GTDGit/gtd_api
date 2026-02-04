package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

// LivenessRepository handles liveness database operations
type LivenessRepository struct {
	db *sqlx.DB
}

// NewLivenessRepository creates a new liveness repository
func NewLivenessRepository(db *sqlx.DB) *LivenessRepository {
	return &LivenessRepository{db: db}
}

// Create inserts a new liveness session
func (r *LivenessRepository) Create(ctx context.Context, session *models.LivenessSession) error {
	// Serialize challenges to JSON - always use empty array if no challenges
	var challengesJSON []byte
	if len(session.Challenges) > 0 {
		challengesJSON, _ = json.Marshal(session.Challenges)
	} else {
		challengesJSON = []byte("[]") // Empty JSON array for JSONB column
	}

	query := `
		INSERT INTO liveness_sessions (
			id, client_id, nik, session_id, method, status, challenges, expires_at, expired_at, created_at
		) VALUES (
			$1, $2, $3, $4, $5, 'Pending', $6, $7, $7, NOW()
		)`

	_, err := r.db.ExecContext(ctx, query,
		session.ID,
		session.ClientID,
		nullString(session.NIK), // NIK is nullable in DB
		session.SessionID,
		session.Method,
		challengesJSON,
		session.ExpiresAt,
	)
	return err
}

// Update updates a liveness session
func (r *LivenessRepository) Update(ctx context.Context, session *models.LivenessSession) error {
	query := `
		UPDATE liveness_sessions SET
			status = $1,
			is_live = $2,
			confidence = $3,
			best_frame_url = $4,
			failed_reason = $5,
			completed_at = $6,
			updated_at = NOW()
		WHERE id = $7 OR session_id = $7`

	_, err := r.db.ExecContext(ctx, query,
		session.Status,
		session.IsLive,
		session.Confidence,
		nullString(session.FaceURL),
		nullString(session.FailureReason),
		session.CompletedAt,
		session.ID,
	)
	return err
}

// GetBySessionID retrieves a liveness session by session ID
func (r *LivenessRepository) GetBySessionID(ctx context.Context, sessionID string) (*models.LivenessSession, error) {
	query := `
		SELECT id, client_id, COALESCE(nik, ''), session_id, COALESCE(method, 'passive'), status,
		       is_live, confidence, COALESCE(best_frame_url, ''), COALESCE(failed_reason, ''),
		       challenges, created_at, completed_at, expires_at
		FROM liveness_sessions
		WHERE session_id = $1`

	return r.scanSession(ctx, query, sessionID)
}

// GetByID retrieves a liveness session by internal ID
func (r *LivenessRepository) GetByID(ctx context.Context, id string) (*models.LivenessSession, error) {
	query := `
		SELECT id, client_id, COALESCE(nik, ''), session_id, COALESCE(method, 'passive'), status,
		       is_live, confidence, COALESCE(best_frame_url, ''), COALESCE(failed_reason, ''),
		       challenges, created_at, completed_at, expires_at
		FROM liveness_sessions
		WHERE id = $1`

	return r.scanSession(ctx, query, id)
}

// GetBySessionIDAndClientID retrieves a session by session ID and client ID
func (r *LivenessRepository) GetBySessionIDAndClientID(ctx context.Context, sessionID string, clientID int) (*models.LivenessSession, error) {
	query := `
		SELECT id, client_id, COALESCE(nik, ''), session_id, COALESCE(method, 'passive'), status,
		       is_live, confidence, COALESCE(best_frame_url, ''), COALESCE(failed_reason, ''),
		       challenges, created_at, completed_at, expires_at
		FROM liveness_sessions
		WHERE session_id = $1 AND client_id = $2`

	return r.scanSession(ctx, query, sessionID, clientID)
}

// GetByNIK retrieves the latest liveness session for a NIK
func (r *LivenessRepository) GetByNIK(ctx context.Context, nik string, clientID int) (*models.LivenessSession, error) {
	query := `
		SELECT id, client_id, COALESCE(nik, ''), session_id, COALESCE(method, 'passive'), status,
		       is_live, confidence, COALESCE(best_frame_url, ''), COALESCE(failed_reason, ''),
		       challenges, created_at, completed_at, expires_at
		FROM liveness_sessions
		WHERE nik = $1 AND client_id = $2
		ORDER BY created_at DESC
		LIMIT 1`

	return r.scanSession(ctx, query, nik, clientID)
}

// scanSession scans a row into a LivenessSession
func (r *LivenessRepository) scanSession(ctx context.Context, query string, args ...interface{}) (*models.LivenessSession, error) {
	var session models.LivenessSession
	var challengesJSON []byte
	var method string
	var expiresAt sql.NullTime
	var status string

	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&session.ID,
		&session.ClientID,
		&session.NIK,
		&session.SessionID,
		&method,
		&status,
		&session.IsLive,
		&session.Confidence,
		&session.FaceURL,
		&session.FailureReason,
		&challengesJSON,
		&session.CreatedAt,
		&session.CompletedAt,
		&expiresAt,
	)
	if err != nil {
		return nil, err
	}

	session.Method = models.LivenessMethod(method)
	session.Status = status

	if expiresAt.Valid {
		session.ExpiresAt = expiresAt.Time
	}

	// Deserialize challenges
	if len(challengesJSON) > 0 {
		json.Unmarshal(challengesJSON, &session.Challenges)
	}

	return &session, nil
}

// ExpireOldSessions expires sessions that have passed their expiration time
func (r *LivenessRepository) ExpireOldSessions(ctx context.Context) (int64, error) {
	query := `
		UPDATE liveness_sessions
		SET status = 'Expired', updated_at = NOW()
		WHERE status = 'Pending' AND expires_at < NOW()`

	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// nullStringLiveness helper for nullable strings
func nullStringLiveness(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
