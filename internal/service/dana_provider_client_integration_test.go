package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/dana"
)

// Task 11.1 — Mocked-HTTP integration tests for the DANA provider adapter.
//
// These tests stand up an httptest server that serves canned DANA SNAP/QRIS
// responses, point a real dana.Client at it, and exercise DanaProviderClient
// end-to-end. They assert that each flow hits the correct DANA endpoint path
// and that the adapter normalizes the provider response into the shared
// PaymentDetailNormalized shape.
//
// Requirements: 9.1, 9.2, 9.3, 9.4, 9.5, 9.6
//
// Signing note: pkg/dana signs every request (RSA asymmetric for QRIS/create,
// HMAC for SNAP cancel after fetching an access token). The mock server does
// not verify signatures, so a throwaway RSA key generated in-test is enough to
// satisfy dana.NewClient and produce a full HTTP round-trip.

// danaTestRecorder captures the request paths seen by the mock DANA server so a
// test can assert the adapter called the expected endpoint.
type danaTestRecorder struct {
	mu    sync.Mutex
	paths []string
}

func (r *danaTestRecorder) record(p string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, p)
}

func (r *danaTestRecorder) contains(p string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, got := range r.paths {
		if got == p {
			return true
		}
	}
	return false
}

func (r *danaTestRecorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.paths))
	copy(out, r.paths)
	return out
}

// danaTestPrivateKeyPEM generates a throwaway RSA private key in PKCS1 PEM form.
func danaTestPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("danaTest: generate RSA key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	return string(pemBytes)
}

