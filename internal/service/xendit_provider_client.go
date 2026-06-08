package service

import (
	"context"
	"strings"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/xendit"
)

// XenditProviderClient implements PaymentProviderClient using Xendit Payment
// Request API for retail (Indomaret/Alfamart) channels.
type XenditProviderClient struct {
	client *xendit.Client
}

func NewXenditProviderClient(client *xendit.Client) *XenditProviderClient {
	return &XenditProviderClient{client: client}
}

func (p *XenditProviderClient) Code() models.PaymentProvider {
	return models.ProviderXendit
}

// Available reports whether the adapter is configured to serve requests.
func (p *XenditProviderClient) Available() bool {
	return true
}

func (p *XenditProviderClient) CreatePayment(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	switch req.Type {
	case models.PaymentTypeRetail:
		return p.createRetail(ctx, method, req)
	case models.PaymentTypeQRIS:
		return p.createQRIS(ctx, method, req)
	case models.PaymentTypeEwallet:
		return p.createEwallet(ctx, method, req)
	default:
		return nil, newPaymentError(400, "UNSUPPORTED_PAYMENT_TYPE", "Xendit adapter supports retail, QRIS, and e-wallet payments", nil)
	}
}

func (p *XenditProviderClient) createRetail(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	channel := strings.ToUpper(strings.TrimSpace(method.Code))
	create := xendit.PaymentRequestCreate{
		ReferenceID:   req.PartnerRef,
		Type:          "PAY",
		Country:       "ID",
		Currency:      "IDR",
		ChannelCode:   channel,
		RequestAmount: req.TotalAmount,
		ChannelProperties: xendit.PaymentRequestChannelProperties{
			PayerName: firstNonEmpty(req.CustomerName, "Customer"),
			ExpiresAt: formatXenditExpiry(req.ExpiredAt),
		},
		Description: req.Description,
	}
	resp, err := p.client.CreatePaymentRequest(ctx, create)
	if err != nil {
		return nil, mapXenditError(err)
	}
	retailName := method.Name
	if retailName == "" {
		retailName = channel
	}
	norm := PaymentDetailNormalized{
		RetailName:  retailName,
		PaymentCode: resp.ChannelProperties.PaymentCode,
	}
	return &PaymentCreateResponse{
		ProviderRef: resp.PaymentRequestID,
		Normalized:  norm,
		RawResponse: resp.RawResponse,
	}, nil
}

func (p *XenditProviderClient) createQRIS(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	create := xendit.PaymentRequestCreate{
		ReferenceID:   req.PartnerRef,
		Type:          "PAY",
		Country:       "ID",
		Currency:      "IDR",
		ChannelCode:   "QRIS",
		RequestAmount: req.TotalAmount,
		ChannelProperties: xendit.PaymentRequestChannelProperties{
			ExpiresAt: formatXenditExpiry(req.ExpiredAt),
		},
		Description: req.Description,
	}
	resp, err := p.client.CreatePaymentRequest(ctx, create)
	if err != nil {
		return nil, mapXenditError(err)
	}

	// Xendit returns QR string in actions[].value where descriptor=QR_STRING
	qrString := resp.ChannelProperties.QRString
	if qrString == "" {
		qrString = extractXenditQRString(resp.Actions)
	}

	norm := PaymentDetailNormalized{
		QRString:   qrString,
		QRImageURL: resp.ChannelProperties.QRImageURL,
	}
	return &PaymentCreateResponse{
		ProviderRef: resp.PaymentRequestID,
		Normalized:  norm,
		RawResponse: resp.RawResponse,
	}, nil
}

func (p *XenditProviderClient) createEwallet(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	channelCode := xenditEwalletChannelCode(method.Code)
	if channelCode == "" {
		return nil, newPaymentError(400, "UNSUPPORTED_PAYMENT_TYPE", "Unsupported ewallet code for Xendit: "+method.Code, nil)
	}
	props := xendit.PaymentRequestChannelProperties{
		ExpiresAt: formatXenditExpiry(req.ExpiredAt),
	}
	if req.CustomerPhone != "" {
		props.MobileNumber = req.CustomerPhone
	}
	if req.ReturnURL != "" {
		props.SuccessReturnURL = req.ReturnURL
		props.FailureReturnURL = req.ReturnURL
		props.CancelReturnURL = req.ReturnURL
	}
	create := xendit.PaymentRequestCreate{
		ReferenceID:       req.PartnerRef,
		Type:              "PAY",
		Country:           "ID",
		Currency:          "IDR",
		ChannelCode:       channelCode,
		RequestAmount:     req.TotalAmount,
		ChannelProperties: props,
		Description:       req.Description,
	}
	resp, err := p.client.CreatePaymentRequest(ctx, create)
	if err != nil {
		return nil, mapXenditError(err)
	}
	norm := PaymentDetailNormalized{}
	// Xendit returns deeplink/checkout URL in actions array
	for _, a := range resp.Actions {
		m, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		url, _ := m["url"].(string)
		if url == "" {
			continue
		}
		switch strings.ToUpper(typ) {
		case "WEB", "DESKTOP_WEB":
			norm.CheckoutURL = url
		case "MOBILE", "MOBILE_DEEPLINK", "APP":
			norm.Deeplink = url
		}
	}
	return &PaymentCreateResponse{
		ProviderRef: resp.PaymentRequestID,
		Normalized:  norm,
		RawResponse: resp.RawResponse,
	}, nil
}

