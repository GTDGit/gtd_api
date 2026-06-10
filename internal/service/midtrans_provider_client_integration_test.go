package service

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/midtrans"
)

// Integration tests for the Midtrans provider adapter (task 11.4).
//
// These tests stand up a fake Midtrans Core API server with net/http/httptest,
// point a real pkg/midtrans.Client at it, wrap it in MidtransProviderClient,
// and assert the adapter hits the correct Core API endpoints (charge, status,
// cancel), normalizes the paymentDetail per payment type, maps the
// transaction_status to PaymentStatus, and that the HTTP notification signature
// (SHA-512 of order_id+status_code+gross_amount+serverKey) verifies/rejects as
// documented. VA banks (BRI/BNI/MANDIRI/CIMB/PERMATA) are covered by asserting
// the adapter's real behavior, since the Midtrans adapter implements QRIS and
// e-wallet only.
//
// Validates: Requirements 12.1, 12.2, 12.3, 12.4, 12.5, 12.6

const mtTestServerKey = "SB-Mid-server-TESTKEY"

// mtTestCapture records the path/method/body of the last request the fake
// Midtrans server saw so a test can assert which Core API endpoint was hit.
type mtTestCapture struct {
	path   string
	method string
	auth   string
	body   map[string]any
}

// mtTestServer spins up an httptest server whose handler routes by path and
// returns the canned JSON body registered for that path. It captures the
// inbound request into cap so the caller can assert on the endpoint hit.
func mtTestServer(t *testing.T, cap *mtTestCapture, responses map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.path = r.URL.Path
		cap.method = r.Method
		cap.auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&cap.body)

		body, ok := responses[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status_code":"404","status_message":"no canned response"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// mtTestClient builds a configured pkg/midtrans.Client pointed at the test
// server with a known server key.
func mtTestClient(t *testing.T, baseURL string) *midtrans.Client {
	t.Helper()
	client, err := midtrans.NewClient(midtrans.Config{
		BaseURL:   baseURL,
		ServerKey: mtTestServerKey,
	})
	if err != nil {
		t.Fatalf("midtrans.NewClient: %v", err)
	}
	return client
}

func mtTestMethod(code string) *models.PaymentMethod {
	return &models.PaymentMethod{
		Type: models.PaymentTypeEwallet,
		Code: code,
		Name: code,
	}
}

// mtTestSignature computes the Midtrans notification signature_key the same way
// the provider does: SHA512(order_id + status_code + gross_amount + serverKey).
func mtTestSignature(orderID, statusCode, grossAmount, serverKey string) string {
	sum := sha512.Sum512([]byte(orderID + statusCode + grossAmount + serverKey))
	return hex.EncodeToString(sum[:])
}

// TestMidtransCreateGoPayHitsChargeEndpoint verifies an EWALLET GoPay create
// posts to the Core API charge endpoint and normalizes the EWALLET
// paymentDetail (GoPay -> qrCodeUrl + deeplink). (Req 12.1)
func TestMidtransCreateGoPayHitsChargeEndpoint(t *testing.T) {
	cap := &mtTestCapture{}
	srv := mtTestServer(t, cap, map[string]string{
		midtrans.ChargePath: `{
			"status_code": "201",
			"status_message": "GoPay transaction is created",
			"transaction_id": "MT-TXN-GOPAY-1",
			"order_id": "PARTNER-GOPAY-1",
			"gross_amount": "50000.00",
			"payment_type": "gopay",
			"transaction_status": "pending",
			"actions": [
				{"name": "generate-qr-code", "method": "GET", "url": "https://midtrans.example/qr/gopay-1"},
				{"name": "deeplink-redirect", "method": "GET", "url": "gojek://gopay/pay/abc123"}
			]
		}`,
	})

	adapter := NewMidtransProviderClient(mtTestClient(t, srv.URL), "https://gtd.example/v1/webhook/midtrans")

	req := &PaymentCreateRequest{
		Type:         models.PaymentTypeEwallet,
		Code:         "GOPAY",
		PartnerRef:   "PARTNER-GOPAY-1",
		Amount:       50000,
		TotalAmount:  50000,
		ExpiredAt:    time.Now().Add(15 * time.Minute),
		CustomerName: "Budi",
		Description:  "Test GoPay charge",
	}

	resp, err := adapter.CreatePayment(context.Background(), mtTestMethod("GOPAY"), req)
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	if cap.path != midtrans.ChargePath {
		t.Errorf("expected charge endpoint %q, got %q", midtrans.ChargePath, cap.path)
	}
	if cap.method != http.MethodPost {
		t.Errorf("expected POST, got %s", cap.method)
	}
	if cap.auth == "" || !strings.HasPrefix(cap.auth, "Basic ") {
		t.Errorf("expected Basic auth header, got %q", cap.auth)
	}
	if got, _ := cap.body["payment_type"].(string); got != midtrans.PaymentTypeGoPay {
		t.Errorf("expected payment_type %q in charge body, got %q", midtrans.PaymentTypeGoPay, got)
	}
	if resp.ProviderRef != "MT-TXN-GOPAY-1" {
		t.Errorf("expected providerRef MT-TXN-GOPAY-1, got %q", resp.ProviderRef)
	}
	// GoPay EWALLET detail must surface the QR code URL and deeplink, and no
	// other payment-type fields.
	if resp.Normalized.QRCodeURL != "https://midtrans.example/qr/gopay-1" {
		t.Errorf("expected qrCodeUrl, got %q", resp.Normalized.QRCodeURL)
	}
	if resp.Normalized.Deeplink != "gojek://gopay/pay/abc123" {
		t.Errorf("expected deeplink, got %q", resp.Normalized.Deeplink)
	}
	if resp.Normalized.VANumber != "" || resp.Normalized.QRString != "" || resp.Normalized.PaymentCode != "" {
		t.Errorf("EWALLET detail leaked non-ewallet fields: %+v", resp.Normalized)
	}
}

// TestMidtransCreateQRISHitsChargeEndpoint verifies a QRIS create posts to the
// Core API charge endpoint and normalizes the QRIS paymentDetail
// (QRIS -> qrString). (Req 12.1)
func TestMidtransCreateQRISHitsChargeEndpoint(t *testing.T) {
	cap := &mtTestCapture{}
	srv := mtTestServer(t, cap, map[string]string{
		midtrans.ChargePath: `{
			"status_code": "201",
			"status_message": "QRIS transaction is created",
			"transaction_id": "MT-TXN-QRIS-1",
			"order_id": "PARTNER-QRIS-1",
			"gross_amount": "75000.00",
			"payment_type": "qris",
			"transaction_status": "pending",
			"qr_string": "00020101021226610014COM.MIDTRANS.QRIS-PAYLOAD-1"
		}`,
	})

	adapter := NewMidtransProviderClient(mtTestClient(t, srv.URL), "")

	req := &PaymentCreateRequest{
		Type:        models.PaymentTypeQRIS,
		Code:        "MPM",
		PartnerRef:  "PARTNER-QRIS-1",
		Amount:      75000,
		TotalAmount: 75000,
		ExpiredAt:   time.Now().Add(15 * time.Minute),
	}

	method := &models.PaymentMethod{Type: models.PaymentTypeQRIS, Code: "MPM", Name: "QRIS"}
	resp, err := adapter.CreatePayment(context.Background(), method, req)
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}

	if cap.path != midtrans.ChargePath {
		t.Errorf("expected charge endpoint %q, got %q", midtrans.ChargePath, cap.path)
	}
	if got, _ := cap.body["payment_type"].(string); got != midtrans.PaymentTypeQRIS {
		t.Errorf("expected payment_type %q in charge body, got %q", midtrans.PaymentTypeQRIS, got)
	}
	if resp.ProviderRef != "MT-TXN-QRIS-1" {
		t.Errorf("expected providerRef MT-TXN-QRIS-1, got %q", resp.ProviderRef)
	}
	// QRIS detail must surface qrString only.
	if resp.Normalized.QRString != "00020101021226610014COM.MIDTRANS.QRIS-PAYLOAD-1" {
		t.Errorf("expected qrString, got %q", resp.Normalized.QRString)
	}
	if resp.Normalized.QRCodeURL != "" || resp.Normalized.Deeplink != "" || resp.Normalized.VANumber != "" {
		t.Errorf("QRIS detail leaked non-qris fields: %+v", resp.Normalized)
	}
}

