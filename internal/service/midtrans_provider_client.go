package service

import (
	"context"
	"strings"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/midtrans"
)

// MidtransProviderClient implements PaymentProviderClient using Midtrans Core
// API for GoPay and ShopeePay e-wallets.
type MidtransProviderClient struct {
	client      *midtrans.Client
	callbackURL string
}

func NewMidtransProviderClient(client *midtrans.Client, callbackURL string) *MidtransProviderClient {
	return &MidtransProviderClient{client: client, callbackURL: callbackURL}
}

func (p *MidtransProviderClient) Code() models.PaymentProvider {
	return models.ProviderMidtrans
}

// Available reports whether the adapter is configured to serve requests.
func (p *MidtransProviderClient) Available() bool {
	return true
}

func (p *MidtransProviderClient) CreatePayment(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	cust := &midtrans.CustomerDetails{
		FirstName: firstNonEmpty(req.CustomerName, "Customer"),
		Email:     req.CustomerEmail,
		Phone:     req.CustomerPhone,
	}
	var resp *midtrans.ChargeResponse
	var err error
	code := strings.ToUpper(strings.TrimSpace(method.Code))

	switch req.Type {
	case models.PaymentTypeQRIS:
		// Use native QRIS — returns qr_string directly in response body
		resp, err = p.client.ChargeQRIS(ctx, req.PartnerRef, req.TotalAmount, "gopay", cust)
		if err != nil {
			return nil, mapMidtransError(err)
		}
		norm := PaymentDetailNormalized{
			QRString: resp.QRString,
		}
		return &PaymentCreateResponse{
			ProviderRef: resp.TransactionID,
			Normalized:  norm,
			RawResponse: resp.RawResponse,
		}, nil

	case models.PaymentTypeEwallet:
		switch code {
		case "GOPAY", "PAYGOPAY":
			resp, err = p.client.ChargeGoPay(ctx, req.PartnerRef, req.TotalAmount, firstNonEmpty(req.CallbackURL, p.callbackURL), cust)
		case "SHOPEEPAY", "PAYSHOPEE":
			resp, err = p.client.ChargeShopeePay(ctx, req.PartnerRef, req.TotalAmount, firstNonEmpty(req.CallbackURL, p.callbackURL), cust)
		default:
			return nil, newPaymentError(400, "UNSUPPORTED_PAYMENT_TYPE", "Unsupported e-wallet code for Midtrans: "+code, nil)
		}
		if err != nil {
			return nil, mapMidtransError(err)
		}
		norm := PaymentDetailNormalized{}
		switch code {
		case "GOPAY", "PAYGOPAY":
			norm.QRCodeURL = resp.Action("generate-qr-code")
			norm.Deeplink = resp.Action("deeplink-redirect")
		case "SHOPEEPAY", "PAYSHOPEE":
			norm.Deeplink = resp.Action("deeplink-redirect")
		}
		return &PaymentCreateResponse{
			ProviderRef: resp.TransactionID,
			Normalized:  norm,
			RawResponse: resp.RawResponse,
		}, nil

	default:
		return nil, newPaymentError(400, "UNSUPPORTED_PAYMENT_TYPE", "Midtrans adapter supports QRIS and e-wallet payments only", nil)
	}
}

func (p *MidtransProviderClient) InquiryPayment(ctx context.Context, payment *models.Payment) (*PaymentInquiryResult, error) {
	resp, err := p.client.Status(ctx, payment.PaymentID)
	if err != nil {
		return nil, mapMidtransError(err)
	}
	return &PaymentInquiryResult{
		Status:      mapMidtransTransactionStatus(resp.TransactionStatus, resp.FraudStatus),
		ProviderRef: resp.TransactionID,
		PaidAmount:  payment.Amount,
		RawResponse: resp.RawResponse,
	}, nil
}

func (p *MidtransProviderClient) CancelPayment(ctx context.Context, payment *models.Payment, reason string) (*PaymentCancelResult, error) {
	resp, err := p.client.Cancel(ctx, payment.PaymentID)
	if err != nil {
		return nil, mapMidtransError(err)
	}
	return &PaymentCancelResult{Cancelled: true, RawResponse: resp.RawResponse}, nil
}

func mapMidtransTransactionStatus(status, fraudStatus string) models.PaymentStatus {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case midtrans.StatusSettlement, midtrans.StatusCapture:
		if strings.EqualFold(fraudStatus, "challenge") {
			return models.PaymentStatusPending
		}
		return models.PaymentStatusPaid
	case midtrans.StatusDeny:
		return models.PaymentStatusFailed
	case midtrans.StatusExpire:
		return models.PaymentStatusExpired
	case midtrans.StatusCancel:
		return models.PaymentStatusCancelled
	case midtrans.StatusRefund:
		return models.PaymentStatusRefunded
	case "partial_refund":
		return models.PaymentStatusPartialRefund
	default:
		return models.PaymentStatusPending
	}
}

func mapMidtransError(err error) error {
	if err == nil {
		return nil
	}
	apiErr, ok := err.(*midtrans.APIError)
	if ok {
		if apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			return newPaymentError(400, "PROVIDER_REQUEST_REJECTED", firstNonEmpty(apiErr.StatusMessage, "Provider rejected request"), err)
		}
		if apiErr.HTTPStatus >= 500 {
			return newPaymentError(503, "PROVIDER_UNAVAILABLE", "Payment provider temporarily unavailable", err)
		}
	}
	return newPaymentError(502, "PROVIDER_ERROR", "Payment provider error", err)
}
