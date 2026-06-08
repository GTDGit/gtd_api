package service

import (
	"context"
	"strings"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/ovo"
)

// OVOProviderClient implements PaymentProviderClient for OVO Direct using the
// OVO partner push-to-pay payment API (pkg/ovo).
//
// When credentials are absent the client is nil and Available() reports false,
// letting ProviderSelector fall back to other OVO-capable providers
// (pakailink, dana_direct, xendit) per Req 13.3.
type OVOProviderClient struct {
	client          *ovo.Client // nil when unconfigured
	notificationURL string
}

// NewOVOProviderClient wraps a configured pkg/ovo client. Pass a nil client to
// construct an unconfigured (unavailable) adapter.
func NewOVOProviderClient(client *ovo.Client, notificationURL string) *OVOProviderClient {
	return &OVOProviderClient{client: client, notificationURL: notificationURL}
}

func (p *OVOProviderClient) Code() models.PaymentProvider {
	return models.ProviderOVODirect
}

// Available reports whether OVO Direct credentials are configured. The
// ProviderSelector uses this to skip OVO Direct and fall back to other OVO
// providers when no client is wired (Req 13.3).
func (p *OVOProviderClient) Available() bool {
	return p.client != nil
}

// CreatePayment pushes a push-to-pay request to the customer's OVO app
// (Req 13.1). OVO returns a PENDING transaction; the customer approves in-app
// and OVO later POSTs an async notification to /v1/webhook/ovo.
func (p *OVOProviderClient) CreatePayment(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	if p.client == nil {
		return nil, newPaymentError(503, "PROVIDER_UNAVAILABLE", "OVO Direct is not configured", nil)
	}
	if req.Type != models.PaymentTypeEwallet {
		return nil, newPaymentError(400, "UNSUPPORTED_PAYMENT_TYPE", "OVO Direct supports e-wallet payments only", nil)
	}
	phone := strings.TrimSpace(req.CustomerPhone)
	if phone == "" {
		return nil, newPaymentError(400, "MISSING_FIELD", "customer phone (OVO MSISDN) is required for OVO Direct", nil)
	}

	pushReq := ovo.PushPaymentRequest{
		PartnerReferenceNo: req.PartnerRef,
		Phone:              phone,
		Amount:             req.TotalAmount,
		Currency:           "IDR",
		Description:        firstNonEmpty(req.Description, method.Name),
		ExpiredAt:          formatXenditExpiry(req.ExpiredAt), // RFC3339 UTC
		NotificationURL:    firstNonEmpty(req.CallbackURL, p.notificationURL),
	}
	resp, err := p.client.PushPayment(ctx, pushReq)
	if err != nil {
		return nil, mapOVOError(err)
	}

	// EWALLET normalized detail: surface any deeplink/checkout URL OVO returns
	// so the customer can approve the push. For a silent in-app push these stay
	// empty and the transaction remains pending until the notification arrives.
	norm := PaymentDetailNormalized{
		Deeplink:    resp.Deeplink,
		CheckoutURL: resp.CheckoutURL,
	}
	return &PaymentCreateResponse{
		ProviderRef: resp.ReferenceNo,
		Normalized:  norm,
		RawResponse: resp.RawResponse,
	}, nil
}

// InquiryPayment reconciles the transaction status via the OVO status endpoint
// (Req 13.2).
func (p *OVOProviderClient) InquiryPayment(ctx context.Context, payment *models.Payment) (*PaymentInquiryResult, error) {
	if p.client == nil {
		return nil, newPaymentError(503, "PROVIDER_UNAVAILABLE", "OVO Direct is not configured", nil)
	}
	statusReq := ovo.StatusRequest{
		PartnerReferenceNo: payment.PaymentID,
	}
	if payment.ProviderRef != nil {
		statusReq.ReferenceNo = *payment.ProviderRef
	}
	resp, err := p.client.QueryStatus(ctx, statusReq)
	if err != nil {
		return nil, mapOVOError(err)
	}
	paid := resp.Amount
	if paid == 0 {
		paid = payment.Amount
	}
	return &PaymentInquiryResult{
		Status:      mapOVOStatus(resp.TransactionStatus),
		ProviderRef: resp.ReferenceNo,
		PaidAmount:  paid,
		RawResponse: resp.RawResponse,
	}, nil
}

// CancelPayment voids a pending OVO transaction.
func (p *OVOProviderClient) CancelPayment(ctx context.Context, payment *models.Payment, reason string) (*PaymentCancelResult, error) {
	if p.client == nil {
		return &PaymentCancelResult{Cancelled: true}, nil
	}
	voidReq := ovo.VoidRequest{
		PartnerReferenceNo: payment.PaymentID,
		Reason:             firstNonEmpty(reason, "Customer cancellation"),
	}
	if payment.ProviderRef != nil {
		voidReq.ReferenceNo = *payment.ProviderRef
	}
	resp, err := p.client.Void(ctx, voidReq)
	if err != nil {
		return nil, mapOVOError(err)
	}
	return &PaymentCancelResult{Cancelled: true, RawResponse: resp.RawResponse}, nil
}

func mapOVOStatus(status string) models.PaymentStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case ovo.StatusSuccess, "PAID", "COMPLETED", "00":
		return models.PaymentStatusPaid
	case ovo.StatusExpired:
		return models.PaymentStatusExpired
	case ovo.StatusVoided, "CANCELLED", "CANCELED":
		return models.PaymentStatusCancelled
	case ovo.StatusFailed:
		return models.PaymentStatusFailed
	default:
		return models.PaymentStatusPending
	}
}

func mapOVOError(err error) error {
	if err == nil {
		return nil
	}
	if apiErr, ok := err.(*ovo.APIError); ok {
		if apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			return newPaymentError(400, "PROVIDER_REQUEST_REJECTED", firstNonEmpty(apiErr.Message, "Provider rejected request"), err)
		}
		if apiErr.HTTPStatus >= 500 {
			return newPaymentError(503, "PROVIDER_UNAVAILABLE", "Payment provider temporarily unavailable", err)
		}
	}
	return newPaymentError(502, "PROVIDER_ERROR", "Payment provider error", err)
}