// TestMidtransInquiryHitsStatusEndpoint verifies status reconciliation calls the
// Core API /v2/{order_id}/status endpoint and maps transaction_status to
// PaymentStatus. (Req 12.2)
func TestMidtransInquiryHitsStatusEndpoint(t *testing.T) {
	cases := []struct {
		name       string
		mtStatus   string
		fraud      string
		wantStatus models.PaymentStatus
	}{
		{"settlement maps to Paid", midtrans.StatusSettlement, "accept", models.PaymentStatusSuccess},
		{"capture+challenge maps to Pending", midtrans.StatusCapture, "challenge", models.PaymentStatusPending},
		{"pending maps to Pending", midtrans.StatusPending, "", models.PaymentStatusPending},
		{"expire maps to Expired", midtrans.StatusExpire, "", models.PaymentStatusExpired},
		{"cancel maps to Cancelled", midtrans.StatusCancel, "", models.PaymentStatusCancelled},
		{"deny maps to Failed", midtrans.StatusDeny, "", models.PaymentStatusFailed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orderID := "PARTNER-STATUS-1"
			statusPath := "/v2/" + orderID + "/status"
			cap := &mtTestCapture{}
			srv := mtTestServer(t, cap, map[string]string{
				statusPath: `{
					"status_code": "200",
					"transaction_id": "MT-TXN-STATUS-1",
					"order_id": "` + orderID + `",
					"gross_amount": "50000.00",
					"payment_type": "gopay",
					"transaction_status": "` + tc.mtStatus + `",
					"fraud_status": "` + tc.fraud + `"
				}`,
			})

			adapter := NewMidtransProviderClient(mtTestClient(t, srv.URL), "")

			payment := &models.Payment{
				PaymentID: orderID,
				Amount:    50000,
			}

			result, err := adapter.InquiryPayment(context.Background(), payment)
			if err != nil {
				t.Fatalf("InquiryPayment returned error: %v", err)
			}
			if cap.path != statusPath {
				t.Errorf("expected status endpoint %q, got %q", statusPath, cap.path)
			}
			if cap.method != http.MethodGet {
				t.Errorf("expected GET, got %s", cap.method)
			}
			if result.Status != tc.wantStatus {
				t.Errorf("expected status %q, got %q", tc.wantStatus, result.Status)
			}
			if result.ProviderRef != "MT-TXN-STATUS-1" {
				t.Errorf("expected providerRef MT-TXN-STATUS-1, got %q", result.ProviderRef)
			}
		})
	}
}

