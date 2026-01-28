package repository

import (
    "database/sql"
    "fmt"

    "github.com/jmoiron/sqlx"

    "github.com/GTDGit/gtd_api/internal/models"
)

// TransactionRepository handles data access for transactions.
type TransactionRepository struct {
    db *sqlx.DB
}

// NewTransactionRepository creates a new TransactionRepository.
func NewTransactionRepository(db *sqlx.DB) *TransactionRepository {
    return &TransactionRepository{db: db}
}

// Create inserts a new transaction row.
func (r *TransactionRepository) Create(trx *models.Transaction) error {
    const q = `
        INSERT INTO transactions (
            transaction_id, reference_id, client_id, product_id, sku_id, is_sandbox,
            customer_no, customer_name, type, status, serial_number, amount, admin,
            period, description, failed_reason, retry_count, next_retry_at, expired_at,
            inquiry_id, digi_ref_id, created_at, processed_at
        ) VALUES (
            $1,$2,$3,$4,$5,$6,
            $7,$8,$9,$10,$11,$12,$13,
            $14,$15,$16,$17,$18,$19,
            $20,$21,NOW(),$22
        )`

    stmt, err := r.db.Preparex(q)
    if err != nil {
        return err
    }
    defer stmt.Close()

    _, err = stmt.Exec(
        trx.TransactionID, trx.ReferenceID, trx.ClientID, trx.ProductID, trx.SkuID, trx.IsSandbox,
        trx.CustomerNo, trx.CustomerName, trx.Type, trx.Status, trx.SerialNumber, trx.Amount, trx.Admin,
        trx.Period, trx.Description, trx.FailedReason, trx.RetryCount, trx.NextRetryAt, trx.ExpiredAt,
        trx.InquiryID, trx.DigiRefID, trx.ProcessedAt,
    )
    return err
}

// Update updates an existing transaction identified by transaction_id.
func (r *TransactionRepository) Update(trx *models.Transaction) error {
    const q = `
        UPDATE transactions SET
            sku_id = $2,
            status = $3,
            serial_number = $4,
            amount = $5,
            admin = $6,
            period = $7,
            description = $8,
            failed_reason = $9,
            retry_count = $10,
            next_retry_at = $11,
            expired_at = $12,
            inquiry_id = $13,
            digi_ref_id = $14,
            processed_at = $15,
            updated_at = NOW()
        WHERE transaction_id = $1`

    stmt, err := r.db.Preparex(q)
    if err != nil {
        return err
    }
    defer stmt.Close()

    _, err = stmt.Exec(
        trx.TransactionID,
        trx.SkuID,
        trx.Status,
        trx.SerialNumber,
        trx.Amount,
        trx.Admin,
        trx.Period,
        trx.Description,
        trx.FailedReason,
        trx.RetryCount,
        trx.NextRetryAt,
        trx.ExpiredAt,
        trx.InquiryID,
        trx.DigiRefID,
        trx.ProcessedAt,
    )
    return err
}

// GetByTransactionID returns transaction by transaction_id.
func (r *TransactionRepository) GetByTransactionID(transactionID string) (*models.Transaction, error) {
    const q = `SELECT * FROM transactions WHERE transaction_id = $1 LIMIT 1`
    stmt, err := r.db.Preparex(q)
    if err != nil {
        return nil, err
    }
    defer stmt.Close()
    var t models.Transaction
    if err := stmt.Get(&t, transactionID); err != nil {
        if err == sql.ErrNoRows {
            return nil, sql.ErrNoRows
        }
        return nil, err
    }
    return &t, nil
}

// GetByReferenceID returns transaction by client_id and reference_id.
func (r *TransactionRepository) GetByReferenceID(clientID int, referenceID string) (*models.Transaction, error) {
    const q = `SELECT * FROM transactions WHERE client_id = $1 AND reference_id = $2 LIMIT 1`
    stmt, err := r.db.Preparex(q)
    if err != nil {
        return nil, err
    }
    defer stmt.Close()
    var t models.Transaction
    if err := stmt.Get(&t, clientID, referenceID); err != nil {
        if err == sql.ErrNoRows {
            return nil, sql.ErrNoRows
        }
        return nil, err
    }
    return &t, nil
}

