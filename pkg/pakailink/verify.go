package pakailink

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// VerifyWebhookSignature verifies an inbound SNAP callback signature.
//
// Pakailink (SNAP BI) signs callbacks with RSA (SHA256withRSA), NOT the HMAC
// symmetric scheme used for outbound request signing. The string to sign per
// Pakailink docs is:
//
//	<HTTP METHOD>:<PATH>:LowerCase(HexEncode(SHA-256(Minify(<BODY>)))):<X-TIMESTAMP>
//
// The docs label the 2nd field "PATH URL CALLBACK" but their example shows a
// full URL, so the exact value is ambiguous. We try a set of candidates (the
// request path and any configured full callback URLs) and accept if any
// verifies. body MUST be the raw bytes exactly as received.
func VerifyWebhookSignature(method string, pathCandidates []string, body []byte, timestamp, signature string, pub *rsa.PublicKey) bool {
	if signature == "" || pub == nil {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return false
	}
	bodyHashHex := strings.ToLower(hex.EncodeToString(sha256Sum(minifyJSON(body))))
	m := strings.ToUpper(method)
	for _, p := range pathCandidates {
		if p == "" {
			continue
		}
		stringToSign := m + ":" + p + ":" + bodyHashHex + ":" + timestamp
		digest := sha256.Sum256([]byte(stringToSign))
		if rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig) == nil {
			return true
		}
	}
	return false
}

func sha256Sum(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}
