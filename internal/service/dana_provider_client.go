package service

import (
	"context"
	"strings"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/dana"
)

// DanaProviderClient wraps pkg/dana to implement PaymentProviderClient for
// DANA e-wallet and QRIS MPM via DANA Direct.
type DanaProviderClient struct {
	client          *dana.Client
	notificationURL string
	returnURL       string
	storeID         string // QRIS Acquirer: store ID (required)
	terminalID      string // QRIS Acquirer: terminal ID (optional)
}

func NewDanaProviderClient(client *dana.Client, notificationURL, returnURL string) *DanaProviderClient {
	return &DanaProviderClient{client: client, notificationURL: notificationURL, returnURL: returnURL}
}

// SetExternalStoreID sets the DANA store ID required for QRIS Custom Checkout (externalStoreId).
func (p *DanaProviderClient) SetExternalStoreID(id string) {
	p.storeID = id
}

// SetTerminalID sets the DANA terminal ID (reserved for future use).
func (p *DanaProviderClient) SetTerminalID(id string) {
	p.terminalID = id
}

func (p *DanaProviderClient) Code() models.PaymentProvider {
	return models.ProviderDanaDirect
}

// Available reports whether the adapter is configured to serve requests.
func (p *DanaProviderClient) Available() bool {
	return true
}

func (p *DanaProviderClient) CreatePayment(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	switch req.Type {
	case models.PaymentTypeEwallet:
		payMethod, payOption := danaEwalletMethodOption(method.Code)
		return p.createOrder(ctx, method, req, payMethod, payOption)
	case models.PaymentTypeVA:
		payOption := danaVAPayOption(method.Code)
		if payOption == "" {
			return nil, newPaymentError(400, "UNSUPPORTED_PAYMENT_TYPE", "Unsupported VA bank code for DANA: "+method.Code, nil)
		}
		return p.createOrder(ctx, method, req, dana.PayMethodVirtualAccount, payOption)
	case models.PaymentTypeQRIS:
		if strings.ToUpper(strings.TrimSpace(method.Code)) == "CPM" {
			return p.createCPMQRIS(ctx, method, req)
		}
		// MPM: Use DANA Gapura Custom Checkout with NETWORK_PAY + NETWORK_PAY_PG_QRIS.
		// Returns qrContent in additionalInfo.paymentCode.
		return p.createOrder(ctx, method, req, dana.PayMethodNetworkPay, dana.PayOptionQRIS)
	default:
		return nil, newPaymentError(400, "UNSUPPORTED_PAYMENT_TYPE", "DANA does not support this payment type", nil)
	}
}

// danaVAPayOption maps the numeric DB bank code to the DANA VA payOption
// (VIRTUAL_ACCOUNT_<bank>). Unknown codes return "" so the selector falls
// through to the next provider.
func danaVAPayOption(code string) string {
	switch strings.TrimSpace(code) {
	case "002":
		return dana.PayOptionVABRI
	case "009":
		return dana.PayOptionVABNI
	case "008":
		return dana.PayOptionVAMandiri
	case "022":
		return dana.PayOptionVACIMB
	case "019":
		return dana.PayOptionVAPanin
	case "213":
		return dana.PayOptionVABTPN
	// 451 BSI and 013 Permata are intentionally omitted: DANA rejects both
	// VIRTUAL_ACCOUNT_BSI_PAYMENT/VIRTUAL_ACCOUNT_BSI and VIRTUAL_ACCOUNT_PERMATA
	// with 4005402 Invalid Field Format on this account. Returning "" makes the
	// selector fall through to Pakailink/Xendit instead of hard-failing.
	default:
		return ""
	}
}

