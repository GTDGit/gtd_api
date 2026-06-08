package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/xendit"
)

// Integration tests for XenditProviderClient (task 11.3, Req 11.1-11.5).
//
// These tests use net/http/httptest to serve canned Xendit Payment Request API
// responses and point a real pkg/xendit.Client at the test server. The httptest
// handler captures the request method, path, JSON body, and Authorization
// header so we can assert the adapter hits the correct endpoint
// (create-payment-request / get-payment-request / cancel-payment-request),
// sends the expected channel code, and normalizes paymentDetail per type:
//
//	RETAIL  (ALFAMART/INDOMARET) -> paymentCode + retailName
//	QRIS                          -> qrString
//	EWALLET (OVO/SHOPEEPAY/...)   -> checkoutUrl / deeplink (via xenditEwalletChannelCode)
//	VA                            -> not implemented by the adapter (documented below)
//
// Webhook callback-token verification (Req 11.2) is covered against
// pkg/xendit.VerifyWebhookToken, which is the surface the adapter/HTTP layer
// relies on to authenticate Xendit's payment-webhook-notification.
//
// Helpers are prefixed with xnTest to avoid collisions with the existing
// Xendit property tests (TestProperty12_XenditEwalletChannelMapping etc.).

const (
	xnTestAPIKey       = "xnd_test_api_key"
	xnTestWebhookToken = "xn-test-callback-token"
)

// xnTestCapture records what the most recent Xendit request carried.
type xnTestCapture struct {
	Method string
	Path   string
	Auth   string
	Body   map[string]any
}

// xnTestServer spins up an httptest server that captures every request and, for
// the path matching wantPath, encodes the canned response. Any other path
// returns 404 so an unexpected endpoint is surfaced as a test failure.
func xnTestServer(t *testing.T, cap *xnTestCapture, wantPath string, response map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.Method = r.Method
		cap.Path = r.URL.Path
		cap.Auth = r.Header.Get("Authorization")
		body := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		cap.Body = body

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == wantPath {
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error_code": "NOT_FOUND",
			"message":    "Unexpected path: " + r.URL.Path,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// xnTestClient builds an XenditProviderClient backed by a real pkg/xendit.Client
// pointed at baseURL with a test API key and callback token.
func xnTestClient(t *testing.T, baseURL string) *XenditProviderClient {
	t.Helper()
	c, err := xendit.NewClient(xendit.Config{
		BaseURL:      baseURL,
		APIKey:       xnTestAPIKey,
		WebhookToken: xnTestWebhookToken,
	})
	if err != nil {
		t.Fatalf("new xendit client: %v", err)
	}
	return NewXenditProviderClient(c)
}

// xnTestWantAuth is the expected Basic auth header for the test API key
// (Xendit uses Basic base64(apiKey + ":")).
func xnTestWantAuth() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(xnTestAPIKey+":"))
}

func xnTestStrPtr(s string) *string { return &s }

// Req 11.1 / 11.5: Create RETAIL (ALFAMART, INDOMARET) hits create-payment-request,
// sends the channel code as the method code, and normalizes paymentCode +
// retailName.
func TestXenditIntegration_CreateRetail_PaymentCodeAndRetailName(t *testing.T) {
	cases := []struct {
		code string
		name string
	}{
		{"ALFAMART", "Alfamart"},
		{"INDOMARET", "Indomaret"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			cap := &xnTestCapture{}
			resp := map[string]any{
				"payment_request_id": "pr-retail-001",
				"reference_id":       "xn-ref-retail-001",
				"channel_code":       tc.code,
				"status":             xendit.StatusAccepting,
				"channel_properties": map[string]any{
					"payment_code": "RETAIL12345",
				},
			}
			srv := xnTestServer(t, cap, xendit.CreatePaymentRequestPath, resp)
			client := xnTestClient(t, srv.URL)

			method := &models.PaymentMethod{Type: models.PaymentTypeRetail, Code: tc.code, Name: tc.name}
			req := &PaymentCreateRequest{
				Type:         models.PaymentTypeRetail,
				Code:         tc.code,
				PartnerRef:   "xn-ref-retail-001",
				TotalAmount:  30000,
				CustomerName: "John Doe",
			}

			out, err := client.CreatePayment(context.Background(), method, req)
			if err != nil {
				t.Fatalf("CreatePayment RETAIL %s: %v", tc.code, err)
			}
			if cap.Method != http.MethodPost || cap.Path != xendit.CreatePaymentRequestPath {
				t.Errorf("create endpoint = %s %q, want POST %q", cap.Method, cap.Path, xendit.CreatePaymentRequestPath)
			}
			if cap.Auth != xnTestWantAuth() {
				t.Errorf("Authorization = %q, want %q", cap.Auth, xnTestWantAuth())
			}
			if got, _ := cap.Body["channel_code"].(string); got != tc.code {
				t.Errorf("wire channel_code = %q, want %q", got, tc.code)
			}
			if out.Normalized.PaymentCode != "RETAIL12345" {
				t.Errorf("paymentCode = %q, want RETAIL12345", out.Normalized.PaymentCode)
			}
			if out.Normalized.RetailName != tc.name {
				t.Errorf("retailName = %q, want %q", out.Normalized.RetailName, tc.name)
			}
			if out.ProviderRef != "pr-retail-001" {
				t.Errorf("providerRef = %q, want pr-retail-001", out.ProviderRef)
			}
		})
	}
}