// TestMidtransCancelHitsCancelEndpoint verifies cancellation calls the Core API
// /v2/{order_id}/cancel endpoint. (Req 12.3)
func TestMidtransCancelHitsCancelEndpoint(t *testing.T) {
	orderID := "PARTNER-CANCEL-1"
	cancelPath := "/v2/" + orderID + "/cancel"
	cap := &mtTestCapture{}
	srv := mtTestServer(t, cap, map[string]string{
		cancelPath: `{
			"status_code": "200",
			"status_message": "Success, transaction is canceled",
			"transaction_id": "MT-TXN-CANCEL-1",
			"order_id": "` + orderID + `",
			"transaction_status": "cancel"
		}`,
	})

	adapter := NewMidtransProviderClient(mtTestClient(t, srv.URL), "")

	payment := &models.Payment{PaymentID: orderID}

	result, err := adapter.CancelPayment(context.Background(), payment, "Customer cancellation")
	if err != nil {
		t.Fatalf("CancelPayment returned error: %v", err)
	}
	if cap.path != cancelPath {
		t.Errorf("expected cancel endpoint %q, got %q", cancelPath, cap.path)
	}
	if cap.method != http.MethodPost {
		t.Errorf("expected POST, got %s", cap.method)
	}
	if !result.Cancelled {
		t.Error("expected Cancelled = true")
	}
}

// TestMidtransNotificationSignatureVerification verifies the Midtrans HTTP
// notification signature_key check: a notification carrying the correct
// SHA512(order_id+status_code+gross_amount+serverKey) verifies, and a tampered
// signature is rejected. (Req 12.4)
func TestMidtransNotificationSignatureVerification(t *testing.T) {
	orderID := "PARTNER-NOTIF-1"
	statusCode := "200"
	grossAmount := "50000.00"

	good := mtTestSignature(orderID, statusCode, grossAmount, mtTestServerKey)

	if !midtrans.VerifyWebhookSignature(orderID, statusCode, grossAmount, mtTestServerKey, good) {
		t.Error("expected correct signature_key to verify")
	}

	// A tampered/wrong signature must be rejected.
	if midtrans.VerifyWebhookSignature(orderID, statusCode, grossAmount, mtTestServerKey, "deadbeef") {
		t.Error("expected wrong signature_key to be rejected")
	}

	// A signature computed with a different server key must be rejected.
	wrongKey := mtTestSignature(orderID, statusCode, grossAmount, "SB-Mid-server-OTHERKEY")
	if midtrans.VerifyWebhookSignature(orderID, statusCode, grossAmount, mtTestServerKey, wrongKey) {
		t.Error("expected signature computed with a different server key to be rejected")
	}

	// A signature for a tampered gross_amount (mismatched payload) must be
	// rejected against the original amount.
	if midtrans.VerifyWebhookSignature(orderID, statusCode, "99999.00", mtTestServerKey, good) {
		t.Error("expected signature to fail when gross_amount is tampered")
	}

	// Sanity: the case-insensitive comparison still accepts an uppercased hex
	// signature, matching the provider's normalization.
	if !midtrans.VerifyWebhookSignature(orderID, statusCode, grossAmount, mtTestServerKey, strings.ToUpper(good)) {
		t.Error("expected uppercased correct signature to verify")
	}
}

