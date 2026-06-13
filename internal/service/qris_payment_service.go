package service

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
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
