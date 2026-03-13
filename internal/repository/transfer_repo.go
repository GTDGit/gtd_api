package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

type TransferRepository struct {
	db *sqlx.DB
}

func nullableTransferJSON(v models.NullableRawMessage) any {
	if len(v) == 0 {
		return nil
	}
	return []byte(v)
}

func NewTransferRepository(db *sqlx.DB) *TransferRepository {
	return &TransferRepository{db: db}
}

func (r *TransferRepository) CreateInquiry(ctx context.Context, inquiry *models.TransferInquiry) error {
	const q = `
		INSERT INTO transfer_inquiries (
			inquiry_id, client_id, is_sandbox, bank_code, bank_name, account_number, account_name,
			transfer_type, provider, provider_ref, provider_data, expired_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12
		)
		RETURNING id, created_at`

	return r.db.QueryRowContext(
		ctx,
		q,
		inquiry.InquiryID,
		inquiry.ClientID,
		inquiry.IsSandbox,
		inquiry.BankCode,
		inquiry.BankName,
		inquiry.AccountNumber,
		inquiry.AccountName,
		inquiry.TransferType,
		inquiry.Provider,
		inquiry.ProviderRef,
		nullableTransferJSON(inquiry.ProviderData),
		inquiry.ExpiredAt,
	).Scan(&inquiry.ID, &inquiry.CreatedAt)
}

func (r *TransferRepository) GetInquiryByInquiryID(ctx context.Context, inquiryID string) (*models.TransferInquiry, error) {
	const q = `
		SELECT id, inquiry_id, client_id, is_sandbox, bank_code, bank_name, account_number, account_name,
		       transfer_type, provider, provider_ref, provider_data, expired_at, created_at
		FROM transfer_inquiries
		WHERE inquiry_id = $1
		LIMIT 1`

	var inquiry models.TransferInquiry
	if err := r.db.GetContext(ctx, &inquiry, q, inquiryID); err != nil {
		return nil, err
	}
	return &inquiry, nil
}