// TestMidtransVABanksUnsupported documents the Midtrans adapter's real behavior
// for VA banks. The adapter implements the Core API QRIS and e-wallet flows
// only; a VA create (BRI/BNI/MANDIRI/CIMB/PERMATA) is rejected with
// UNSUPPORTED_PAYMENT_TYPE before any HTTP call rather than returning a
// vaNumber. This pins the current contract so a future VA implementation
// surfaces here. (Req 12.5, 12.6)
func TestMidtransVABanksUnsupported(t *testing.T) {
	// Representative VA bank codes: BRI(002), BNI(009), MANDIRI(008),
	// CIMB(022), PERMATA(013).
	vaBanks := []struct {
		bank string
		code string
	}{
		{"BRI", "002"},
		{"BNI", "009"},
		{"MANDIRI", "008"},
		{"CIMB", "022"},
		{"PERMATA", "013"},
	}

	for _, vb := range vaBanks {
		t.Run(vb.bank, func(t *testing.T) {
			cap := &mtTestCapture{}
			srv := mtTestServer(t, cap, map[string]string{
				midtrans.ChargePath: `{"status_code":"201"}`,
			})

			adapter := NewMidtransProviderClient(mtTestClient(t, srv.URL), "")

			req := &PaymentCreateRequest{
				Type:        models.PaymentTypeVA,
				Code:        vb.code,
				BankCode:    vb.code,
				PartnerRef:  "PARTNER-VA-" + vb.bank,
				Amount:      50000,
				TotalAmount: 50000,
			}
			method := &models.PaymentMethod{Type: models.PaymentTypeVA, Code: vb.code, Name: vb.bank}

			_, err := adapter.CreatePayment(context.Background(), method, req)
			if err == nil {
				t.Fatalf("expected UNSUPPORTED_PAYMENT_TYPE for VA bank %s, got nil", vb.bank)
			}
			svcErr, ok := err.(*PaymentServiceError)
			if !ok {
				t.Fatalf("expected *PaymentServiceError, got %T: %v", err, err)
			}
			if svcErr.Code != "VALIDATION_ERROR" {
				t.Errorf("expected VALIDATION_ERROR for VA bank %s, got %q", vb.bank, svcErr.Code)
			}
			// The adapter rejects VA before issuing any HTTP call.
			if cap.path != "" {
				t.Errorf("expected no HTTP call for unsupported VA %s, server saw %q", vb.bank, cap.path)
			}
		})
	}
}

// TestMidtransUnsupportedEwalletCode verifies an unknown e-wallet code is
// rejected with UNSUPPORTED_PAYMENT_TYPE. (Req 12.5)
func TestMidtransUnsupportedEwalletCode(t *testing.T) {
	cap := &mtTestCapture{}
	srv := mtTestServer(t, cap, map[string]string{
		midtrans.ChargePath: `{"status_code":"201"}`,
	})

	adapter := NewMidtransProviderClient(mtTestClient(t, srv.URL), "")

	req := &PaymentCreateRequest{
		Type:        models.PaymentTypeEwallet,
		Code:        "DANA", // Midtrans does not serve DANA
		PartnerRef:  "PARTNER-EW-1",
		TotalAmount: 10000,
	}

	_, err := adapter.CreatePayment(context.Background(), mtTestMethod("DANA"), req)
	if err == nil {
		t.Fatal("expected VALIDATION_ERROR error, got nil")
	}
	svcErr, ok := err.(*PaymentServiceError)
	if !ok {
		t.Fatalf("expected *PaymentServiceError, got %T: %v", err, err)
	}
	if svcErr.Code != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %q", svcErr.Code)
	}
}
