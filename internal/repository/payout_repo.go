package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

// PayoutFilter is the admin-facing list filter for payouts.
type PayoutFilter struct {
	Status      string
	MethodType  string
	Provider    string
	ChannelCode string
	ClientID    int
	IsSandbox   *bool
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Search      string
}

// PayoutStats summarizes payout aggregates for admin dashboards.
type PayoutStats struct {
	Total           int   `db:"total" json:"total"`
	TotalSuccess    int   `db:"total_success" json:"totalSuccess"`
	TotalProcessing int   `db:"total_processing" json:"totalProcessing"`
	TotalPending    int   `db:"total_pending" json:"totalPending"`
	TotalFailed     int   `db:"total_failed" json:"totalFailed"`
	TotalVolume     int64 `db:"total_volume" json:"totalVolume"`
}

type PayoutRepository struct {
	db *sqlx.DB
}

func nullablePayoutJSON(v models.NullableRawMessage) any {
	if len(v) == 0 {
		return nil
	}
	return []byte(v)
}

func NewPayoutRepository(db *sqlx.DB) *PayoutRepository {
	return &PayoutRepository{db: db}
}

// ---------------------------------------------------------------------------
// Routes (per method_type provider routing)
// ---------------------------------------------------------------------------

// ListRoutesByMethodType returns active+inactive routes for a method_type
// ordered by priority ASC (lower = preferred).
func (r *PayoutRepository) ListRoutesByMethodType(ctx context.Context, mt models.MethodType) ([]models.PayoutRoute, error) {
	const q = `
		SELECT id, method_type, provider, priority, is_active, is_maintenance,
		       maintenance_message, created_at, updated_at
		FROM payout_routes
		WHERE method_type = $1
		ORDER BY priority ASC`
	rows := []models.PayoutRoute{}
	if err := r.db.SelectContext(ctx, &rows, q, mt); err != nil {
		return nil, err
	}
	return rows, nil
}

// ListRoutes returns all routes ordered by method_type, priority.
func (r *PayoutRepository) ListRoutes(ctx context.Context) ([]models.PayoutRoute, error) {
	const q = `
		SELECT id, method_type, provider, priority, is_active, is_maintenance,
		       maintenance_message, created_at, updated_at
		FROM payout_routes
		ORDER BY method_type, priority ASC`
	rows := []models.PayoutRoute{}
	if err := r.db.SelectContext(ctx, &rows, q); err != nil {
		return nil, err
	}
	return rows, nil
}

// UpdateRoute updates the mutable fields of a payout route.
func (r *PayoutRepository) UpdateRoute(ctx context.Context, route *models.PayoutRoute) error {
	const q = `
		UPDATE payout_routes
		SET priority = $2,
		    is_active = $3,
		    is_maintenance = $4,
		    maintenance_message = $5,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`
	return r.db.QueryRowContext(
		ctx, q,
		route.ID,
		route.Priority,
		route.IsActive,
		route.IsMaintenance,
		route.MaintenanceMessage,
	).Scan(&route.UpdatedAt)
}

// ---------------------------------------------------------------------------
// Inquiries
// ---------------------------------------------------------------------------

func (r *PayoutRepository) CreateInquiry(ctx context.Context, inquiry *models.PayoutInquiry) error {
	const q = `
		INSERT INTO payout_inquiries (
			inquiry_id, client_id, is_sandbox, method_type, channel_code,
			bank_code, bank_name, account_number, account_name,
			transfer_type, provider, provider_ref, provider_data, expired_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12, $13, $14
		)
		RETURNING id, created_at`

	return r.db.QueryRowContext(
		ctx, q,
		inquiry.InquiryID,
		inquiry.ClientID,
		inquiry.IsSandbox,
		inquiry.MethodType,
		inquiry.ChannelCode,
		inquiry.BankCode,
		inquiry.BankName,
		inquiry.AccountNumber,
		inquiry.AccountName,
		inquiry.TransferType,
		inquiry.Provider,
		inquiry.ProviderRef,
		nullablePayoutJSON(inquiry.ProviderData),
		inquiry.ExpiredAt,
	).Scan(&inquiry.ID, &inquiry.CreatedAt)
}