func (r *TransferRepository) CreateTransfer(ctx context.Context, transfer *models.Transfer) error {
	const q = `
		INSERT INTO transfers (
			transfer_id, reference_id, client_id, is_sandbox, transfer_type, provider,
			bank_code, bank_name, account_number, account_name, source_bank_code, source_account_number,
			amount, fee, total_amount, status, failed_reason, failed_code, purpose_code, remark,
			inquiry_id, provider_ref, provider_data, callback_sent, callback_sent_at, completed_at, failed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24, $25, $26, $27
		)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(
		ctx,
		q,
		transfer.TransferID,
		transfer.ReferenceID,
		transfer.ClientID,
		transfer.IsSandbox,
		transfer.TransferType,
		transfer.Provider,
		transfer.BankCode,
		transfer.BankName,
		transfer.AccountNumber,
		transfer.AccountName,
		transfer.SourceBankCode,
		transfer.SourceAccountNumber,
		transfer.Amount,
		transfer.Fee,
		transfer.TotalAmount,
		transfer.Status,
		transfer.FailedReason,
		transfer.FailedCode,
		transfer.PurposeCode,
		transfer.Remark,
		transfer.InquiryRowID,
		transfer.ProviderRef,
		nullableTransferJSON(transfer.ProviderData),
		transfer.CallbackSent,
		transfer.CallbackSentAt,
		transfer.CompletedAt,
		transfer.FailedAt,
	).Scan(&transfer.ID, &transfer.CreatedAt, &transfer.UpdatedAt)
}

func (r *TransferRepository) GetTransferByTransferID(ctx context.Context, transferID string) (*models.Transfer, error) {
	const q = `
		SELECT id, transfer_id, reference_id, client_id, is_sandbox, transfer_type, provider,
		       bank_code, bank_name, account_number, account_name, source_bank_code, source_account_number,
		       amount, fee, total_amount, status, failed_reason, failed_code, purpose_code, remark,
		       inquiry_id, provider_ref, provider_data, callback_sent, callback_sent_at, created_at,
		       completed_at, failed_at, updated_at
		FROM transfers
		WHERE transfer_id = $1
		LIMIT 1`

	var transfer models.Transfer
	if err := r.db.GetContext(ctx, &transfer, q, transferID); err != nil {
		return nil, err
	}
	return &transfer, nil
}

func (r *TransferRepository) GetTransferByReferenceID(ctx context.Context, clientID int, referenceID string) (*models.Transfer, error) {
	const q = `
		SELECT id, transfer_id, reference_id, client_id, is_sandbox, transfer_type, provider,
		       bank_code, bank_name, account_number, account_name, source_bank_code, source_account_number,
		       amount, fee, total_amount, status, failed_reason, failed_code, purpose_code, remark,
		       inquiry_id, provider_ref, provider_data, callback_sent, callback_sent_at, created_at,
		       completed_at, failed_at, updated_at
		FROM transfers
		WHERE client_id = $1 AND reference_id = $2
		LIMIT 1`

	var transfer models.Transfer
	if err := r.db.GetContext(ctx, &transfer, q, clientID, referenceID); err != nil {
		return nil, err
	}
	return &transfer, nil
}

func (r *TransferRepository) UpdateTransfer(ctx context.Context, transfer *models.Transfer) error {
	const q = `
		UPDATE transfers
		SET status = $2,
		    failed_reason = $3,
		    failed_code = $4,
		    fee = $5,
		    total_amount = $6,
		    purpose_code = $7,
		    remark = $8,
		    provider_ref = $9,
		    provider_data = $10,
		    callback_sent = $11,
		    callback_sent_at = $12,
		    completed_at = $13,
		    failed_at = $14,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`

	return r.db.QueryRowContext(
		ctx,
		q,
		transfer.ID,
		transfer.Status,
		transfer.FailedReason,
		transfer.FailedCode,
		transfer.Fee,
		transfer.TotalAmount,
		transfer.PurposeCode,
		transfer.Remark,
		transfer.ProviderRef,
		nullableTransferJSON(transfer.ProviderData),
		transfer.CallbackSent,
		transfer.CallbackSentAt,
		transfer.CompletedAt,
		transfer.FailedAt,
	).Scan(&transfer.UpdatedAt)
}

func (r *TransferRepository) ListTransfersForStatusCheck(ctx context.Context, updatedBefore, createdAfter time.Time, limit int) ([]models.Transfer, error) {
	const q = `
		SELECT id, transfer_id, reference_id, client_id, is_sandbox, transfer_type, provider,
		       bank_code, bank_name, account_number, account_name, source_bank_code, source_account_number,
		       amount, fee, total_amount, status, failed_reason, failed_code, purpose_code, remark,
		       inquiry_id, provider_ref, provider_data, callback_sent, callback_sent_at, created_at,
		       completed_at, failed_at, updated_at
		FROM transfers
		WHERE status IN ('Processing', 'Pending')
		  AND updated_at <= $1
		  AND created_at >= $2
		ORDER BY updated_at ASC
		LIMIT $3`

	var transfers []models.Transfer
	if err := r.db.SelectContext(ctx, &transfers, q, updatedBefore, createdAfter, limit); err != nil {
		return nil, err
	}
	return transfers, nil
}

func (r *TransferRepository) ListFinalCallbackPending(ctx context.Context, limit int) ([]models.Transfer, error) {
	const q = `
		SELECT id, transfer_id, reference_id, client_id, is_sandbox, transfer_type, provider,
		       bank_code, bank_name, account_number, account_name, source_bank_code, source_account_number,
		       amount, fee, total_amount, status, failed_reason, failed_code, purpose_code, remark,
		       inquiry_id, provider_ref, provider_data, callback_sent, callback_sent_at, created_at,
		       completed_at, failed_at, updated_at
		FROM transfers
		WHERE status IN ('Success', 'Failed')
		  AND callback_sent = false
		ORDER BY updated_at ASC
		LIMIT $1`

	var transfers []models.Transfer
	if err := r.db.SelectContext(ctx, &transfers, q, limit); err != nil {
		return nil, err
	}
	return transfers, nil
}

func (r *TransferRepository) MarkCallbackSent(ctx context.Context, transferID int) error {
	const q = `
		UPDATE transfers
		SET callback_sent = true,
		    callback_sent_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, q, transferID)
	return err
}

func (r *TransferRepository) CreateTransferCallback(ctx context.Context, callback *models.TransferCallback) error {
	const q = `
		INSERT INTO transfer_callbacks (
			provider, provider_ref, headers, payload, signature, is_valid_signature,
			transfer_id, status, is_processed, processed_at, process_error
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11
		)
		RETURNING id, created_at`

	return r.db.QueryRowContext(
		ctx,
		q,
		callback.Provider,
		callback.ProviderRef,
		nullableTransferJSON(callback.Headers),
		nullableTransferJSON(callback.Payload),
		callback.Signature,
		callback.IsValidSignature,
		callback.TransferID,
		callback.Status,
		callback.IsProcessed,
		callback.ProcessedAt,
		callback.ProcessError,
	).Scan(&callback.ID, &callback.CreatedAt)
}

func (r *TransferRepository) UpdateTransferCallbackProcessed(
	ctx context.Context,
	callbackID int,
	isProcessed bool,
	processError *string,
) error {
	const q = `
		UPDATE transfer_callbacks
		SET is_processed = $2,
		    processed_at = CASE WHEN $2 THEN NOW() ELSE NULL END,
		    process_error = $3
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, q, callbackID, isProcessed, processError)
	return err
}

func (r *TransferRepository) UpdateTransferCallbackSignature(
	ctx context.Context,
	callbackID int,
	isValidSignature bool,
) error {
	const q = `
		UPDATE transfer_callbacks
		SET is_valid_signature = $2
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, q, callbackID, isValidSignature)
	return err
}
