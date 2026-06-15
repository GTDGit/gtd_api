package service

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

// QRISPaymentServiceError carries an HTTP status + message for handlers to map
// onto a provider-specific SNAP response code.
type QRISPaymentServiceError struct {
	HTTPStatus int
	Message    string
	Err        error
}

func (e *QRISPaymentServiceError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Message
	}
	return e.Message + ": " + e.Err.Error()
}

func (e *QRISPaymentServiceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// QRISPaymentEvent is the provider-neutral shape a webhook/notify handler builds
// from an inbound successful QRIS payment. StoreID is the identification key.
type QRISPaymentEvent struct {
	Provider           models.QRISProvider
	StoreID            string // matches qris_merchants.store_id
	ReferenceNo        string // provider reference (idempotency key)
	PartnerReferenceNo string // empty for static QR
	RRN                string
	PaymentReferenceNo string
	IssuerID           string
	TerminalID         string
	Amount             int64
	FeeAmount          int64
	NettAmount         int64
	PayerName          string
	PayerPhone         string
	PaidAt             *time.Time
	RawPayload         []byte
}

// QRISPaymentService records successful static-QRIS payments arriving by webhook.
type QRISPaymentService struct {
	merchantRepo *repository.QRISMerchantRepository
	paymentRepo  *repository.QRISPaymentRepository
	callbackSvc  QRISActivationCallback
}

func NewQRISPaymentService(
	merchantRepo *repository.QRISMerchantRepository,
	paymentRepo *repository.QRISPaymentRepository,
) *QRISPaymentService {
	return &QRISPaymentService{
		merchantRepo: merchantRepo,
		paymentRepo:  paymentRepo,
	}
}

// WithCallback wires the client-webhook enqueuer so a newly recorded payment
// fires qris.payment.success. Optional: nil leaves payments un-notified.
func (s *QRISPaymentService) WithCallback(callbackSvc QRISActivationCallback) *QRISPaymentService {
	s.callbackSvc = callbackSvc
	return s
}

// RecordQRISPayment resolves the merchant by (provider, store_id) and inserts the
// payment idempotently. A replayed webhook (same provider reference) is a no-op.
// Returns the persisted payment and whether it was newly inserted.
func (s *QRISPaymentService) RecordQRISPayment(ctx context.Context, e QRISPaymentEvent) (*models.QRISPayment, bool, error) {
	if s.merchantRepo == nil || s.paymentRepo == nil {
		return nil, false, &QRISPaymentServiceError{HTTPStatus: http.StatusServiceUnavailable, Message: "qris payment service unavailable"}
	}
	storeID := strings.TrimSpace(e.StoreID)
	if storeID == "" {
		return nil, false, &QRISPaymentServiceError{HTTPStatus: http.StatusBadRequest, Message: "missing storeId"}
	}
	if strings.TrimSpace(e.ReferenceNo) == "" {
		return nil, false, &QRISPaymentServiceError{HTTPStatus: http.StatusBadRequest, Message: "missing reference number"}
	}

	merchant, err := s.merchantRepo.GetByStore(ctx, e.Provider, storeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, &QRISPaymentServiceError{
				HTTPStatus: http.StatusNotFound,
				Message:    "qris merchant not found for storeId",
			}
		}
		return nil, false, &QRISPaymentServiceError{HTTPStatus: http.StatusInternalServerError, Message: "lookup merchant", Err: err}
	}

	payment := &models.QRISPayment{
		QRISMerchantID:     &merchant.ID,
		Provider:           e.Provider,
		ReferenceNo:        strings.TrimSpace(e.ReferenceNo),
		PartnerReferenceNo: nilIfBlank(e.PartnerReferenceNo),
		RRN:                nilIfBlank(e.RRN),
		PaymentReferenceNo: nilIfBlank(e.PaymentReferenceNo),
		IssuerID:           nilIfBlank(e.IssuerID),
		StoreID:            storeID,
		TerminalID:         nilIfBlank(e.TerminalID),
		Amount:             e.Amount,
		FeeAmount:          nilIfZeroInt64(e.FeeAmount),
		NettAmount:         nilIfZeroInt64(e.NettAmount),
		PayerName:          nilIfBlank(e.PayerName),
		PayerPhone:         nilIfBlank(e.PayerPhone),
		Status:             string(models.PaymentStatusSuccess),
		PaidAt:             e.PaidAt,
		RawPayload:         models.NullableRawMessage(e.RawPayload),
	}

	inserted, err := s.paymentRepo.CreateIfNew(ctx, payment)
	if err != nil {
		return nil, false, &QRISPaymentServiceError{HTTPStatus: http.StatusInternalServerError, Message: "persist qris payment", Err: err}
	}
	if !inserted {
		log.Info().
			Str("provider", string(e.Provider)).
			Str("reference_no", payment.ReferenceNo).
			Msg("qris payment webhook is a duplicate; ignoring")
		return payment, false, nil
	}

	log.Info().
		Str("provider", string(e.Provider)).
		Str("store_id", storeID).
		Str("reference_no", payment.ReferenceNo).
		Int64("amount", payment.Amount).
		Msg("qris payment recorded")

	// Notify the owning client (best-effort, async). The merchant carries the
	// client_id; static QR with no merchant client_id (legacy) is not notified.
	if s.callbackSvc != nil && merchant.ClientID != nil {
		pid := payment.ID
		mid := merchant.ID
		go s.callbackSvc.Enqueue(context.Background(), *merchant.ClientID, models.QRISEventPaymentSuccess, &mid, &pid, payment)
	}
	return payment, true, nil
}

