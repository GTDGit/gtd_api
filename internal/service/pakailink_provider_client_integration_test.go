package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/pakailink"
)

// Integration tests for PakailinkProviderClient (task 11.2, Req 10.1-10.6).
//
// These tests use net/http/httptest to serve canned Pakailink SNAP responses
// and point a real pkg/pakailink.Client at the test server. SNAP signing is
// satisfied with a throwaway RSA key generated per test, so the full create →
// token → signed-request → response round-trip is exercised. The httptest
// handler captures the requested path and body so we can assert the correct
// endpoint is hit and (for e-money) the PAY* product code is carried on the
// wire while the canonical code stays unchanged.
//
// Helpers are prefixed with plTest to avoid collisions with other Pakailink
// tests in the package.

// plTestCapture records what the most recent SNAP operation request carried.
type plTestCapture struct {
	Path string
	Body map[string]any
}

// plTestKeyPEM generates a throwaway PKCS1 RSA private key in PEM form so the
// SNAP asymmetric token signature can be produced without real credentials.
func plTestKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	return string(pemBytes)
}

// plTestServer spins up an httptest server that always answers the SNAP B2B
// token request and routes operation paths to opHandler. The token path is
// handled transparently so each test only cares about the operation endpoint.
// The op request path and JSON body are captured into cap.
func plTestServer(t *testing.T, cap *plTestCapture, opPath string, opResponse map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == pakailink.TokenPath {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"responseCode":    "2007300",
				"responseMessage": "Successful",
				"accessToken":     "pl-test-access-token",
				"tokenType":       "BearerToken",
				"expiresIn":       "900",
			})
			return
		}
		cap.Path = r.URL.Path
		body := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		cap.Body = body
		if r.URL.Path == opPath {
			_ = json.NewEncoder(w).Encode(opResponse)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"responseCode":    "4040000",
			"responseMessage": "Unexpected path: " + r.URL.Path,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// plTestClient builds a PakailinkProviderClient backed by a real pakailink.Client
// pointed at baseURL with a throwaway signing key.
func plTestClient(t *testing.T, baseURL string) *PakailinkProviderClient {
	t.Helper()
	c, err := pakailink.NewClient(pakailink.Config{
		BaseURL:       baseURL,
		ClientID:      "pl-test-client",
		ClientSecret:  "pl-test-secret",
		PartnerID:     "pl-test-partner",
		PrivateKeyPEM: plTestKeyPEM(t),
	})
	if err != nil {
		t.Fatalf("new pakailink client: %v", err)
	}
	return NewPakailinkProviderClient(c, "https://merchant.example/callback")
}

func plTestEwalletMethod(code string) *models.PaymentMethod {
	return &models.PaymentMethod{Type: models.PaymentTypeEwallet, Code: code, Name: code + " Wallet"}
}

// Req 10.1: Create VA hits the create-VA endpoint and returns paymentDetail.vaNumber.
func TestPakailinkIntegration_CreateVA_ReturnsVANumber(t *testing.T) {
	cap := &plTestCapture{}
	resp := map[string]any{
		"responseCode":    "2002700",
		"responseMessage": "Successful",
		"virtualAccountData": map[string]any{
			"partnerReferenceNo": "pl-ref-001",
			"customerNo":         "pl-ref-001",
			"virtualAccountNo":   "01410001234567",
			"virtualAccountName": "John Doe",
			"totalAmount":        map[string]any{"value": "25000.00", "currency": "IDR"},
		},
	}
	srv := plTestServer(t, cap, pakailink.CreateVAPath, resp)
	client := plTestClient(t, srv.URL)

	method := &models.PaymentMethod{Type: models.PaymentTypeVA, Code: "014", Name: "BCA Virtual Account"}
	req := &PaymentCreateRequest{
		Type:         models.PaymentTypeVA,
		Code:         "014",
		PartnerRef:   "pl-ref-001",
		TotalAmount:  25000,
		CustomerName: "John Doe",
	}

	out, err := client.CreatePayment(context.Background(), method, req)
	if err != nil {
		t.Fatalf("CreatePayment VA: %v", err)
	}
	if cap.Path != pakailink.CreateVAPath {
		t.Errorf("VA endpoint = %q, want %q", cap.Path, pakailink.CreateVAPath)
	}
	if out.Normalized.VANumber != "01410001234567" {
		t.Errorf("vaNumber = %q, want %q", out.Normalized.VANumber, "01410001234567")
	}
	if out.Normalized.BankCode != "014" {
		t.Errorf("bankCode = %q, want %q", out.Normalized.BankCode, "014")
	}
}

// Req 10.2 / Req 1.x: Create e-money hits the e-money endpoint, the wire request
// carries the correct PAY* product code, and the canonical method code stays
// unchanged (wire-only mapping).
func TestPakailinkIntegration_CreateEmoney_WireProductCodeAndCanonicalCode(t *testing.T) {
	cases := []struct {
		code        string
		wantProduct string
	}{
		{"DANA", "PAYDANA"},
		{"OVO", "PAYOVO"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			cap := &plTestCapture{}
			resp := map[string]any{
				"responseCode":    "2005400",
				"responseMessage": "Successful",
				"emoneyData": map[string]any{
					"partnerReferenceNo": "pl-emoney-001",
					"referenceNo":        "PL-EMN-XYZ",
					"customerId":         "pl-emoney-001",
					"customerName":       "Jane Doe",
					"totalAmount":        map[string]any{"value": "50000.00", "currency": "IDR"},
					"additionalInfo": map[string]any{
						"urlPayment": "https://pay.pakailink.test/redirect/abc",
					},
				},
			}
			srv := plTestServer(t, cap, pakailink.CreateEmoneyPath, resp)
			client := plTestClient(t, srv.URL)

			method := plTestEwalletMethod(tc.code)
			req := &PaymentCreateRequest{
				Type:          models.PaymentTypeEwallet,
				Code:          tc.code,
				PartnerRef:    "pl-emoney-001",
				TotalAmount:   50000,
				CustomerName:  "Jane Doe",
				CustomerPhone: "08123456789",
			}

			out, err := client.CreatePayment(context.Background(), method, req)
			if err != nil {
				t.Fatalf("CreatePayment e-money %s: %v", tc.code, err)
			}
			if cap.Path != pakailink.CreateEmoneyPath {
				t.Errorf("e-money endpoint = %q, want %q", cap.Path, pakailink.CreateEmoneyPath)
			}
			// Wire request must carry the PAY* product code (nested in additionalInfo).
			ai, _ := cap.Body["additionalInfo"].(map[string]any)
			gotProduct, _ := ai["productCode"].(string)
			if gotProduct != tc.wantProduct {
				t.Errorf("wire productCode = %q, want %q", gotProduct, tc.wantProduct)
			}
			// Canonical code on the method stays the plain wallet name (never PAY*).
			if method.Code != tc.code {
				t.Errorf("canonical method code mutated to %q, want %q", method.Code, tc.code)
			}
			if out.Normalized.Deeplink != "https://pay.pakailink.test/redirect/abc" {
				t.Errorf("deeplink = %q, want canned urlPayment", out.Normalized.Deeplink)
			}
		})
	}
}