func (r *PayoutRepository) GetInquiryByInquiryID(ctx context.Context, inquiryID string) (*models.PayoutInquiry, error) {
	const q = `
		SELECT id, inquiry_id, client_id, is_sandbox, method_type, channel_code,
		       bank_code, bank_name, account_number, account_name,
		       transfer_type, provider, provider_ref, provider_data, expired_at, created_at
		FROM payout_inquiries
		WHERE inquiry_id = $1
		LIMIT 1`
	var inquiry models.PayoutInquiry
	if err := r.db.GetContext(ctx, &inquiry, q, inquiryID); err != nil {
		return nil, err
	}
	return &inquiry, nil
}

// ---------------------------------------------------------------------------
// Payouts
// ---------------------------------------------------------------------------

const payoutColumns = `id, payout_id, reference_id, client_id, is_sandbox, method_type, channel_code,
		transfer_type, provider, bank_code, bank_name, account_number, account_name,
		source_bank_code, source_account_number, amount, fee, send_amount, total_amount, fee_paid_by,
		status, failed_reason, failed_code, purpose_code, remark, description,
		customer_name, customer_email, customer_phone,
		inquiry_id, provider_ref, provider_data, callback_url, callback_sent, callback_sent_at,
		callback_attempts, created_at, completed_at, failed_at, updated_at`

func (r *PayoutRepository) CreatePayout(ctx context.Context, p *models.Payout) error {
	const q = `
		INSERT INTO payouts (
			payout_id, reference_id, client_id, is_sandbox, method_type, channel_code,
			transfer_type, provider, bank_code, bank_name, account_number, account_name,
			source_bank_code, source_account_number, amount, fee, send_amount, total_amount, fee_paid_by,
			status, failed_reason, failed_code, purpose_code, remark, description,
			customer_name, customer_email, customer_phone,
			inquiry_id, provider_ref, provider_data, callback_url, callback_sent, callback_sent_at,
			callback_attempts, completed_at, failed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19,
			$20, $21, $22, $23, $24, $25,
			$26, $27, $28,
			$29, $30, $31, $32, $33, $34,
			$35, $36, $37
		)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(
		ctx, q,
		p.PayoutID,
		p.ReferenceID,
		p.ClientID,
		p.IsSandbox,
		p.MethodType,
		p.ChannelCode,
		p.TransferType,
		p.Provider,
		p.BankCode,
		p.BankName,
		p.AccountNumber,
		p.AccountName,
		p.SourceBankCode,
		p.SourceAccountNumber,
		p.Amount,
		p.Fee,
		p.SendAmount,
		p.TotalAmount,
		p.FeePaidBy,
		p.Status,
		p.FailedReason,
		p.FailedCode,
		p.PurposeCode,
		p.Remark,
		p.Description,
		p.CustomerName,
		p.CustomerEmail,
		p.CustomerPhone,
		p.InquiryRowID,
		p.ProviderRef,
		nullablePayoutJSON(p.ProviderData),
		p.CallbackURL,
		p.CallbackSent,
		p.CallbackSentAt,
		p.CallbackAttempts,
		p.CompletedAt,
		p.FailedAt,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *PayoutRepository) GetPayoutByPayoutID(ctx context.Context, payoutID string) (*models.Payout, error) {
	q := `SELECT ` + payoutColumns + ` FROM payouts WHERE payout_id = $1 LIMIT 1`
	var p models.Payout
	if err := r.db.GetContext(ctx, &p, q, payoutID); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PayoutRepository) GetPayoutByReferenceID(ctx context.Context, clientID int, referenceID string) (*models.Payout, error) {
	q := `SELECT ` + payoutColumns + ` FROM payouts WHERE client_id = $1 AND reference_id = $2 LIMIT 1`
	var p models.Payout
	if err := r.db.GetContext(ctx, &p, q, clientID, referenceID); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PayoutRepository) GetPayoutByID(ctx context.Context, id int) (*models.Payout, error) {
	q := `SELECT ` + payoutColumns + ` FROM payouts WHERE id = $1 LIMIT 1`
	var p models.Payout
	if err := r.db.GetContext(ctx, &p, q, id); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PayoutRepository) UpdatePayout(ctx context.Context, p *models.Payout) error {
	const q = `
		UPDATE payouts
		SET status = $2,
		    failed_reason = $3,
		    failed_code = $4,
		    fee = $5,
		    send_amount = $6,
		    total_amount = $7,
		    provider = $8,
		    transfer_type = $9,
		    source_bank_code = $10,
		    source_account_number = $11,
		    purpose_code = $12,
		    remark = $13,
		    provider_ref = $14,
		    provider_data = $15,
		    callback_sent = $16,
		    callback_sent_at = $17,
		    callback_attempts = $18,
		    completed_at = $19,
		    failed_at = $20,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`

	return r.db.QueryRowContext(
		ctx, q,
		p.ID,
		p.Status,
		p.FailedReason,
		p.FailedCode,
		p.Fee,
		p.SendAmount,
		p.TotalAmount,
		p.Provider,
		p.TransferType,
		p.SourceBankCode,
		p.SourceAccountNumber,
		p.PurposeCode,
		p.Remark,
		p.ProviderRef,
		nullablePayoutJSON(p.ProviderData),
		p.CallbackSent,
		p.CallbackSentAt,
		p.CallbackAttempts,
		p.CompletedAt,
		p.FailedAt,
	).Scan(&p.UpdatedAt)
}

func (r *PayoutRepository) ListPayoutsForStatusCheck(ctx context.Context, updatedBefore, createdAfter time.Time, limit int) ([]models.Payout, error) {
	q := `SELECT ` + payoutColumns + `
		FROM payouts
		WHERE status = 'Processing'
		  AND updated_at <= $1
		  AND created_at >= $2
		ORDER BY updated_at ASC
		LIMIT $3`
	var rows []models.Payout
	if err := r.db.SelectContext(ctx, &rows, q, updatedBefore, createdAfter, limit); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *PayoutRepository) ListFinalCallbackPending(ctx context.Context, limit int) ([]models.Payout, error) {
	q := `SELECT ` + payoutColumns + `
		FROM payouts
		WHERE status IN ('Success', 'Failed')
		  AND callback_sent = false
		ORDER BY updated_at ASC
		LIMIT $1`
	var rows []models.Payout
	if err := r.db.SelectContext(ctx, &rows, q, limit); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *PayoutRepository) MarkCallbackSent(ctx context.Context, payoutID int) error {
	const q = `
		UPDATE payouts
		SET callback_sent = true,
		    callback_sent_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, q, payoutID)
	return err
}