// Req 11.1: Create QRIS hits create-payment-request with channel QRIS and
// normalizes the QR string (here returned via channel_properties.qr_string).
func TestXenditIntegration_CreateQRIS_ReturnsQRString(t *testing.T) {
	cap := &xnTestCapture{}
	const qr = "00020101021126670016COM.XENDIT.WWW...6304ABCD"
	resp := map[string]any{
		"payment_request_id": "pr-qris-001",
		"reference_id":       "xn-ref-qris-001",
		"channel_code":       "QRIS",
		"status":             xendit.StatusAccepting,
		"channel_properties": map[string]any{
			"qr_string":    qr,
			"qr_image_url": "https://qr.xendit.test/img/001.png",
		},
	}
	srv := xnTestServer(t, cap, xendit.CreatePaymentRequestPath, resp)
	client := xnTestClient(t, srv.URL)

	method := &models.PaymentMethod{Type: models.PaymentTypeQRIS, Code: "MPM", Name: "QRIS"}
	req := &PaymentCreateRequest{
		Type:        models.PaymentTypeQRIS,
		Code:        "MPM",
		PartnerRef:  "xn-ref-qris-001",
		TotalAmount: 15000,
	}

	out, err := client.CreatePayment(context.Background(), method, req)
	if err != nil {
		t.Fatalf("CreatePayment QRIS: %v", err)
	}
	if cap.Path != xendit.CreatePaymentRequestPath {
		t.Errorf("create endpoint = %q, want %q", cap.Path, xendit.CreatePaymentRequestPath)
	}
	if got, _ := cap.Body["channel_code"].(string); got != "QRIS" {
		t.Errorf("wire channel_code = %q, want QRIS", got)
	}
	if out.Normalized.QRString != qr {
		t.Errorf("qrString = %q, want canned qr_string", out.Normalized.QRString)
	}
	if out.Normalized.QRImageURL != "https://qr.xendit.test/img/001.png" {
		t.Errorf("qrImageUrl = %q, want canned qr_image_url", out.Normalized.QRImageURL)
	}
}

// Req 11.1 / 11.5: Create EWALLET maps the canonical e-wallet code to the Xendit
// channel via xenditEwalletChannelCode (OVO -> OVO, SHOPEEPAY -> SHOPEEPAY) and
// normalizes the checkout URL (WEB action) and deeplink (MOBILE action) from the
// actions array.
func TestXenditIntegration_CreateEwallet_ChannelMappingAndActions(t *testing.T) {
	cases := []struct {
		code        string
		wantChannel string
	}{
		{"OVO", "OVO"},
		{"SHOPEEPAY", "SHOPEEPAY"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			cap := &xnTestCapture{}
			resp := map[string]any{
				"payment_request_id": "pr-ewallet-001",
				"reference_id":       "xn-ref-ewallet-001",
				"channel_code":       tc.wantChannel,
				"status":             xendit.StatusAccepting,
				"actions": []any{
					map[string]any{"type": "WEB", "url": "https://checkout.xendit.test/web/abc"},
					map[string]any{"type": "MOBILE", "url": "https://checkout.xendit.test/app/abc"},
				},
			}
			srv := xnTestServer(t, cap, xendit.CreatePaymentRequestPath, resp)
			client := xnTestClient(t, srv.URL)

			method := &models.PaymentMethod{Type: models.PaymentTypeEwallet, Code: tc.code, Name: tc.code + " Wallet"}
			req := &PaymentCreateRequest{
				Type:          models.PaymentTypeEwallet,
				Code:          tc.code,
				PartnerRef:    "xn-ref-ewallet-001",
				TotalAmount:   50000,
				CustomerPhone: "08123456789",
				ReturnURL:     "https://merchant.example/return",
			}

			out, err := client.CreatePayment(context.Background(), method, req)
			if err != nil {
				t.Fatalf("CreatePayment EWALLET %s: %v", tc.code, err)
			}
			if cap.Path != xendit.CreatePaymentRequestPath {
				t.Errorf("create endpoint = %q, want %q", cap.Path, xendit.CreatePaymentRequestPath)
			}
			if got, _ := cap.Body["channel_code"].(string); got != tc.wantChannel {
				t.Errorf("wire channel_code = %q, want %q (via xenditEwalletChannelCode)", got, tc.wantChannel)
			}
			if out.Normalized.CheckoutURL != "https://checkout.xendit.test/web/abc" {
				t.Errorf("checkoutUrl = %q, want WEB action url", out.Normalized.CheckoutURL)
			}
			if out.Normalized.Deeplink != "https://checkout.xendit.test/app/abc" {
				t.Errorf("deeplink = %q, want MOBILE action url", out.Normalized.Deeplink)
			}
		})
	}
}