// danaEwalletMethodOption maps a payment method code to the DANA payMethod/payOption pair.
// PAYDANA uses DANA's own balance (BALANCE). Other wallets go through NETWORK_PAY.
func danaEwalletMethodOption(code string) (payMethod, payOption string) {
	switch strings.ToUpper(code) {
	case "PAYDANA":
		return dana.PayMethodBalance, ""
	case "PAYGOPAY":
		return dana.PayMethodNetworkPay, dana.PayOptionGoPay
	case "PAYOVO":
		return dana.PayMethodNetworkPay, dana.PayOptionOVO
	case "PAYSHOPEE":
		return dana.PayMethodNetworkPay, dana.PayOptionShopeePay
	case "PAYLINKAJA":
		return dana.PayMethodNetworkPay, dana.PayOptionLinkAja
	default:
		return dana.PayMethodBalance, ""
	}
}

func (p *DanaProviderClient) createOrder(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest, payMethod, payOption string) (*PaymentCreateResponse, error) {
	partnerRef := req.PartnerRef
	// QRIS via Custom Checkout has max 25 chars for partnerReferenceNo per DANA docs.
	if payOption == dana.PayOptionQRIS && len(partnerRef) > 25 {
		partnerRef = partnerRef[:25]
	}

	order := dana.CreateOrderRequest{
		PartnerReferenceNo: partnerRef,
		Amount:             req.TotalAmount,
		ValidUpTo:          formatDanaExpiry(req.ExpiredAt),
		// Provider notification always goes to OUR webhook endpoint, never the
		// client's callback URL (the client webhook is delivered separately).
		NotificationURL: p.notificationURL,
		// For QRIS (server-to-server) there is no user redirect; pass empty so
		// pkg/dana will substitute a safe placeholder URL for the mandatory PAY_RETURN field.
		ReturnURL: func() string {
			if payOption == dana.PayOptionQRIS {
				return ""
			}
			return firstNonEmpty(req.ReturnURL, p.returnURL)
		}(),
		PayMethod:   payMethod,
		PayOption:   payOption,
		OrderTitle:  firstNonEmpty(req.Description, method.Name),
	}
	if p.storeID != "" {
		order.ExternalStoreID = p.storeID
	}
	resp, err := p.client.CreateOrder(ctx, order)
	if err != nil {
		return nil, mapDanaError(err)
	}
	norm := PaymentDetailNormalized{}
	switch req.Type {
	case models.PaymentTypeQRIS:
		// Gapura Custom Checkout QRIS: QR string in additionalInfo.paymentCode
		norm.QRString = resp.PaymentCode()
	case models.PaymentTypeVA:
		norm.BankCode = method.Code
		norm.BankName = firstNonEmpty(method.Name, payOption)
		// DANA returns the VA number in additionalInfo.paymentCode (same field
		// as QRIS); fall back to the documented VA keys just in case.
		norm.VANumber = firstNonEmpty(resp.VirtualAccountNumber(), resp.PaymentCode())
		norm.AccountName = firstNonEmpty(req.CustomerName, req.ClientName, "Customer")
	default:
		norm.CheckoutURL = resp.CheckoutURL
		norm.MobileWebURL = resp.WebRedirectURL
		norm.Deeplink = resp.DeeplinkURL
	}
	return &PaymentCreateResponse{
		ProviderRef: resp.ReferenceNo,
		Normalized:  norm,
		RawResponse: resp.RawResponse,
	}, nil
}

func (p *DanaProviderClient) InquiryPayment(ctx context.Context, payment *models.Payment) (*PaymentInquiryResult, error) {
	// CPM uses a different inquiry endpoint (serviceCode=60, /rest/v1.1/debit/status)
	if payment.PaymentCode == "CPM" {
		resp, err := p.client.InquiryCPMOrder(ctx, payment.PaymentID)
		if err != nil {
			return nil, mapDanaError(err)
		}
		status := mapDanaTransactionStatus(resp.LatestTransactionStatus)
		amount, _ := dana.ParseWebhookAmount(resp.Amount)
		return &PaymentInquiryResult{
			Status:      status,
			ProviderRef: resp.OriginalReferenceNo,
			PaidAmount:  amount,
			RawResponse: resp.RawResponse,
		}, nil
	}
	// MPM / e-wallet uses Gapura PG status endpoint (serviceCode=55)
	resp, err := p.client.InquiryOrder(ctx, payment.PaymentID)
	if err != nil {
		return nil, mapDanaError(err)
	}
	status := mapDanaTransactionStatus(resp.LatestTransactionStatus)
	amount, _ := dana.ParseWebhookAmount(resp.Amount)
	return &PaymentInquiryResult{
		Status:      status,
		ProviderRef: resp.OriginalReferenceNo,
		PaidAmount:  amount,
		RawResponse: resp.RawResponse,
	}, nil
}