func nilIfBlank(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func nilIfZeroInt64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

// QRISPaymentHistoryItem is the standardized (ASPI-style) payment record returned
// by GET /v1/qris/payments. Amounts are SNAP money strings ("10000.00").
type QRISPaymentHistoryItem struct {
	ReferenceNumber    string  `json:"referenceNumber"`              // provider reference (idempotency key)
	PartnerReferenceNo *string `json:"partnerReferenceNo,omitempty"` // partner reference if any
	OrderID            string  `json:"orderId"`                      // = referenceNumber (client-facing alias)
	StoreID            string  `json:"storeId"`                      // NMID
	SubMerchantID      *string `json:"subMerchantId,omitempty"`      // Nobu MID
	TerminalID         *string `json:"terminalId,omitempty"`
	TransactionDate    string  `json:"transactionDate"` // ISO 8601 (+07:00)
	Amount             string  `json:"amount"`          // 2-decimal money string
	Currency           string  `json:"currency"`        // IDR
	Status             string  `json:"status"`          // SUCCESS
	PayerName          *string `json:"payerName,omitempty"`
}

// ListPaymentsForClient returns the client's QRIS payment history in the
// standardized response shape, plus a total count for pagination.
func (s *QRISPaymentService) ListPaymentsForClient(ctx context.Context, f repository.QRISPaymentFilter) ([]QRISPaymentHistoryItem, int, error) {
	if s.paymentRepo == nil {
		return nil, 0, &QRISPaymentServiceError{HTTPStatus: http.StatusServiceUnavailable, Message: "qris payment service unavailable"}
	}
	rows, total, err := s.paymentRepo.ListForClient(ctx, f)
	if err != nil {
		return nil, 0, &QRISPaymentServiceError{HTTPStatus: http.StatusInternalServerError, Message: "list qris payments", Err: err}
	}
	out := make([]QRISPaymentHistoryItem, 0, len(rows))
	for i := range rows {
		out = append(out, toQRISHistoryItem(&rows[i]))
	}
	return out, total, nil
}

func toQRISHistoryItem(p *models.QRISPayment) QRISPaymentHistoryItem {
	wib := time.FixedZone("WIB", 7*3600)
	txTime := p.CreatedAt
	if p.PaidAt != nil {
		txTime = *p.PaidAt
	}
	item := QRISPaymentHistoryItem{
		ReferenceNumber:    p.ReferenceNo,
		PartnerReferenceNo: p.PartnerReferenceNo,
		OrderID:            p.ReferenceNo,
		StoreID:            p.StoreID,
		TerminalID:         p.TerminalID,
		TransactionDate:    txTime.In(wib).Format("2006-01-02T15:04:05-07:00"),
		Amount:             formatSNAPMoney(p.Amount),
		Currency:           "IDR",
		Status:             "SUCCESS",
		PayerName:          p.PayerName,
	}
	if p.PartnerReferenceNo != nil && *p.PartnerReferenceNo != "" {
		item.OrderID = *p.PartnerReferenceNo
	}
	return item
}

// formatSNAPMoney renders whole-rupiah units as a 2-decimal SNAP money string.
func formatSNAPMoney(v int64) string {
	return strconv.FormatInt(v, 10) + ".00"
}
