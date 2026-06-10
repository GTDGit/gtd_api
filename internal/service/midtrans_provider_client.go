package service

import (
	"context"
	"strings"
	"time"

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
			resp, err = p.client.ChargeGoPay(ctx, req.PartnerRef, req.TotalAmount, p.callbackURL, cust)
		case "SHOPEEPAY", "PAYSHOPEE":
			resp, err = p.client.ChargeShopeePay(ctx, req.PartnerRef, req.TotalAmount, p.callbackURL, cust)
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

	case models.PaymentTypeVA:
		return p.createVA(ctx, method, req, cust)

	default:
		return nil, newPaymentError(400, "UNSUPPORTED_PAYMENT_TYPE", "Midtrans adapter supports QRIS, e-wallet, and VA payments", nil)
	}
}

// createVA issues a Virtual Account charge. Mandiri (008) uses Midtrans echannel
// (Bill Payment); the other supported banks use bank_transfer.
func (p *MidtransProviderClient) createVA(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest, cust *midtrans.CustomerDetails) (*PaymentCreateResponse, error) {
	bank := midtransVABank(method.Code)
	if bank == "" {
		return nil, newPaymentError(400, "UNSUPPORTED_PAYMENT_TYPE", "Unsupported VA bank code for Midtrans: "+method.Code, nil)
	}

	expirySec := midtransExpirySeconds(req.ExpiredAt)
	var resp *midtrans.ChargeResponse
	var err error
	if bank == "echannel" {
		// Mandiri Bill Payment — bill_info defaults applied inside the client.
		resp, err = p.client.ChargeEchannel(ctx, req.PartnerRef, req.TotalAmount, firstNonEmpty(req.Description, "Payment"), "", expirySec, cust)
	} else {
		resp, err = p.client.ChargeBankTransfer(ctx, req.PartnerRef, req.TotalAmount, bank, firstNonEmpty(req.CustomerName, "Customer"), expirySec, cust)
	}
	if err != nil {
		return nil, mapMidtransError(err)
	}

	norm := PaymentDetailNormalized{
		BankCode:    method.Code,
		BankName:    method.Name,
		AccountName: firstNonEmpty(req.CustomerName, "Customer"),
	}
	switch {
	case len(resp.VANumbers) > 0 && resp.VANumbers[0].VANumber != "":
		norm.VANumber = resp.VANumbers[0].VANumber
	case resp.PermataVANumber != "":
		norm.VANumber = resp.PermataVANumber
	case resp.BillKey != "":
		// Mandiri echannel: payment requires both biller_code and bill_key.
		norm.VANumber = resp.BillKey
		norm.BillerCode = resp.BillerCode
	}

	return &PaymentCreateResponse{
		ProviderRef: resp.TransactionID,
		Normalized:  norm,
		RawResponse: resp.RawResponse,
	}, nil
}

// midtransVABank maps the numeric DB bank code to the Midtrans bank_transfer
// bank name (lowercase). Mandiri (008) returns "echannel" to signal the
// Bill Payment pathway. Unknown codes return "" so the selector falls through
// to the next provider.
func midtransVABank(code string) string {
	switch strings.TrimSpace(code) {
	case "002":
		return "bri"
	case "009":
		return "bni"
	case "022":
		return "cimb"
	case "013":
		return "permata"
	case "008":
		return "echannel"
	default:
		return ""
	}
}

// midtransExpirySeconds returns the seconds from now until t. Returns 0 when t
// is zero or already past, so Midtrans applies its default expiry.
func midtransExpirySeconds(t time.Time) int {
	if t.IsZero() {
		return 0
	}
	d := int(time.Until(t).Seconds())
	if d <= 0 {
		return 0
	}
	return d
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
		return models.PaymentStatusSuccess
	case midtrans.StatusDeny:
		return models.PaymentStatusFailed
	case midtrans.StatusExpire:
		return models.PaymentStatusExpired
	case midtrans.StatusCancel:
		return models.PaymentStatusCancelled
	case midtrans.StatusRefund, "partial_refund":
		return models.PaymentStatusFailed
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
