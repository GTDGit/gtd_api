package service

import (
	"context"
	"fmt"
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
	case models.PaymentTypeVA:
		return p.createVA(ctx, method, req)
	default:
		return nil, newPaymentError(400, "VALIDATION_ERROR", "Xendit adapter supports retail, QRIS, e-wallet, and VA payments", nil)
	}
}

// createVA creates a closed single-use Fixed Virtual Account via the legacy
// /callback_virtual_accounts API. This account does not serve VA through the
// v3 payment_requests flow (that endpoint rejects VA channel codes), so VA uses
// the dedicated FVA endpoint. Payment confirmation arrives via the FVA-paid
// webhook (handled in HandleXendit), keyed by external_id == our PaymentID.
func (p *XenditProviderClient) createVA(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	bankCode := xenditVAChannelCode(method.Code)
	if bankCode == "" {
		return nil, newPaymentError(400, "VALIDATION_ERROR", "Unsupported VA bank code for Xendit: "+method.Code, nil)
	}
	customerName := firstNonEmpty(req.CustomerName, req.ClientName, "Customer")
	create := xendit.VirtualAccountCreate{
		ExternalID:     req.PartnerRef,
		BankCode:       bankCode,
		Name:           customerName,
		IsClosed:       true,
		IsSingleUse:    true,
		ExpectedAmount: req.TotalAmount,
		ExpirationDate: formatXenditExpiry(req.ExpiredAt),
	}
	resp, err := p.client.CreateVirtualAccount(ctx, create)
	if err != nil {
		return nil, mapXenditError(err)
	}
	norm := PaymentDetailNormalized{
		BankCode:    method.Code,
		BankName:    firstNonEmpty(method.Name, bankCode),
		VANumber:    resp.AccountNumber,
		AccountName: customerName,
	}
	return &PaymentCreateResponse{
		ProviderRef: resp.ID,
		Normalized:  norm,
		RawResponse: resp.RawResponse,
	}, nil
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
	// Xendit returns the OTC payment code in actions[] with
	// descriptor=PAYMENT_CODE, not in channel_properties.payment_code.
	paymentCode := resp.ChannelProperties.PaymentCode
	if paymentCode == "" {
		paymentCode = extractXenditPaymentCode(resp.Actions)
	}
	norm := PaymentDetailNormalized{
		RetailName:  retailName,
		PaymentCode: paymentCode,
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
		return nil, newPaymentError(400, "VALIDATION_ERROR", "Unsupported ewallet code for Xendit: "+method.Code, nil)
	}
	props := xendit.PaymentRequestChannelProperties{
		ExpiresAt: formatXenditExpiry(req.ExpiredAt),
	}
	phone := normalizePhone62(req.CustomerPhone)
	// OVO requires the account mobile number; reject early if missing.
	if channelCode == "OVO" && phone == "" {
		return nil, newPaymentError(400, "VALIDATION_ERROR", "customer.phone is required for OVO payments", nil)
	}
	if phone != "" {
		// Xendit account_mobile_number requires E.164 with leading "+": ^\+[0-9]\d{1,14}$
		props.AccountMobileNumber = "+" + phone
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
	// Xendit returns the redirect/deeplink URL in the actions array. The URL is
	// in "value" (sometimes "url"); the kind is in "descriptor" (WEB_URL,
	// MOBILE_URL, DEEPLINK_URL) or "type" (REDIRECT_CUSTOMER/PRESENT_TO_CUSTOMER).
	for _, a := range resp.Actions {
		m, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		url, _ := m["value"].(string)
		if url == "" {
			url, _ = m["url"].(string)
		}
		if url == "" {
			continue
		}
		descriptor := strings.ToUpper(fmt.Sprint(m["descriptor"]))
		typ := strings.ToUpper(fmt.Sprint(m["type"]))
		switch {
		case strings.Contains(descriptor, "DEEPLINK"), strings.Contains(descriptor, "MOBILE"),
			strings.Contains(typ, "MOBILE"), strings.Contains(typ, "APP"):
			norm.Deeplink = url
		default:
			// WEB_URL, REDIRECT_CUSTOMER, or anything else with a URL → web checkout.
			if norm.CheckoutURL == "" {
				norm.CheckoutURL = url
			}
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

// extractXenditPaymentCode pulls the OTC payment code from Xendit's actions
// array. Xendit returns: actions:[{"descriptor":"PAYMENT_CODE","type":"PRESENT_TO_CUSTOMER","value":"PTGSX2DABN4379"}]
func extractXenditPaymentCode(actions []any) string {
	for _, a := range actions {
		m, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		descriptor, _ := m["descriptor"].(string)
		val, _ := m["value"].(string)
		if strings.EqualFold(descriptor, "PAYMENT_CODE") && val != "" {
			return val
		}
	}
	return ""
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

// xenditVAChannelCode maps the numeric DB bank code to the Xendit legacy
// Fixed-VA bank code (uppercase). Unknown codes return "" so the selector
// falls through to the next provider. Only banks activated on the account
// (per GET /available_virtual_account_banks) will actually succeed; others
// surface a provider error and trigger fallback.
func xenditVAChannelCode(code string) string {
	switch strings.TrimSpace(code) {
	case "002":
		return "BRI"
	case "009":
		return "BNI"
	case "008":
		return "MANDIRI"
	case "451":
		return "BSI"
	case "022":
		return "CIMB"
	case "013":
		return "PERMATA"
	case "110":
		return "BJB"
	case "120":
		return "SAHABAT_SAMPOERNA"
	default:
		return ""
	}
}

func (p *XenditProviderClient) InquiryPayment(ctx context.Context, payment *models.Payment) (*PaymentInquiryResult, error) {
	if payment.ProviderRef == nil || *payment.ProviderRef == "" {
		return &PaymentInquiryResult{Status: payment.Status}, nil
	}
	// VA uses the legacy Fixed-VA API. Its object status is a lifecycle value
	// (PENDING/ACTIVE/INACTIVE), not a payment status — a paid single-use VA
	// reads back INACTIVE, which would wrongly clobber a paid payment. So we
	// fetch it only to record the raw object and return an empty status,
	// deferring authoritative payment confirmation to the FVA-paid webhook.
	if payment.PaymentType == models.PaymentTypeVA {
		resp, err := p.client.GetVirtualAccount(ctx, *payment.ProviderRef)
		if err != nil {
			return nil, mapXenditError(err)
		}
		return &PaymentInquiryResult{
			Status:      "",
			ProviderRef: resp.ID,
			PaidAmount:  payment.Amount,
			RawResponse: resp.RawResponse,
		}, nil
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
	// VA uses the legacy Fixed-VA API; the v3 cancel endpoint would 404 on a
	// legacy VA id. A closed single-use VA simply expires, so cancel is a
	// local no-op (the payment is marked cancelled by the caller).
	if payment.PaymentType == models.PaymentTypeVA {
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

// normalizePhone62 normalizes an Indonesian mobile number to the 62 format
// (no leading +). "08123" → "628123", "+628123" → "628123", "628123" → "628123".
// Returns "" for empty/garbage input.
func normalizePhone62(phone string) string {
	p := strings.TrimSpace(phone)
	if p == "" {
		return ""
	}
	p = strings.TrimPrefix(p, "+")
	var b strings.Builder
	for _, r := range p {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	if digits == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(digits, "62"):
		return digits
	case strings.HasPrefix(digits, "0"):
		return "62" + strings.TrimPrefix(digits, "0")
	default:
		return "62" + digits
	}
}

func mapXenditStatus(status string) models.PaymentStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case xendit.StatusSucceeded:
		return models.PaymentStatusSuccess
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

// mapXenditError maps a Xendit failure to the unified payment error taxonomy.
// It inspects the specific error_code first, falling back to HTTP-status range.
// Duplicate signals are surfaced as PROVIDER_DUPLICATE (409) so CreatePayment
// can answer idempotently with the existing payment.
func mapXenditError(err error) error {
	if err == nil {
		return nil
	}
	apiErr, ok := err.(*xendit.APIError)
	if !ok {
		// Non-API error (network/timeout/empty body): status unknown.
		return newPaymentError(504, "PROVIDER_TIMEOUT", "No response from provider, payment status unknown", err)
	}

	switch strings.ToUpper(strings.TrimSpace(apiErr.ErrorCode)) {
	case "DUPLICATE_ERROR", "DATA_NOT_FOUND", "ACCOUNT_ALREADY_LINKED":
		return newPaymentError(409, "PROVIDER_DUPLICATE", "Duplicate reference at provider", err)
	case "INVALID_VALUE_ERROR", "API_VALIDATION_ERROR", "INVALID_PAYMENT_DETAILS", "CARD_EXPIRED":
		return newPaymentError(400, "PROVIDER_REJECTED", "Provider rejected the request", err)
	case "ACCOUNT_ACCESS_BLOCKED", "INVALID_MERCHANT_SETTINGS", "SKIP_3DS_FORBIDDEN":
		return newPaymentError(402, "PAYMENT_DENIED", "Payment was denied", err)
	case "PAYMENT_REQUEST_RATE_LIMITED", "CHANNEL_UNAVAILABLE", "ISSUER_UNAVAILABLE", "SERVER_ERROR":
		return newPaymentError(503, "PROVIDER_UNAVAILABLE", "Payment provider is temporarily unavailable", err)
	}

	// Fallback by HTTP status range.
	switch {
	case apiErr.HTTPStatus == 409:
		return newPaymentError(409, "PROVIDER_DUPLICATE", "Duplicate reference at provider", err)
	case apiErr.HTTPStatus == 429, apiErr.HTTPStatus >= 500:
		return newPaymentError(503, "PROVIDER_UNAVAILABLE", "Payment provider is temporarily unavailable", err)
	case apiErr.HTTPStatus >= 400:
		return newPaymentError(400, "PROVIDER_REJECTED", "Provider rejected the request", err)
	}
	return newPaymentError(504, "PROVIDER_TIMEOUT", "No response from provider, payment status unknown", err)
}
