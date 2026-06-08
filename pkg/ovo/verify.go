package ovo

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SignRequest computes the HMAC-SHA256 request signature OVO expects, rendered
// as lowercase hex. The signing string is METHOD:path:timestamp:hex(body).
//
// TODO(ovo-docs): confirm the exact signing string layout and digest encoding
// against the OVO partner technical documentation before production use.
func SignRequest(method, path string, body []byte, timestamp, clientSecret string) string {
	mac := hmac.New(sha256.New, []byte(clientSecret))
	mac.Write([]byte(strings.ToUpper(method)))
	mac.Write([]byte(":"))
	mac.Write([]byte(path))
	mac.Write([]byte(":"))
	mac.Write([]byte(timestamp))
	mac.Write([]byte(":"))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhookSignature recomputes the HMAC-SHA256 signature over the raw
// notification body and compares it, in constant time, against the signature
// header OVO sent. Both lowercase-hex and uppercase-hex encodings are accepted.
//
// TODO(ovo-docs): confirm whether OVO signs the raw body alone or a composed
// string (with timestamp/headers), and the digest encoding, against the OVO
// partner technical documentation.
func VerifyWebhookSignature(body []byte, signature, clientSecret string) bool {
	if signature == "" || clientSecret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(clientSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	provided := strings.ToLower(strings.TrimSpace(signature))
	return hmac.Equal([]byte(expected), []byte(provided))
}