// GetByDigiRefID returns transaction by digi_ref_id.
func (r *TransactionRepository) GetByDigiRefID(digiRefID string) (*models.Transaction, error) {
    const q = `SELECT * FROM transactions WHERE digi_ref_id = $1 LIMIT 1`
    stmt, err := r.db.Preparex(q)
    if err != nil {
        return nil, err
    }
    defer stmt.Close()
    var t models.Transaction
    if err := stmt.Get(&t, digiRefID); err != nil {
        if err == sql.ErrNoRows {
            return nil, sql.ErrNoRows
        }
        return nil, err
    }
    return &t, nil
}

// GetPendingForRetry selects pending transactions due for retry.
// Note: FOR UPDATE SKIP LOCKED prevents multiple workers reprocessing the same rows.
func (r *TransactionRepository) GetPendingForRetry() ([]models.Transaction, error) {
    const q = `
        SELECT * FROM transactions
        WHERE status = 'Pending'
          AND next_retry_at <= NOW()
        ORDER BY next_retry_at ASC
        FOR UPDATE SKIP LOCKED`

    // Use a transaction to hold row locks if caller intends to process immediately.
    // Here we just prepare and select; caller should manage tx if needed.
    stmt, err := r.db.Preparex(q)
    if err != nil {
        return nil, err
    }
    defer stmt.Close()
    var list []models.Transaction
    if err := stmt.Select(&list); err != nil {
        return nil, err
    }
    return list, nil
}

// ExistsReferenceID checks if a client has already used a reference_id.
func (r *TransactionRepository) ExistsReferenceID(clientID int, referenceID string) (bool, error) {
    const q = `SELECT EXISTS(SELECT 1 FROM transactions WHERE client_id = $1 AND reference_id = $2)`
    stmt, err := r.db.Preparex(q)
    if err != nil {
        return false, err
    }
    defer stmt.Close()
    var exists bool
    if err := stmt.Get(&exists, clientID, referenceID); err != nil {
        return false, err
    }
    return exists, nil
}

// GenerateTransactionID returns an ID like GRB-YYYYMMDD-NNNNNN using Asia/Jakarta date.
func (r *TransactionRepository) GenerateTransactionID() (string, error) {
    // Determine today's date in Asia/Jakarta within the DB and compute next sequence.
    const seqQ = `
        SELECT COALESCE(MAX(
            CAST(SUBSTRING(transaction_id FROM 14) AS INT)
        ), 0) + 1 
        FROM transactions
        WHERE transaction_id LIKE 'GRB-' || TO_CHAR(NOW() AT TIME ZONE 'Asia/Jakarta', 'YYYYMMDD') || '-%'`

    stmt, err := r.db.Preparex(seqQ)
    if err != nil {
        return "", err
    }
    defer stmt.Close()
    var next int
    if err := stmt.Get(&next); err != nil {
        return "", err
    }

    // Get date string in Asia/Jakarta from DB to avoid TZ mismatches.
    const dateQ = `SELECT TO_CHAR(NOW() AT TIME ZONE 'Asia/Jakarta', 'YYYYMMDD')`
    var ymd string
    if err := r.db.Get(&ymd, dateQ); err != nil {
        return "", err
    }
    return fmt.Sprintf("GRB-%s-%06d", ymd, next), nil
}

// GetInquiryForPayment returns the inquiry transaction eligible for linking to a payment.
// Validations: type='inquiry', status='Success', expired_at > NOW(), and not already used by a payment.
func (r *TransactionRepository) GetInquiryForPayment(transactionID string) (*models.Transaction, error) {
    const q = `
        SELECT * FROM transactions t
        WHERE t.transaction_id = $1
          AND t.type = 'inquiry'
          AND t.status = 'Success'
          AND t.expired_at > NOW()
          AND NOT EXISTS (
              SELECT 1 FROM transactions p WHERE p.inquiry_id = t.id
          )
        LIMIT 1`

    stmt, err := r.db.Preparex(q)
    if err != nil {
        return nil, err
    }
    defer stmt.Close()
    var inq models.Transaction
    if err := stmt.Get(&inq, transactionID); err != nil {
        if err == sql.ErrNoRows {
            return nil, sql.ErrNoRows
        }
        return nil, err
    }
    return &inq, nil
}

// MarkInquiryAsPaid links an inquiry to a payment by setting payment.inquiry_id = inquiryID.
func (r *TransactionRepository) MarkInquiryAsPaid(inquiryID int, paymentID int) error {
    const q = `UPDATE transactions SET inquiry_id = $1, updated_at = NOW() WHERE id = $2`
    stmt, err := r.db.Preparex(q)
    if err != nil {
        return err
    }
    defer stmt.Close()
    _, err = stmt.Exec(inquiryID, paymentID)
    return err
}