func (p *DanaProviderClient) CancelPayment(ctx context.Context, payment *models.Payment, reason string) (*PaymentCancelResult, error) {
	req := dana.CancelOrderRequest{
		PartnerReferenceNo: payment.PaymentID,
		Reason:             firstNonEmpty(reason, "Customer cancellation"),
	}
	resp, err := p.client.CancelOrder(ctx, req)
	if err != nil {
		return nil, mapDanaError(err)
	}
	return &PaymentCancelResult{Cancelled: true, RawResponse: resp.RawResponse}, nil
}

func (p *DanaProviderClient) createCPMQRIS(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	refNo := req.PartnerRef
	if len(refNo) > 25 {
		refNo = refNo[:25]
	}
	storeID := p.storeID
	if storeID == "" {
		return nil, newPaymentError(400, "MISSING_CONFIG", "DANA CPM QRIS requires DANA_EXTERNAL_STORE_ID to be configured", nil)
	}
	// QRContent is the QR string scanned from the customer's DANA app.
	qrContent := req.ScanData
	if qrContent == "" {
		return nil, newPaymentError(400, "MISSING_FIELD", "scanData (QR content from customer's DANA app) is required for CPM QRIS", nil)
	}
	cpmReq := dana.CPMPaymentRequest{
		PartnerReferenceNo: refNo,
		QRContent:          qrContent,
		Amount:             req.TotalAmount,
		StoreID:            storeID,
		TerminalID:         p.terminalID,
		Title:              firstNonEmpty(req.Description, method.Name),
		ValidityPeriod:     formatDanaExpiry(req.ExpiredAt),
		NotificationURL:    p.notificationURL,
	}
	resp, err := p.client.CPMPayment(ctx, cpmReq)
	if err != nil {
		return nil, mapDanaError(err)
	}
	norm := PaymentDetailNormalized{
		QRString: qrContent, // Echo back — CPM doesn't generate a new QR
	}
	return &PaymentCreateResponse{
		ProviderRef: resp.ReferenceNo,
		Normalized:  norm,
		RawResponse: resp.RawResponse,
	}, nil
}

func formatDanaExpiry(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02T15:04:05+07:00")
}

func mapDanaTransactionStatus(code string) models.PaymentStatus {
	switch strings.TrimSpace(code) {
	case "00":
		return models.PaymentStatusSuccess
	case "05":
		return models.PaymentStatusCancelled
	case "06":
		return models.PaymentStatusFailed
	case "07":
		return models.PaymentStatusExpired
	default:
		return models.PaymentStatusPending
	}
}

func mapDanaError(err error) error {
	if err == nil {
		return nil
	}
	apiErr, ok := err.(*dana.APIError)
	if ok {
		if strings.HasPrefix(apiErr.ResponseCode, "4") {
			return newPaymentError(400, "PROVIDER_REQUEST_REJECTED", firstNonEmpty(apiErr.ResponseMessage, "Provider rejected request"), err)
		}
		if strings.HasPrefix(apiErr.ResponseCode, "5") {
			return newPaymentError(503, "PROVIDER_UNAVAILABLE", "Payment provider temporarily unavailable", err)
		}
	}
	return newPaymentError(502, "PROVIDER_ERROR", "Payment provider error", err)
}