// Req 11.5 (negative): GOPAY is not supported by Xendit, so the adapter rejects
// the request with UNSUPPORTED_PAYMENT_TYPE before any HTTP call is made
// (xenditEwalletChannelCode returns "" to force provider fallback).
func TestXenditIntegration_CreateEwallet_UnsupportedChannelRejected(t *testing.T) {
	cap := &xnTestCapture{}
	srv := xnTestServer(t, cap, xendit.CreatePaymentRequestPath, map[string]any{})
	client := xnTestClient(t, srv.URL)

	method := &models.PaymentMethod{Type: models.PaymentTypeEwallet, Code: "GOPAY", Name: "GoPay"}
	req := &PaymentCreateRequest{
		Type:        models.PaymentTypeEwallet,
		Code:        "GOPAY",
		PartnerRef:  "xn-ref-gopay-001",
		TotalAmount: 50000,
	}

	_, err := client.CreatePayment(context.Background(), method, req)
	if err == nil {
		t.Fatal("expected error for unsupported GOPAY channel, got nil")
	}
	var svcErr *PaymentServiceError
	if !errors.As(err, &svcErr) {
		t.Fatalf("error is not *PaymentServiceError: %T", err)
	}
	if svcErr.Code != "UNSUPPORTED_PAYMENT_TYPE" {
		t.Errorf("error code = %q, want UNSUPPORTED_PAYMENT_TYPE", svcErr.Code)
	}
	if cap.Path != "" {
		t.Errorf("unsupported channel should not hit any endpoint, hit %q", cap.Path)
	}
}

// Documented gap: the Xendit adapter does not implement a VA flow. CreatePayment
// only switches on RETAIL, QRIS, and EWALLET; any other type (including VA)
// falls through to UNSUPPORTED_PAYMENT_TYPE. We assert that real behavior so the
// gap is recorded and a future VA implementation will surface here. (Req 11.5
// lists VA channels in the mapping matrix, but the current adapter delegates VA
// to other providers.)
func TestXenditIntegration_CreateVA_NotImplemented(t *testing.T) {
	cap := &xnTestCapture{}
	srv := xnTestServer(t, cap, xendit.CreatePaymentRequestPath, map[string]any{})
	client := xnTestClient(t, srv.URL)

	method := &models.PaymentMethod{Type: models.PaymentTypeVA, Code: "002", Name: "BRI Virtual Account"}
	req := &PaymentCreateRequest{
		Type:        models.PaymentTypeVA,
		Code:        "002",
		BankCode:    "002",
		PartnerRef:  "xn-ref-va-001",
		TotalAmount: 25000,
	}

	_, err := client.CreatePayment(context.Background(), method, req)
	if err == nil {
		t.Fatal("expected error for unimplemented VA flow, got nil")
	}
	var svcErr *PaymentServiceError
	if !errors.As(err, &svcErr) {
		t.Fatalf("error is not *PaymentServiceError: %T", err)
	}
	if svcErr.Code != "UNSUPPORTED_PAYMENT_TYPE" {
		t.Errorf("error code = %q, want UNSUPPORTED_PAYMENT_TYPE", svcErr.Code)
	}
	if cap.Path != "" {
		t.Errorf("unimplemented VA should not hit any endpoint, hit %q", cap.Path)
	}
}

