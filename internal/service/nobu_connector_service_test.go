package service

import (
	"net/http"
	"testing"
)

// Test vector taken verbatim from "Nobu_Signature and Key Guidance v2.4" §C.
// It validates the full symmetric-signature path: minify(body) → SHA-256 →
// stringToSign(method:path:token:hash:timestamp) → HMAC-SHA512 → base64.
func TestNobuVerifyNotificationSignature_DocVector(t *testing.T) {
	const (
		clientSecret = "9ef0539f-b84a-4dad-98ef-bb5686006fe9"
		method       = http.MethodPost
		path         = "/v0.1/emoney/account-inquiry/"
		accessToken  = "Oh5ESxfUAgKuGQde0w3E627SGpHGJfk1F1NR8gLUFsNlPDt1kA2R2G"
		timestamp    = "2022-02-25T17:13:45+07:00"
		expectedSig  = "PdNqt5DmWY/pVEB20cI4L9ClQK1jJs0MB0WDb+d+iIgbTB8uNT9HXQgf/Jk7K+Zk3Q/WsJbqiOE0yYmur4+uPA=="
	)
	body := []byte(`{"partnerReferenceNo":"2020102900000000000001","customerNumber":"6287377388272","amount":{"value":"12345678.00","currency":"IDR"},"transactionDate":"2020-12-21T14:56:11+07:00","additionalInfo":{"partnerServiceId":"20fb646e","inquiryRequestId":"0f62c4af-4042-45f4-b197-36e820c80de9","emoneyType":"03","channel":"mobile"}}`)

	s := &NobuConnectorService{clientSecret: clientSecret}

	if err := s.verifyNotificationSignature(method, path, accessToken, timestamp, body, expectedSig); err != nil {
		t.Fatalf("verifyNotificationSignature rejected the doc test vector: %v", err)
	}

	// A tampered body must fail.
	if err := s.verifyNotificationSignature(method, path, accessToken, timestamp, append(body, ' '), expectedSig); err == nil {
		// trailing space is stripped by minify, so this should still PASS;
		// instead tamper inside a value.
		tampered := []byte(`{"partnerReferenceNo":"2020102900000000000002"}`)
		if err2 := s.verifyNotificationSignature(method, path, accessToken, timestamp, tampered, expectedSig); err2 == nil {
			t.Errorf("expected signature mismatch for tampered body, got nil")
		}
	}
}
