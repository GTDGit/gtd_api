// Package nobu is the outbound SNAP-BI client we use to call Nobu after a
// merchant has been provisioned (via the Excel form) — specifically the
// qr-mpm-generate API that returns a merchant's static QR string.
//
// Nobu has NO registration API, so this client deliberately exposes only the
// generate call plus B2B token minting. Auth mirrors the SNAP-BI scheme used by
// pkg/pakailink: an asymmetric SHA256withRSA signature over "clientKey|timestamp"
// to mint a B2B token, then a symmetric HMAC-SHA512 string-to-sign per request.
package nobu

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// SNAP-BI paths. Nobu's generate endpoint sits on v1.2; token on v2.0.
	TokenPath      = "/v2.0/access-token/b2b"
	GenerateQRPath = "/v1.2/qr/qr-mpm-generate"

	DefaultChannelID = "95221"
)

// Config holds the credentials for the outbound Nobu generate client. The
// signing key is OUR RSA private key; ClientSecret is the shared HMAC secret.
type Config struct {
	BaseURL        string
	ClientID       string
	ClientSecret   string
	PartnerID      string
	ChannelID      string
	PrivateKeyPath string
	PrivateKeyPEM  string
	HTTPClient     *http.Client
}

type Client struct {
	cfg        Config
	privateKey *rsa.PrivateKey
	httpClient *http.Client

	mu           sync.Mutex
	accessToken  string
	tokenExpires time.Time
}

// APIError wraps a non-success Nobu SNAP response.
type APIError struct {
	HTTPStatus      int
	ResponseCode    string
	ResponseMessage string
	RawResponse     json.RawMessage
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.ResponseCode == "" {
		return e.ResponseMessage
	}
	return e.ResponseCode + ": " + e.ResponseMessage
}

// NewClient validates config and loads the RSA signing key.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("nobu: base URL is required")
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("nobu: clientID and clientSecret are required")
	}
	if cfg.PartnerID == "" {
		cfg.PartnerID = cfg.ClientID
	}
	if cfg.ChannelID == "" {
		cfg.ChannelID = DefaultChannelID
	}
	key, err := loadPrivateKey(cfg.PrivateKeyPath, cfg.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{cfg: cfg, privateKey: key, httpClient: httpClient}, nil
}

type tokenResponse struct {
	ResponseCode    string `json:"responseCode"`
	ResponseMessage string `json:"responseMessage"`
	AccessToken     string `json:"accessToken"`
	TokenType       string `json:"tokenType"`
	ExpiresIn       string `json:"expiresIn"`
}

