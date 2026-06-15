package repository

import (
	"context"
	"database/sql"
	"strconv"
	"time"

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

// QRISPaymentFilter narrows a client-scoped payment-history query. ClientID is
// mandatory (enforced via a join on qris_merchants.client_id).
type QRISPaymentFilter struct {
	ClientID      int
	StoreID       string // qris_merchants.store_id (NMID)
	SubMerchantID string // qris_merchants.sub_merchant_id (Nobu MID)
	OrderID       string // matches reference_no or partner_reference_no
	From          *time.Time
	To            *time.Time
	Limit         int
	Offset        int
}

const qrisPaymentColumns = `p.id, p.qris_merchant_id, p.provider, p.reference_no, p.partner_reference_no,
    p.rrn, p.payment_reference_no, p.issuer_id, p.store_id, p.terminal_id, p.amount,
    p.fee_amount, p.nett_amount, p.payer_name, p.payer_phone, p.status, p.paid_at,
    p.raw_payload, p.created_at`

// ListForClient returns the client's QRIS payments (newest first) plus a total
// count. Only payments for merchants owned by the client are visible.
func (r *QRISPaymentRepository) ListForClient(ctx context.Context, f QRISPaymentFilter) ([]models.QRISPayment, int, error) {
	where := ` FROM qris_payments p
	           JOIN qris_merchants m ON m.id = p.qris_merchant_id
	           WHERE m.client_id = $1`
	args := []any{f.ClientID}
	n := 2
	if f.StoreID != "" {
		where += ` AND p.store_id = $` + strconv.Itoa(n)
		args = append(args, f.StoreID)
		n++
	}
	if f.SubMerchantID != "" {
		where += ` AND m.sub_merchant_id = $` + strconv.Itoa(n)
		args = append(args, f.SubMerchantID)
		n++
	}
	if f.OrderID != "" {
		where += ` AND (p.reference_no = $` + strconv.Itoa(n) + ` OR p.partner_reference_no = $` + strconv.Itoa(n) + `)`
		args = append(args, f.OrderID)
		n++
	}
	if f.From != nil {
		where += ` AND p.created_at >= $` + strconv.Itoa(n)
		args = append(args, *f.From)
		n++
	}
	if f.To != nil {
		where += ` AND p.created_at <= $` + strconv.Itoa(n)
		args = append(args, *f.To)
		n++
	}

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*)`+where, args...); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	q := `SELECT ` + qrisPaymentColumns + where +
		` ORDER BY p.created_at DESC, p.id DESC LIMIT $` + strconv.Itoa(n) + ` OFFSET $` + strconv.Itoa(n+1)
	args = append(args, limit, f.Offset)

	var items []models.QRISPayment
	if err := r.db.SelectContext(ctx, &items, q, args...); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