// Req 10.3: Generate QR MPM hits the QR endpoint and returns paymentDetail.qrString.
func TestPakailinkIntegration_GenerateQRMPM_ReturnsQRString(t *testing.T) {
	cap := &plTestCapture{}
	resp := map[string]any{
		"responseCode":    "2004900",
		"responseMessage": "Successful",
		"referenceNo":     "PL-QR-001",
		"qrContent":       "00020101021126670016COM.PAKAILINK.WWW...6304ABCD",
		"additionalInfo": map[string]any{
			"qrImageUrl": "https://qr.pakailink.test/img/001.png",
		},
	}
	srv := plTestServer(t, cap, pakailink.GenerateQRPath, resp)
	client := plTestClient(t, srv.URL)

	method := &models.PaymentMethod{Type: models.PaymentTypeQRIS, Code: "MPM", Name: "QRIS"}
	req := &PaymentCreateRequest{
		Type:        models.PaymentTypeQRIS,
		Code:        "MPM",
		PartnerRef:  "pl-qr-0000000000000001",
		TotalAmount: 15000,
	}

	out, err := client.CreatePayment(context.Background(), method, req)
	if err != nil {
		t.Fatalf("CreatePayment QRIS MPM: %v", err)
	}
	if cap.Path != pakailink.GenerateQRPath {
		t.Errorf("QR endpoint = %q, want %q", cap.Path, pakailink.GenerateQRPath)
	}
	if out.Normalized.QRString != "00020101021126670016COM.PAKAILINK.WWW...6304ABCD" {
		t.Errorf("qrString = %q, want canned qrContent", out.Normalized.QRString)
	}
	if out.Normalized.QRImageURL != "https://qr.pakailink.test/img/001.png" {
		t.Errorf("qrImageUrl = %q, want canned qrImageUrl", out.Normalized.QRImageURL)
	}
}

// Req 10.4: The Pakailink adapter does not implement a RETAIL flow — pkg/pakailink
// exposes no retail endpoint, and CreatePayment rejects RETAIL with
// UNSUPPORTED_PAYMENT_TYPE. We assert that real behavior so the gap is documented
// and any future retail implementation will surface here. (Pakailink retail
// paymentCode is currently delivered through the e-money flow instead.)
func TestPakailinkIntegration_CreateRetail_Unsupported(t *testing.T) {
	cap := &plTestCapture{}
	// No retail endpoint exists; serve a stub that should never be reached.
	srv := plTestServer(t, cap, "/snap/v1.0/payment/retail-unused", map[string]any{
		"responseCode": "2009900",
	})
	client := plTestClient(t, srv.URL)

	method := &models.PaymentMethod{Type: models.PaymentTypeRetail, Code: "ALFAMART", Name: "Alfamart"}
	req := &PaymentCreateRequest{
		Type:        models.PaymentTypeRetail,
		Code:        "ALFAMART",
		PartnerRef:  "pl-retail-001",
		TotalAmount: 30000,
	}

	_, err := client.CreatePayment(context.Background(), method, req)
	if err == nil {
		t.Fatal("expected error for unsupported RETAIL flow, got nil")
	}
	var svcErr *PaymentServiceError
	if !errors.As(err, &svcErr) {
		t.Fatalf("error is not *PaymentServiceError: %T", err)
	}
	if svcErr.Code != "UNSUPPORTED_PAYMENT_TYPE" {
		t.Errorf("error code = %q, want UNSUPPORTED_PAYMENT_TYPE", svcErr.Code)
	}
	if cap.Path != "" {
		t.Errorf("unsupported RETAIL should not hit any provider endpoint, hit %q", cap.Path)
	}
}