// GetAccessToken returns a cached token or mints a new one via SNAP B2B.
func (c *Client) GetAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpires) {
		return c.accessToken, nil
	}

	ts := formatTimestamp(time.Now())
	sig, err := signAsymmetric(c.cfg.ClientID, ts, c.privateKey)
	if err != nil {
		return "", err
	}

	body, _ := json.Marshal(map[string]string{"grantType": "client_credentials"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+TokenPath, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TIMESTAMP", ts)
	req.Header.Set("X-CLIENT-KEY", c.cfg.ClientID)
	req.Header.Set("X-SIGNATURE", sig)

	var tr tokenResponse
	if _, err := c.doRequest(req, &tr); err != nil {
		return "", err
	}

	expiresIn, _ := strconv.Atoi(tr.ExpiresIn)
	if expiresIn <= 0 {
		expiresIn = 900
	}
	c.accessToken = tr.AccessToken
	c.tokenExpires = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
	return c.accessToken, nil
}

// GenerateQRRequest asks Nobu to mint the static QR string for a provisioned
// merchant. The merchant identifiers come from Nobu's activation response.
type GenerateQRRequest struct {
	PartnerReferenceNo string
	MerchantID         string // subMerchantId (Nobu MID)
	StoreID            string // storeId (NMID)
	TerminalID         string // terminalId (TID)
	MerchantName       string
}

type GenerateQRResponse struct {
	ResponseCode       string          `json:"responseCode"`
	ResponseMessage    string          `json:"responseMessage"`
	ReferenceNo        string          `json:"referenceNo,omitempty"`
	PartnerReferenceNo string          `json:"partnerReferenceNo,omitempty"`
	QRContent          string          `json:"qrContent,omitempty"`
	TerminalID         string          `json:"terminalId,omitempty"`
	AdditionalInfo     map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse        json.RawMessage `json:"-"`
}

// GenerateStaticQR calls qr-mpm-generate to obtain a static QR string. A static
// QR carries no amount; additionalInfo.type="statis" requests the static form.
func (c *Client) GenerateStaticQR(ctx context.Context, req GenerateQRRequest) (*GenerateQRResponse, error) {
	body := map[string]any{
		"partnerReferenceNo": req.PartnerReferenceNo,
		"amount": map[string]string{
			"value":    "0.00",
			"currency": "IDR",
		},
	}
	if req.MerchantID != "" {
		body["merchantId"] = req.MerchantID
	}
	if req.StoreID != "" {
		body["storeId"] = req.StoreID
	}
	if req.TerminalID != "" {
		body["terminalId"] = req.TerminalID
	}
	info := map[string]any{"type": "statis"}
	if req.MerchantName != "" {
		info["merchantName"] = req.MerchantName
	}
	body["additionalInfo"] = info

	var resp GenerateQRResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, GenerateQRPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	if resp.QRContent == "" {
		if v, ok := resp.AdditionalInfo["qrContent"].(string); ok {
			resp.QRContent = v
		} else if v, ok := resp.AdditionalInfo["paymentQrString"].(string); ok {
			resp.QRContent = v
		}
	}
	return &resp, nil
}

func (c *Client) doSNAPRequest(ctx context.Context, method, path string, body any, out any) (json.RawMessage, error) {
	token, err := c.GetAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	ts := formatTimestamp(time.Now())
	sig := signSymmetric(method, path, token, bodyBytes, ts, c.cfg.ClientSecret)
	externalID := strconv.FormatInt(time.Now().UnixNano(), 10)

	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TIMESTAMP", ts)
	req.Header.Set("X-SIGNATURE", sig)
	req.Header.Set("X-PARTNER-ID", c.cfg.PartnerID)
	req.Header.Set("X-EXTERNAL-ID", externalID)
	req.Header.Set("CHANNEL-ID", c.cfg.ChannelID)

	return c.doRequest(req, out)
}

func (c *Client) doRequest(req *http.Request, out any) (json.RawMessage, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return raw, fmt.Errorf("nobu: decode response: %w", err)
		}
	}

	code, msg := extractResponseStatus(raw)
	if resp.StatusCode >= http.StatusBadRequest || !isSuccessCode(code) {
		log.Debug().
			Str("url", req.URL.String()).
			Int("httpStatus", resp.StatusCode).
			Str("responseCode", code).
			Str("responseMessage", msg).
			RawJSON("raw", raw).
			Msg("nobu: provider error response")
		return raw, &APIError{
			HTTPStatus:      resp.StatusCode,
			ResponseCode:    code,
			ResponseMessage: msg,
			RawResponse:     raw,
		}
	}
	return raw, nil
}

func extractResponseStatus(raw json.RawMessage) (string, string) {
	var s struct {
		ResponseCode    string `json:"responseCode"`
		ResponseMessage string `json:"responseMessage"`
	}
	_ = json.Unmarshal(raw, &s)
	return s.ResponseCode, s.ResponseMessage
}

func isSuccessCode(code string) bool {
	return strings.HasPrefix(code, "200")
}

// signAsymmetric returns Base64(SHA256withRSA(clientKey + "|" + timestamp)).
func signAsymmetric(clientKey, timestamp string, key *rsa.PrivateKey) (string, error) {
	payload := clientKey + "|" + timestamp
	hashed := sha256.Sum256([]byte(payload))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hashed[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// signSymmetric returns Base64(HMAC-SHA512(<method>:<path>:<token>:<bodyHashHex>:<timestamp>)).
func signSymmetric(method, path, accessToken string, body []byte, timestamp, clientSecret string) string {
	bodyHash := sha256.Sum256(minifyJSON(body))
	stringToSign := strings.Join([]string{
		method,
		path,
		accessToken,
		strings.ToLower(hex.EncodeToString(bodyHash[:])),
		timestamp,
	}, ":")
	mac := hmac.New(sha512.New, []byte(clientSecret))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func minifyJSON(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var out strings.Builder
	inString := false
	escape := false
	for _, b := range body {
		if escape {
			out.WriteByte(b)
			escape = false
			continue
		}
		if b == '\\' {
			out.WriteByte(b)
			escape = true
			continue
		}
		if b == '"' {
			inString = !inString
			out.WriteByte(b)
			continue
		}
		if !inString && (b == ' ' || b == '\n' || b == '\r' || b == '\t') {
			continue
		}
		out.WriteByte(b)
	}
	return []byte(out.String())
}

func formatTimestamp(t time.Time) string {
	wib := time.FixedZone("WIB", 7*3600)
	return t.In(wib).Format("2006-01-02T15:04:05+07:00")
}

func loadPrivateKey(path, pemData string) (*rsa.PrivateKey, error) {
	var raw []byte
	if pemData != "" {
		raw = []byte(pemData)
	} else if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		raw = b
	} else {
		return nil, fmt.Errorf("nobu: private key path or PEM content required")
	}

	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("nobu: failed to decode PEM block")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	anyKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("nobu: parse private key: %w", err)
	}
	k, ok := anyKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("nobu: private key is not RSA")
	}
	return k, nil
}
