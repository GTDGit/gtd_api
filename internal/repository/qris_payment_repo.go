package repository

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

type QRISPaymentRepository struct {
	db *sqlx.DB
}

func NewQRISPaymentRepository(db *sqlx.DB) *QRISPaymentRepository {
	return &QRISPaymentRepository{db: db}
}

func nullableQRISJSON(v models.NullableRawMessage) any {
	if len(v) == 0 {
		return nil
	}
	return []byte(v)
}

// CreateIfNew inserts a QRIS payment, treating (provider, reference_no) as an
// idempotency key. It reports inserted=false when a row with the same provider
// reference already exists, so a replayed webhook is a safe no-op.
func (r *QRISPaymentRepository) CreateIfNew(ctx context.Context, p *models.QRISPayment) (inserted bool, err error) {
	const q = `INSERT INTO qris_payments (
        qris_merchant_id, provider, reference_no, partner_reference_no, rrn,
        payment_reference_no, issuer_id, store_id, terminal_id,
        amount, fee_amount, nett_amount, payer_name, payer_phone,
        status, paid_at, raw_payload
    ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
    )
    ON CONFLICT (provider, reference_no) DO NOTHING
    RETURNING id, created_at`

	err = r.db.QueryRowContext(ctx, q,
		p.QRISMerchantID, p.Provider, p.ReferenceNo, p.PartnerReferenceNo, p.RRN,
		p.PaymentReferenceNo, p.IssuerID, p.StoreID, p.TerminalID,
		p.Amount, p.FeeAmount, p.NettAmount, p.PayerName, p.PayerPhone,
		p.Status, p.PaidAt, nullableQRISJSON(p.RawPayload),
	).Scan(&p.ID, &p.CreatedAt)

	if err == sql.ErrNoRows {
		// ON CONFLICT DO NOTHING returned no row → duplicate reference.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