// Req 11.3: InquiryPayment hits get-payment-request and maps the Xendit status
// to the internal PaymentStatus.
func TestXenditIntegration_Inquiry_MapsStatus(t *testing.T) {
	cases := []struct {
		name       string
		providerSt string
		want       models.PaymentStatus
	}{
		{"paid", xendit.StatusSucceeded, models.PaymentStatusPaid},
		{"expired", xendit.StatusExpired, models.PaymentStatusExpired},
		{"cancelled", xendit.StatusCanceled, models.PaymentStatusCancelled},
		{"failed", xendit.StatusFailed, models.PaymentStatusFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const providerRef = "pr-inq-001"
			getPath := xendit.CreatePaymentRequestPath + "/" + providerRef
			cap := &xnTestCapture{}
			resp := map[string]any{
				"payment_request_id": providerRef,
				"reference_id":       "xn-ref-inq-001",
				"status":             tc.providerSt,
			}
			srv := xnTestServer(t, cap, getPath, resp)
			client := xnTestClient(t, srv.URL)

			payment := &models.Payment{
				PaymentID:   "xn-ref-inq-001",
				PaymentType: models.PaymentTypeRetail,
				Amount:      25000,
				Status:      models.PaymentStatusPending,
				ProviderRef: xnTestStrPtr(providerRef),
			}

			res, err := client.InquiryPayment(context.Background(), payment)
			if err != nil {
				t.Fatalf("InquiryPayment: %v", err)
			}
			if cap.Method != http.MethodGet || cap.Path != getPath {
				t.Errorf("inquiry endpoint = %s %q, want GET %q", cap.Method, cap.Path, getPath)
			}
			if res.Status != tc.want {
				t.Errorf("status %q mapped to %q, want %q", tc.providerSt, res.Status, tc.want)
			}
		})
	}
}

// Req 11.4: CancelPayment hits cancel-payment-request.
func TestXenditIntegration_Cancel_HitsCancelEndpoint(t *testing.T) {
	const providerRef = "pr-cancel-001"
	cancelPath := xendit.CreatePaymentRequestPath + "/" + providerRef + "/cancel"
	cap := &xnTestCapture{}
	resp := map[string]any{
		"payment_request_id": providerRef,
		"reference_id":       "xn-ref-cancel-001",
		"status":             xendit.StatusCanceled,
	}
	srv := xnTestServer(t, cap, cancelPath, resp)
	client := xnTestClient(t, srv.URL)

	payment := &models.Payment{
		PaymentID:   "xn-ref-cancel-001",
		PaymentType: models.PaymentTypeRetail,
		Amount:      25000,
		Status:      models.PaymentStatusPending,
		ProviderRef: xnTestStrPtr(providerRef),
	}

	res, err := client.CancelPayment(context.Background(), payment, "customer requested")
	if err != nil {
		t.Fatalf("CancelPayment: %v", err)
	}
	if cap.Method != http.MethodPost || cap.Path != cancelPath {
		t.Errorf("cancel endpoint = %s %q, want POST %q", cap.Method, cap.Path, cancelPath)
	}
	if !res.Cancelled {
		t.Error("expected Cancelled = true")
	}
}

// Req 11.2: Xendit's payment-webhook-notification is authenticated by the
// x-callback-token header. The adapter/HTTP layer relies on
// pkg/xendit.VerifyWebhookToken (constant-time compare against the configured
// token). A request carrying the configured token verifies; an incorrect or
// missing token is rejected.
func TestXenditIntegration_WebhookTokenVerification(t *testing.T) {
	client := xnTestClient(t, "https://api.xendit.test")
	configured := client.client.WebhookToken()
	if configured != xnTestWebhookToken {
		t.Fatalf("configured webhook token = %q, want %q", configured, xnTestWebhookToken)
	}

	if !xendit.VerifyWebhookToken(xnTestWebhookToken, configured) {
		t.Error("correct callback token must verify")
	}
	if xendit.VerifyWebhookToken("wrong-token", configured) {
		t.Error("incorrect callback token must be rejected")
	}
	if xendit.VerifyWebhookToken("", configured) {
		t.Error("empty callback token must be rejected")
	}
	// Surrounding whitespace is tolerated by the verifier.
	if !xendit.VerifyWebhookToken("  "+xnTestWebhookToken+"  ", configured) {
		t.Error("token with surrounding whitespace should still verify")
	}

	// The webhook payload parses into the typed Xendit shape used downstream.
	body := []byte(`{"event":"payment.capture","data":{"payment_request_id":"pr-001","reference_id":"xn-ref-001","status":"SUCCEEDED","request_amount":25000,"channel_code":"ALFAMART"}}`)
	var payload xendit.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal webhook payload: %v", err)
	}
	if !strings.EqualFold(payload.Data.Status, xendit.StatusSucceeded) {
		t.Errorf("payload status = %q, want SUCCEEDED", payload.Data.Status)
	}
	if got := mapXenditStatus(payload.Data.Status); got != models.PaymentStatusPaid {
		t.Errorf("webhook status %q mapped to %q, want Paid", payload.Data.Status, got)
	}
}
