package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/ovo"
)

// Integration tests for the OVO Direct provider adapter (task 10.2).
//
// These tests stand up a fake OVO partner server with net/http/httptest,
// point a real pkg/ovo.Client at it, wrap it in OVOProviderClient, and assert
// the adapter hits the correct endpoint paths, normalizes the EWALLET
// paymentDetail, maps statuses, and honors the unconfigured-fallback contract.
//
// Validates: Requirements 13.1, 13.2, 13.3, 13.4

// ovoTestCapture records the path/method that the fake OVO server last saw so a
// test can assert the adapter called the expected endpoint.
type ovoTestCapture struct {
	path   string
	method string
	body   map[string]any
}

// ovoTestServer spins up an httptest server whose handler routes by path and
// returns the canned JSON body registered for that path. It captures the
// inbound request into cap so the caller can assert on the endpoint hit.
func ovoTestServer(t *testing.T, cap *ovoTestCapture, responses map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.path = r.URL.Path
		cap.method = r.Method
		_ = json.NewDecoder(r.Body).Decode(&cap.body)

		body, ok := responses[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"responseCode":"404","responseMessage":"no canned response"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ovoTestClient builds a configured pkg/ovo.Client pointed at the test server.
func ovoTestClient(t *testing.T, baseURL string) *ovo.Client {
	t.Helper()
	client, err := ovo.NewClient(ovo.Config{
		BaseURL:      baseURL,
		MerchantID:   "TEST_MERCHANT",
		AppID:        "TEST_APP",
		ClientSecret: "test-secret",
	})
	if err != nil {
		t.Fatalf("ovo.NewClient: %v", err)
	}
	return client
}

func ovoTestMethod() *models.PaymentMethod {
	return &models.PaymentMethod{
		Type: models.PaymentTypeEwallet,
		Code: "OVO",
		Name: "OVO",
	}
}

func ovoTestStrPtr(s string) *string { return &s }

// TestOVOCreatePaymentHitsPushEndpoint verifies push-to-pay create calls the
// OVO push endpoint and returns a normalized EWALLET paymentDetail
// (deeplink/checkoutUrl) with the provider reference. (Req 13.1)
func TestOVOCreatePaymentHitsPushEndpoint(t *testing.T) {
	cap := &ovoTestCapture{}
	srv := ovoTestServer(t, cap, map[string]string{
		ovo.PushPaymentPath: `{
			"responseCode": "2000000",
			"responseMessage": "Successful",
			"referenceNo": "OVO-REF-123",
			"partnerReferenceNo": "PARTNER-1",
			"transactionStatus": "PENDING",
			"deeplink": "ovo://pay/abc123",
			"checkoutUrl": "https://ovo.example/checkout/abc123"
		}`,
	})

	adapter := NewOVOProviderClient(ovoTestClient(t, srv.URL), "https://gtd.example/v1/webhook/ovo")

	req := &PaymentCreateRequest{
		Type:          models.PaymentTypeEwallet,
		Code:          "OVO",
		PartnerRef:    "PARTNER-1",
		Amount:        50000,
		TotalAmount:   50000,
		ExpiredAt:     time.Now().Add(15 * time.Minute),
		CustomerPhone: "081234567890",
		Description:   "Test OVO push",
	}

	resp, err := adapter.CreatePayment(context.Background(), ovoTestMethod(), req)
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	if cap.path != ovo.PushPaymentPath {
		t.Errorf("expected push endpoint %q, got %q", ovo.PushPaymentPath, cap.path)
	}
	if cap.method != http.MethodPost {
		t.Errorf("expected POST, got %s", cap.method)
	}
	if resp.ProviderRef != "OVO-REF-123" {
		t.Errorf("expected providerRef OVO-REF-123, got %q", resp.ProviderRef)
	}
	// EWALLET normalized detail must surface the approval links and no other
	// payment-type fields.
	if resp.Normalized.Deeplink != "ovo://pay/abc123" {
		t.Errorf("expected deeplink, got %q", resp.Normalized.Deeplink)
	}
	if resp.Normalized.CheckoutURL != "https://ovo.example/checkout/abc123" {
		t.Errorf("expected checkoutUrl, got %q", resp.Normalized.CheckoutURL)
	}
	if resp.Normalized.VANumber != "" || resp.Normalized.QRString != "" || resp.Normalized.PaymentCode != "" {
		t.Errorf("EWALLET detail leaked non-ewallet fields: %+v", resp.Normalized)
	}

	// Confirm the customer MSISDN was forwarded in the push request body.
	if got, _ := cap.body["phone"].(string); got != "081234567890" {
		t.Errorf("expected phone forwarded in push body, got %q", got)
	}
}

// TestOVOCreatePaymentMissingPhone verifies a push without a customer phone is
// rejected with MISSING_FIELD before any HTTP call. (Req 13.1)
func TestOVOCreatePaymentMissingPhone(t *testing.T) {
	cap := &ovoTestCapture{}
	srv := ovoTestServer(t, cap, map[string]string{
		ovo.PushPaymentPath: `{"responseCode":"2000000"}`,
	})

	adapter := NewOVOProviderClient(ovoTestClient(t, srv.URL), "")

	req := &PaymentCreateRequest{
		Type:        models.PaymentTypeEwallet,
		Code:        "OVO",
		PartnerRef:  "PARTNER-2",
		Amount:      10000,
		TotalAmount: 10000,
		// CustomerPhone intentionally empty.
	}

	_, err := adapter.CreatePayment(context.Background(), ovoTestMethod(), req)
	if err == nil {
		t.Fatal("expected MISSING_FIELD error, got nil")
	}
	svcErr, ok := err.(*PaymentServiceError)
	if !ok {
		t.Fatalf("expected *PaymentServiceError, got %T: %v", err, err)
	}
	if svcErr.Code != "MISSING_FIELD" {
		t.Errorf("expected MISSING_FIELD, got %q", svcErr.Code)
	}
	if cap.path != "" {
		t.Errorf("expected no HTTP call on missing phone, but server saw %q", cap.path)
	}
}

// TestOVOInquiryPaymentHitsStatusEndpoint verifies status reconciliation calls
// the OVO status endpoint and maps provider statuses to PaymentStatus.
// (Req 13.2)
func TestOVOInquiryPaymentHitsStatusEndpoint(t *testing.T) {
	cases := []struct {
		name       string
		ovoStatus  string
		wantStatus models.PaymentStatus
	}{
		{"success maps to Paid", ovo.StatusSuccess, models.PaymentStatusSuccess},
		{"pending maps to Pending", ovo.StatusPending, models.PaymentStatusPending},
		{"expired maps to Expired", ovo.StatusExpired, models.PaymentStatusExpired},
		{"void maps to Cancelled", ovo.StatusVoided, models.PaymentStatusCancelled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cap := &ovoTestCapture{}
			srv := ovoTestServer(t, cap, map[string]string{
				ovo.StatusPath: `{
					"responseCode": "2000000",
					"referenceNo": "OVO-REF-123",
					"partnerReferenceNo": "PARTNER-1",
					"transactionStatus": "` + tc.ovoStatus + `",
					"amount": 50000
				}`,
			})

			adapter := NewOVOProviderClient(ovoTestClient(t, srv.URL), "")

			payment := &models.Payment{
				PaymentID:   "PARTNER-1",
				ProviderRef: ovoTestStrPtr("OVO-REF-123"),
				Amount:      50000,
			}

			result, err := adapter.InquiryPayment(context.Background(), payment)
			if err != nil {
				t.Fatalf("InquiryPayment returned error: %v", err)
			}
			if cap.path != ovo.StatusPath {
				t.Errorf("expected status endpoint %q, got %q", ovo.StatusPath, cap.path)
			}
			if result.Status != tc.wantStatus {
				t.Errorf("expected status %q, got %q", tc.wantStatus, result.Status)
			}
			if result.PaidAmount != 50000 {
				t.Errorf("expected paid amount 50000, got %d", result.PaidAmount)
			}
		})
	}
}

// TestOVOCancelPaymentHitsVoidEndpoint verifies cancellation calls the OVO void
// endpoint. (Req 13.2/13.4 cancel path)
func TestOVOCancelPaymentHitsVoidEndpoint(t *testing.T) {
	cap := &ovoTestCapture{}
	srv := ovoTestServer(t, cap, map[string]string{
		ovo.VoidPath: `{"responseCode":"2000000","responseMessage":"Voided"}`,
	})

	adapter := NewOVOProviderClient(ovoTestClient(t, srv.URL), "")

	payment := &models.Payment{
		PaymentID:   "PARTNER-1",
		ProviderRef: ovoTestStrPtr("OVO-REF-123"),
	}

	result, err := adapter.CancelPayment(context.Background(), payment, "Customer cancellation")
	if err != nil {
		t.Fatalf("CancelPayment returned error: %v", err)
	}
	if cap.path != ovo.VoidPath {
		t.Errorf("expected void endpoint %q, got %q", ovo.VoidPath, cap.path)
	}
	if !result.Cancelled {
		t.Error("expected Cancelled = true")
	}
}

// TestOVOAvailableFallbackContract verifies the unconfigured-fallback contract:
// a nil client reports Available() = false (so ProviderSelector routes to other
// OVO providers), while a configured client reports true. (Req 13.3)
func TestOVOAvailableFallbackContract(t *testing.T) {
	unconfigured := NewOVOProviderClient(nil, "")
	if unconfigured.Available() {
		t.Error("expected Available() = false when client is nil (unconfigured)")
	}

	cap := &ovoTestCapture{}
	srv := ovoTestServer(t, cap, map[string]string{})
	configured := NewOVOProviderClient(ovoTestClient(t, srv.URL), "")
	if !configured.Available() {
		t.Error("expected Available() = true when client is configured")
	}

	// Sanity: the adapter still reports the OVO Direct provider code.
	if configured.Code() != models.ProviderOVODirect {
		t.Errorf("expected provider code %q, got %q", models.ProviderOVODirect, configured.Code())
	}
}

// TestOVOCreatePaymentRejectsNonEwallet verifies OVO Direct refuses non-EWALLET
// types. (Req 13.1 — OVO supports e-wallet only)
func TestOVOCreatePaymentRejectsNonEwallet(t *testing.T) {
	cap := &ovoTestCapture{}
	srv := ovoTestServer(t, cap, map[string]string{
		ovo.PushPaymentPath: `{"responseCode":"2000000"}`,
	})

	adapter := NewOVOProviderClient(ovoTestClient(t, srv.URL), "")

	req := &PaymentCreateRequest{
		Type:          models.PaymentTypeVA,
		Code:          "014",
		PartnerRef:    "PARTNER-3",
		TotalAmount:   10000,
		CustomerPhone: "081234567890",
	}

	_, err := adapter.CreatePayment(context.Background(), ovoTestMethod(), req)
	if err == nil {
		t.Fatal("expected UNSUPPORTED_PAYMENT_TYPE error, got nil")
	}
	svcErr, ok := err.(*PaymentServiceError)
	if !ok {
		t.Fatalf("expected *PaymentServiceError, got %T: %v", err, err)
	}
	if svcErr.Code != "UNSUPPORTED_PAYMENT_TYPE" {
		t.Errorf("expected UNSUPPORTED_PAYMENT_TYPE, got %q", svcErr.Code)
	}
}