// xenditEwalletChannelCode maps a payment method e-wallet code to its Xendit
// Payment Request API channel code. It accepts the canonical plain codes
// (OVO, SHOPEEPAY, LINKAJA, ASTRAPAY) as well as the Pakailink PAY-prefixed
// forms for robustness, in any case and with surrounding whitespace.
//
// Codes Xendit does not support (e.g. GOPAY) return "" so the caller forces a
// fallback to another provider (Midtrans/DANA) instead of calling Xendit.
func xenditEwalletChannelCode(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "OVO", "PAYOVO":
		return "OVO"
	case "SHOPEEPAY", "PAYSHOPEE", "PAYSHOPEEPAY":
		return "SHOPEEPAY"
	case "LINKAJA", "PAYLINKAJA":
		return "LINKAJA"
	case "ASTRAPAY", "PAYASTRAPAY":
		return "ASTRAPAY"
	default:
		// GOPAY and any unknown code: not supported by Xendit → force fallback.
		return ""
	}
}

// extractXenditQRString pulls the QR string from Xendit's actions array.
// Xendit returns: actions:[{"descriptor":"QR_STRING","type":"PRESENT_TO_CUSTOMER","value":"00020101..."}]
func extractXenditQRString(actions []any) string {
	for _, a := range actions {
		m, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		descriptor, _ := m["descriptor"].(string)
		typ, _ := m["type"].(string)
		val, _ := m["value"].(string)
		if (descriptor == "QR_STRING" || typ == "PRESENT_TO_CUSTOMER") && val != "" {
			return val
		}
	}
	return ""
}

func (p *XenditProviderClient) InquiryPayment(ctx context.Context, payment *models.Payment) (*PaymentInquiryResult, error) {
	if payment.ProviderRef == nil || *payment.ProviderRef == "" {
		return &PaymentInquiryResult{Status: payment.Status}, nil
	}
	resp, err := p.client.GetPaymentRequest(ctx, *payment.ProviderRef)
	if err != nil {
		return nil, mapXenditError(err)
	}
	return &PaymentInquiryResult{
		Status:      mapXenditStatus(resp.Status),
		ProviderRef: resp.PaymentRequestID,
		PaidAmount:  payment.Amount,
		RawResponse: resp.RawResponse,
	}, nil
}

func (p *XenditProviderClient) CancelPayment(ctx context.Context, payment *models.Payment, reason string) (*PaymentCancelResult, error) {
	if payment.ProviderRef == nil || *payment.ProviderRef == "" {
		return &PaymentCancelResult{Cancelled: true}, nil
	}
	resp, err := p.client.CancelPaymentRequest(ctx, *payment.ProviderRef)
	if err != nil {
		return nil, mapXenditError(err)
	}
	return &PaymentCancelResult{Cancelled: true, RawResponse: resp.RawResponse}, nil
}

func formatXenditExpiry(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

func mapXenditStatus(status string) models.PaymentStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case xendit.StatusSucceeded:
		return models.PaymentStatusPaid
	case xendit.StatusExpired:
		return models.PaymentStatusExpired
	case xendit.StatusCanceled:
		return models.PaymentStatusCancelled
	case xendit.StatusFailed:
		return models.PaymentStatusFailed
	default:
		return models.PaymentStatusPending
	}
}

func mapXenditError(err error) error {
	if err == nil {
		return nil
	}
	apiErr, ok := err.(*xendit.APIError)
	if ok {
		if apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			return newPaymentError(400, "PROVIDER_REQUEST_REJECTED", firstNonEmpty(apiErr.Message, "Provider rejected request"), err)
		}
		if apiErr.HTTPStatus >= 500 {
			return newPaymentError(503, "PROVIDER_UNAVAILABLE", "Payment provider temporarily unavailable", err)
		}
	}
	return newPaymentError(502, "PROVIDER_ERROR", "Payment provider error", err)
}