func (r *PayoutRepository) IncrementCallbackAttempts(ctx context.Context, payoutID int) error {
	const q = `
		UPDATE payouts
		SET callback_attempts = callback_attempts + 1,
		    updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, q, payoutID)
	return err
}

// ListPayouts returns payouts matching the filter plus the total count.
func (r *PayoutRepository) ListPayouts(ctx context.Context, f PayoutFilter, limit, offset int) ([]models.Payout, int, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1
	add := func(clause string, val any) {
		where = append(where, strings.ReplaceAll(clause, "?", fmt.Sprintf("$%d", idx)))
		args = append(args, val)
		idx++
	}
	if f.Status != "" {
		add("status = ?", f.Status)
	}
	if f.MethodType != "" {
		add("method_type = ?", f.MethodType)
	}
	if f.Provider != "" {
		add("provider = ?", f.Provider)
	}
	if f.ChannelCode != "" {
		add("channel_code = ?", f.ChannelCode)
	}
	if f.ClientID > 0 {
		add("client_id = ?", f.ClientID)
	}
	if f.IsSandbox != nil {
		add("is_sandbox = ?", *f.IsSandbox)
	}
	if f.CreatedFrom != nil {
		add("created_at >= ?", *f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		add("created_at <= ?", *f.CreatedTo)
	}
	if f.Search != "" {
		pattern := "%" + f.Search + "%"
		where = append(where, fmt.Sprintf("(payout_id ILIKE $%d OR reference_id ILIKE $%d OR account_number ILIKE $%d OR provider_ref ILIKE $%d)", idx, idx, idx, idx))
		args = append(args, pattern)
		idx++
	}

	whereClause := strings.Join(where, " AND ")
	countQ := `SELECT COUNT(*) FROM payouts WHERE ` + whereClause
	var total int
	if err := r.db.GetContext(ctx, &total, countQ, args...); err != nil {
		return nil, 0, err
	}

	q := `SELECT ` + payoutColumns + ` FROM payouts WHERE ` + whereClause +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows := []models.Payout{}
	if err := r.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Stats returns aggregate counts and volume for the given filter.
func (r *PayoutRepository) Stats(ctx context.Context, f PayoutFilter) (*PayoutStats, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1
	add := func(clause string, val any) {
		where = append(where, strings.ReplaceAll(clause, "?", fmt.Sprintf("$%d", idx)))
		args = append(args, val)
		idx++
	}
	if f.ClientID > 0 {
		add("client_id = ?", f.ClientID)
	}
	if f.IsSandbox != nil {
		add("is_sandbox = ?", *f.IsSandbox)
	}
	if f.CreatedFrom != nil {
		add("created_at >= ?", *f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		add("created_at <= ?", *f.CreatedTo)
	}
	q := `SELECT
        COUNT(*) AS total,
        COUNT(*) FILTER (WHERE status = 'Success') AS total_success,
        COUNT(*) FILTER (WHERE status = 'Processing') AS total_processing,
        0 AS total_pending,
        COUNT(*) FILTER (WHERE status = 'Failed') AS total_failed,
        COALESCE(SUM(total_amount) FILTER (WHERE status = 'Success'), 0) AS total_volume
    FROM payouts WHERE ` + strings.Join(where, " AND ")
	var s PayoutStats
	if err := r.db.GetContext(ctx, &s, q, args...); err != nil {
		return nil, err
	}
	return &s, nil
}

// ---------------------------------------------------------------------------
// Callbacks
// ---------------------------------------------------------------------------

func (r *PayoutRepository) CreatePayoutCallback(ctx context.Context, callback *models.PayoutCallback) error {
	const q = `
		INSERT INTO payout_callbacks (
			provider, provider_ref, headers, payload, signature, is_valid_signature,
			payout_id, status, is_processed, processed_at, process_error
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11
		)
		RETURNING id, created_at`

	return r.db.QueryRowContext(
		ctx, q,
		callback.Provider,
		callback.ProviderRef,
		nullablePayoutJSON(callback.Headers),
		nullablePayoutJSON(callback.Payload),
		callback.Signature,
		callback.IsValidSignature,
		callback.PayoutID,
		callback.Status,
		callback.IsProcessed,
		callback.ProcessedAt,
		callback.ProcessError,
	).Scan(&callback.ID, &callback.CreatedAt)
}

func (r *PayoutRepository) UpdatePayoutCallbackProcessed(ctx context.Context, callbackID int, isProcessed bool, processError *string) error {
	const q = `
		UPDATE payout_callbacks
		SET is_processed = $2,
		    processed_at = CASE WHEN $2 THEN NOW() ELSE NULL END,
		    process_error = $3
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, q, callbackID, isProcessed, processError)
	return err
}

func (r *PayoutRepository) UpdatePayoutCallbackSignature(ctx context.Context, callbackID int, isValidSignature bool) error {
	const q = `
		UPDATE payout_callbacks
		SET is_valid_signature = $2
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, q, callbackID, isValidSignature)
	return err
}

func (r *PayoutRepository) ListCallbacksByPayoutID(ctx context.Context, payoutID string) ([]models.PayoutCallback, error) {
	const q = `
		SELECT id, provider, provider_ref, headers, payload, signature, is_valid_signature,
		       payout_id, status, is_processed, processed_at, process_error, created_at
		FROM payout_callbacks
		WHERE payout_id = $1
		ORDER BY created_at DESC`
	rows := []models.PayoutCallback{}
	if err := r.db.SelectContext(ctx, &rows, q, payoutID); err != nil {
		return nil, err
	}
	return rows, nil
}