// danaTestServer starts a mock DANA server. It always answers the access-token
// endpoint (needed by SNAP cancel) and serves the canned body registered for
// each path in responses. Every request path is recorded.
func danaTestServer(t *testing.T, responses map[string]string) (*httptest.Server, *danaTestRecorder) {
	t.Helper()
	rec := &danaTestRecorder{}
	const tokenBody = `{"responseCode":"2007300","responseMessage":"Successful","accessToken":"danaTest-token","tokenType":"Bearer","expiresIn":"900"}`

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == dana.TokenPath {
			_, _ = w.Write([]byte(tokenBody))
			return
		}
		if body, ok := responses[r.URL.Path]; ok {
			_, _ = w.Write([]byte(body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"responseCode":"4040000","responseMessage":"Not found"}`))
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, rec
}

// danaTestClient builds a dana.Client wired to the mock server.
func danaTestClient(t *testing.T, server *httptest.Server) *dana.Client {
	t.Helper()
	client, err := dana.NewClient(dana.Config{
		BaseURL:       server.URL,
		MerchantID:    "danaTestMerchant",
		ClientID:      "danaTestClient",
		ClientSecret:  "danaTestSecret",
		PartnerID:     "danaTestPartner",
		PrivateKeyPEM: danaTestPrivateKeyPEM(t),
		HTTPClient:    server.Client(),
	})
	if err != nil {
		t.Fatalf("danaTest: new dana client: %v", err)
	}
	return client
}

// TestDanaIntegration_CreateEwallet_HitsCreateOrderAndNormalizes verifies that
// an e-wallet create call hits create-order (payment-host-to-host) and that the
// adapter normalizes checkoutUrl / mobileWebUrl / deeplink for the EWALLET type.
//
// Requirements: 9.1
func TestDanaIntegration_CreateEwallet_HitsCreateOrderAndNormalizes(t *testing.T) {
	const createBody = `{
		"responseCode":"2005400",
		"responseMessage":"Successful",
		"referenceNo":"danaTest-ref-ewallet",
		"partnerReferenceNo":"danaTestRef01",
		"checkoutUrl":"https://dana.test/checkout/abc",
		"webRedirectUrl":"https://dana.test/m/abc",
		"deeplinkUrl":"danatest://pay/abc"
	}`
	server, rec := danaTestServer(t, map[string]string{
		dana.CreateOrderPath: createBody,
	})
	adapter := NewDanaProviderClient(danaTestClient(t, server), "", "")

	method := &models.PaymentMethod{Type: models.PaymentTypeEwallet, Code: "PAYDANA", Name: "DANA"}
	req := &PaymentCreateRequest{
		Type:        models.PaymentTypeEwallet,
		Code:        "PAYDANA",
		PartnerRef:  "danaTestRef01",
		Amount:      10000,
		TotalAmount: 10000,
	}

	resp, err := adapter.CreatePayment(context.Background(), method, req)
	if err != nil {
		t.Fatalf("CreatePayment ewallet: unexpected error: %v", err)
	}
	if !rec.contains(dana.CreateOrderPath) {
		t.Fatalf("expected create-order endpoint %q to be called, got paths %v", dana.CreateOrderPath, rec.all())
	}
	if resp.ProviderRef != "danaTest-ref-ewallet" {
		t.Errorf("ProviderRef = %q, want %q", resp.ProviderRef, "danaTest-ref-ewallet")
	}
	if resp.Normalized.CheckoutURL != "https://dana.test/checkout/abc" {
		t.Errorf("CheckoutURL = %q, want %q", resp.Normalized.CheckoutURL, "https://dana.test/checkout/abc")
	}
	if resp.Normalized.MobileWebURL != "https://dana.test/m/abc" {
		t.Errorf("MobileWebURL = %q, want %q", resp.Normalized.MobileWebURL, "https://dana.test/m/abc")
	}
	if resp.Normalized.Deeplink != "danatest://pay/abc" {
		t.Errorf("Deeplink = %q, want %q", resp.Normalized.Deeplink, "danatest://pay/abc")
	}
	// EWALLET must not leak a QRIS field.
	if resp.Normalized.QRString != "" {
		t.Errorf("QRString = %q, want empty for EWALLET", resp.Normalized.QRString)
	}
}

// TestDanaIntegration_CreateQRISMPM_HitsCreateOrderReturnsQRString verifies that
// QRIS MPM uses create-order (Gapura Custom Checkout) and that the QR string
// from additionalInfo.paymentCode is normalized into qrString.
//
// Requirements: 9.1, 9.2
func TestDanaIntegration_CreateQRISMPM_HitsCreateOrderReturnsQRString(t *testing.T) {
	const qrString = "00020101021226660014ID.DANA.WWW01189360091800000000000208DANATEST5204"
	createBody := `{
		"responseCode":"2005400",
		"responseMessage":"Successful",
		"referenceNo":"danaTest-ref-qrmpm",
		"additionalInfo":{"paymentCode":"` + qrString + `"}
	}`
	server, rec := danaTestServer(t, map[string]string{
		dana.CreateOrderPath: createBody,
	})
	adapter := NewDanaProviderClient(danaTestClient(t, server), "", "")

	method := &models.PaymentMethod{Type: models.PaymentTypeQRIS, Code: "MPM", Name: "QRIS"}
	req := &PaymentCreateRequest{
		Type:        models.PaymentTypeQRIS,
		Code:        "MPM",
		PartnerRef:  "danaTestRefMPM",
		Amount:      25000,
		TotalAmount: 25000,
	}

	resp, err := adapter.CreatePayment(context.Background(), method, req)
	if err != nil {
		t.Fatalf("CreatePayment QRIS MPM: unexpected error: %v", err)
	}
	if !rec.contains(dana.CreateOrderPath) {
		t.Fatalf("expected create-order endpoint %q, got paths %v", dana.CreateOrderPath, rec.all())
	}
	if resp.Normalized.QRString != qrString {
		t.Errorf("QRString = %q, want %q", resp.Normalized.QRString, qrString)
	}
	// QRIS must not populate e-wallet redirect fields.
	if resp.Normalized.CheckoutURL != "" || resp.Normalized.Deeplink != "" {
		t.Errorf("QRIS MPM leaked ewallet fields: checkoutUrl=%q deeplink=%q", resp.Normalized.CheckoutURL, resp.Normalized.Deeplink)
	}
}

// TestDanaIntegration_CreateQRISCPM_HitsCPMEndpointWithScanData verifies that
// QRIS CPM uses the cpm-payment endpoint, passes the scanData (qrContent) on
// the wire, and echoes it back as qrString.
//
// Requirements: 9.2
func TestDanaIntegration_CreateQRISCPM_HitsCPMEndpointWithScanData(t *testing.T) {
	const scanData = "00020101021126danaTestCustomerPresentedQR5204"
	const cpmBody = `{
		"responseCode":"2005400",
		"responseMessage":"Successful",
		"referenceNo":"danaTest-ref-cpm"
	}`
	server, rec := danaTestServer(t, map[string]string{
		dana.CPMPaymentPath: cpmBody,
	})

	var capturedQRContent string
	// Wrap the recorder by re-registering a handler that inspects the body.
	// Simpler: decode the request body in a custom server.
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == dana.CPMPaymentPath {
			var payload struct {
				QRContent string `json:"qrContent"`
			}
			_ = decodeJSONBody(r, &payload)
			capturedQRContent = payload.QRContent
			_, _ = w.Write([]byte(cpmBody))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	adapter := NewDanaProviderClient(danaTestClient(t, server), "", "")
	adapter.SetExternalStoreID("danaTestStore01")

	method := &models.PaymentMethod{Type: models.PaymentTypeQRIS, Code: "CPM", Name: "QRIS CPM"}
	req := &PaymentCreateRequest{
		Type:        models.PaymentTypeQRIS,
		Code:        "CPM",
		PartnerRef:  "danaTestRefCPM",
		Amount:      15000,
		TotalAmount: 15000,
		ScanData:    scanData,
	}

	resp, err := adapter.CreatePayment(context.Background(), method, req)
	if err != nil {
		t.Fatalf("CreatePayment QRIS CPM: unexpected error: %v", err)
	}
	if !rec.contains(dana.CPMPaymentPath) {
		t.Fatalf("expected cpm-payment endpoint %q, got paths %v", dana.CPMPaymentPath, rec.all())
	}
	if capturedQRContent != scanData {
		t.Errorf("qrContent sent to provider = %q, want scanData %q", capturedQRContent, scanData)
	}
	if resp.Normalized.QRString != scanData {
		t.Errorf("QRString = %q, want echoed scanData %q", resp.Normalized.QRString, scanData)
	}
}

// TestDanaIntegration_CreateQRISCPM_MissingScanDataRejected verifies that the
// adapter rejects a CPM create with MISSING_FIELD when scanData is absent
// (this enforcement lives in DanaProviderClient.createCPMQRIS, before any HTTP
// call). PaymentService.CreatePayment also performs an earlier CPM scanData
// check in the generic create path; the adapter provides defense-in-depth.
//
// Requirements: 9.2, 9.6
func TestDanaIntegration_CreateQRISCPM_MissingScanDataRejected(t *testing.T) {
	server, rec := danaTestServer(t, map[string]string{})
	adapter := NewDanaProviderClient(danaTestClient(t, server), "", "")
	adapter.SetExternalStoreID("danaTestStore01") // set so storeID check passes first

	method := &models.PaymentMethod{Type: models.PaymentTypeQRIS, Code: "CPM", Name: "QRIS CPM"}
	req := &PaymentCreateRequest{
		Type:        models.PaymentTypeQRIS,
		Code:        "CPM",
		PartnerRef:  "danaTestRefCPM",
		Amount:      15000,
		TotalAmount: 15000,
		// ScanData intentionally omitted.
	}

	_, err := adapter.CreatePayment(context.Background(), method, req)
	if err == nil {
		t.Fatalf("expected MISSING_FIELD error, got nil")
	}
	pse, ok := err.(*PaymentServiceError)
	if !ok {
		t.Fatalf("expected *PaymentServiceError, got %T: %v", err, err)
	}
	if pse.Code != "MISSING_FIELD" {
		t.Errorf("error code = %q, want MISSING_FIELD", pse.Code)
	}
	// The adapter must reject before making any provider HTTP call.
	if len(rec.all()) != 0 {
		t.Errorf("expected no provider calls on missing scanData, got %v", rec.all())
	}
}

// TestDanaIntegration_QueryPayment_MapsProviderStatus verifies that query-payment
// (Gapura PG status, serviceCode 55) is hit and the provider transaction status
// is mapped to the canonical PaymentStatus.
//
// Requirements: 9.3
func TestDanaIntegration_QueryPayment_MapsProviderStatus(t *testing.T) {
	cases := []struct {
		name       string
		statusCode string
		want       models.PaymentStatus
	}{
		{"paid", "00", models.PaymentStatusPaid},
		{"cancelled", "05", models.PaymentStatusCancelled},
		{"pending", "01", models.PaymentStatusPending},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inquiryBody := `{
				"responseCode":"2005500",
				"responseMessage":"Successful",
				"originalReferenceNo":"danaTest-orig-ref",
				"latestTransactionStatus":"` + tc.statusCode + `",
				"amount":{"value":"10000.00","currency":"IDR"}
			}`
			server, rec := danaTestServer(t, map[string]string{
				dana.InquiryPath: inquiryBody,
			})
			adapter := NewDanaProviderClient(danaTestClient(t, server), "", "")

			payment := &models.Payment{PaymentID: "danaTestRef01", PaymentCode: "MPM"}
			result, err := adapter.InquiryPayment(context.Background(), payment)
			if err != nil {
				t.Fatalf("InquiryPayment: unexpected error: %v", err)
			}
			if !rec.contains(dana.InquiryPath) {
				t.Fatalf("expected query-payment endpoint %q, got paths %v", dana.InquiryPath, rec.all())
			}
			if result.Status != tc.want {
				t.Errorf("Status = %q, want %q", result.Status, tc.want)
			}
			if result.ProviderRef != "danaTest-orig-ref" {
				t.Errorf("ProviderRef = %q, want %q", result.ProviderRef, "danaTest-orig-ref")
			}
		})
	}
}

// TestDanaIntegration_QueryPaymentCPM_HitsCPMStatusEndpoint verifies that a CPM
// payment inquiry routes to the CPM Acquirer status endpoint (serviceCode 60).
//
// Requirements: 9.3
func TestDanaIntegration_QueryPaymentCPM_HitsCPMStatusEndpoint(t *testing.T) {
	inquiryBody := `{
		"responseCode":"2006000",
		"responseMessage":"Successful",
		"originalReferenceNo":"danaTest-cpm-ref",
		"latestTransactionStatus":"00",
		"amount":{"value":"15000.00","currency":"IDR"}
	}`
	server, rec := danaTestServer(t, map[string]string{
		dana.CPMInquiryPath: inquiryBody,
	})
	adapter := NewDanaProviderClient(danaTestClient(t, server), "", "")

	payment := &models.Payment{PaymentID: "danaTestRefCPM", PaymentCode: "CPM"}
	result, err := adapter.InquiryPayment(context.Background(), payment)
	if err != nil {
		t.Fatalf("InquiryPayment CPM: unexpected error: %v", err)
	}
	if !rec.contains(dana.CPMInquiryPath) {
		t.Fatalf("expected cpm status endpoint %q, got paths %v", dana.CPMInquiryPath, rec.all())
	}
	if result.Status != models.PaymentStatusPaid {
		t.Errorf("Status = %q, want %q", result.Status, models.PaymentStatusPaid)
	}
}

// TestDanaIntegration_CancelPayment_HitsCancelEndpoint verifies that cancel
// hits cancel-order. Cancel uses the SNAP HMAC flow, which first fetches an
// access token, so both the token and cancel endpoints are exercised.
//
// Requirements: 9.4
func TestDanaIntegration_CancelPayment_HitsCancelEndpoint(t *testing.T) {
	const cancelBody = `{
		"responseCode":"2005700",
		"responseMessage":"Successful",
		"originalReferenceNo":"danaTest-orig-ref",
		"cancelTime":"2024-01-01T10:00:00+07:00"
	}`
	server, rec := danaTestServer(t, map[string]string{
		dana.CancelPath: cancelBody,
	})
	adapter := NewDanaProviderClient(danaTestClient(t, server), "", "")

	payment := &models.Payment{PaymentID: "danaTestRef01", PaymentCode: "MPM"}
	result, err := adapter.CancelPayment(context.Background(), payment, "customer request")
	if err != nil {
		t.Fatalf("CancelPayment: unexpected error: %v", err)
	}
	if !rec.contains(dana.CancelPath) {
		t.Fatalf("expected cancel-order endpoint %q, got paths %v", dana.CancelPath, rec.all())
	}
	if !rec.contains(dana.TokenPath) {
		t.Fatalf("expected SNAP cancel to fetch access token at %q, got paths %v", dana.TokenPath, rec.all())
	}
	if !result.Cancelled {
		t.Errorf("Cancelled = false, want true")
	}
}

// decodeJSONBody is a tiny helper to decode a request body in tests.
func decodeJSONBody(r *http.Request, out any) error {
	return json.NewDecoder(r.Body).Decode(out)
}