// Req 10.5: Inquiry status maps the provider's latestTransactionStatus to the
// internal PaymentStatus, and hits the type-appropriate VA status endpoint.
func TestPakailinkIntegration_InquiryVA_MapsProviderStatus(t *testing.T) {
	cases := []struct {
		name       string
		providerSt string
		want       models.PaymentStatus
	}{
		{"paid", pakailink.StatusSuccess, models.PaymentStatusPaid},
		{"cancelled", pakailink.StatusCancelled, models.PaymentStatusCancelled},
		{"expired", "07", models.PaymentStatusExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cap := &plTestCapture{}
			resp := map[string]any{
				"responseCode":               "2002600",
				"responseMessage":            "Successful",
				"originalPartnerReferenceNo": "pl-ref-001",
				"originalReferenceNo":        "PL-VA-REF",
				"latestTransactionStatus":    tc.providerSt,
				"amount":                     map[string]any{"value": "25000.00", "currency": "IDR"},
			}
			srv := plTestServer(t, cap, pakailink.InquiryVAPath, resp)
			client := plTestClient(t, srv.URL)

			payment := &models.Payment{
				PaymentID:   "pl-ref-001",
				PaymentType: models.PaymentTypeVA,
			}
			res, err := client.InquiryPayment(context.Background(), payment)
			if err != nil {
				t.Fatalf("InquiryPayment VA: %v", err)
			}
			if cap.Path != pakailink.InquiryVAPath {
				t.Errorf("inquiry endpoint = %q, want %q", cap.Path, pakailink.InquiryVAPath)
			}
			if res.Status != tc.want {
				t.Errorf("status %q mapped to %q, want %q", tc.providerSt, res.Status, tc.want)
			}
			if tc.want == models.PaymentStatusPaid && res.PaidAmount != 25000 {
				t.Errorf("paidAmount = %d, want 25000", res.PaidAmount)
			}
		})
	}
}

// Req 10.6: Pakailink callbacks are verified with the SNAP symmetric signature
// before processing. This exercises the smallest meaningful surface of the
// callback path: a signature produced with the shared client secret verifies,
// the payload parses, and its transaction status maps to the internal status.
// (Full webhook ingestion lives in the HTTP handler layer; here we cover the
// provider-package verification + parsing the adapter relies on.)
func TestPakailinkIntegration_Callback_VerifyAndParse(t *testing.T) {
	const secret = "pl-test-secret"
	const path = "/v1/webhook/pakailink"
	const timestamp = "2024-01-02T15:04:05+07:00"

	body := []byte(`{"responseCode":"2002600","responseMessage":"Success","originalPartnerReferenceNo":"pl-ref-001","latestTransactionStatus":"00","serviceCode":"52","amount":{"value":"25000.00","currency":"IDR"}}`)

	// Recompute the signature exactly as a verifier would, then confirm it passes.
	sig := plTestSNAPSymmetricSig("POST", path, "", body, timestamp, secret)
	if !pakailink.VerifyWebhookSignature("POST", path, "", body, timestamp, sig, secret) {
		t.Fatal("expected webhook signature to verify with shared secret")
	}
	if pakailink.VerifyWebhookSignature("POST", path, "", body, timestamp, sig, "wrong-secret") {
		t.Fatal("signature must not verify under a different secret")
	}

	var payload pakailink.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal webhook payload: %v", err)
	}
	data := payload.ResolveTransactionData()
	if got := mapPakailinkTransactionStatus(data.PaymentFlagStatus); got != models.PaymentStatusPaid {
		t.Errorf("callback status %q mapped to %q, want Paid", data.PaymentFlagStatus, got)
	}
}

// plTestSNAPSymmetricSig independently reproduces the SNAP symmetric signature
// (Base64(HMAC-SHA512(<method>:<path>:<token>:<sha256hex(body)>:<timestamp>, secret)))
// used by pkg/pakailink. The webhook body in the test is already compact JSON,
// so no JSON minification is needed for the hash to match the package's signer.
func plTestSNAPSymmetricSig(method, path, token string, body []byte, timestamp, secret string) string {
	bodyHash := sha256.Sum256(body)
	stringToSign := strings.Join([]string{
		method,
		path,
		token,
		strings.ToLower(hex.EncodeToString(bodyHash[:])),
		timestamp,
	}, ":")
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
