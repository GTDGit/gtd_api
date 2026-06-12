package dana

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// VerifyWebhookSignature verifies an inbound DANA notification signature.
//
// DANA (SNAP BI) signs notifications with RSA (SHA256withRSA), validated with
// the sender's public key. The string to sign per DANA docs is:
//
//	<HTTP METHOD>:<RELATIVE PATH URL>:LowerCase(HexEncode(SHA-256(Minify(<BODY>)))):<X-TIMESTAMP>
//
// body MUST be the raw bytes exactly as received. pathCandidates lets the caller
// pass the request path (and any alternates) since the exact value DANA signs
// can differ from the gateway's external path.
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
