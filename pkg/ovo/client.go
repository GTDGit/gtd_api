// Package ovo provides a client for the OVO partner push-to-pay payment API.
//
// Flow (typical OVO partner integration):
//   - Push-to-pay: the merchant pushes a payment request to the customer's
//     OVO app by MSISDN/phone. OVO returns a PENDING transaction and the
//     customer approves in-app.
//   - Notification: OVO asynchronously POSTs a signed callback to the
//     merchant once the customer approves/declines the push.
//   - Status query: the merchant can poll transaction status for
//     reconciliation.
//
// NOTE: OVO does not publish a single canonical public API. Exact field names,
// header names, and the signing string layout MUST be confirmed against the
// OVO partner technical documentation before going live. Points that need
// confirmation are marked with `TODO(ovo-docs)`.
package ovo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the OVO partner API base URL. Override per environment.
	DefaultBaseURL = "https://api.ovo.id"

	// PushPaymentPath initiates a push-to-pay transaction.
	// TODO(ovo-docs): confirm exact path against OVO partner docs.
	PushPaymentPath = "/v1.0/payment/push"
	// StatusPath queries a transaction status.
	// TODO(ovo-docs): confirm exact path against OVO partner docs.
	StatusPath = "/v1.0/payment/status"
	// VoidPath cancels/voids a pending transaction.
	// TODO(ovo-docs): confirm exact path against OVO partner docs.
	VoidPath = "/v1.0/payment/void"
)

// Config holds OVO Direct partner credentials and routing.
type Config struct {
	BaseURL    string
	MerchantID string
	AppID      string
	// ClientSecret (a.k.a. shared secret / signature key) is used to sign
	// requests and verify inbound notification signatures (HMAC-SHA256).
	ClientSecret string
	// APIKey is sent as a static credential header when present.
	APIKey string

	HTTPClient *http.Client
}

// Client is the OVO Direct API client.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// NewClient creates a new OVO Client. It requires a merchant ID and a client
// secret so requests can be signed and notifications verified; without them the
// adapter must report itself unavailable so selection can fall back.
func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.MerchantID) == "" {
		return nil, fmt.Errorf("ovo: merchantID is required")
	}
	if strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, fmt.Errorf("ovo: clientSecret is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{cfg: cfg, httpClient: hc}, nil
}

// ClientSecret exposes the configured signing secret for inbound notification
// verification at the webhook layer.
func (c *Client) ClientSecret() string { return c.cfg.ClientSecret }

// PushPayment initiates a push-to-pay transaction against the customer's OVO
// app. OVO returns a PENDING transaction that the customer must approve.
func (c *Client) PushPayment(ctx context.Context, req PushPaymentRequest) (*PushPaymentResponse, error) {
	if req.MerchantID == "" {
		req.MerchantID = c.cfg.MerchantID
	}
	if req.AppID == "" {
		req.AppID = c.cfg.AppID
	}
	if req.Currency == "" {
		req.Currency = "IDR"
	}
	var resp PushPaymentResponse
	raw, err := c.do(ctx, http.MethodPost, PushPaymentPath, req, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// QueryStatus reconciles the status of a transaction by reference.
func (c *Client) QueryStatus(ctx context.Context, req StatusRequest) (*StatusResponse, error) {
	if req.MerchantID == "" {
		req.MerchantID = c.cfg.MerchantID
	}
	var resp StatusResponse
	raw, err := c.do(ctx, http.MethodPost, StatusPath, req, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// Void cancels a pending push-to-pay transaction.
func (c *Client) Void(ctx context.Context, req VoidRequest) (*VoidResponse, error) {
	if req.MerchantID == "" {
		req.MerchantID = c.cfg.MerchantID
	}
	var resp VoidResponse
	raw, err := c.do(ctx, http.MethodPost, VoidPath, req, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) (json.RawMessage, error) {
	var (
		reader  io.Reader
		payload []byte
	)
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = b
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Static credential headers.
	// TODO(ovo-docs): confirm exact header names with OVO partner docs.
	if c.cfg.AppID != "" {
		req.Header.Set("App-ID", c.cfg.AppID)
	}
	if c.cfg.APIKey != "" {
		req.Header.Set("Api-Key", c.cfg.APIKey)
	}
	req.Header.Set("Merchant-ID", c.cfg.MerchantID)
	req.Header.Set("X-Timestamp", timestamp)
	// Request signature: HMAC-SHA256 over METHOD:path:timestamp:body using the
	// shared client secret. TODO(ovo-docs): confirm signing string layout.
	req.Header.Set("X-Signature", SignRequest(method, path, payload, timestamp, c.cfg.ClientSecret))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		apiErr := &APIError{HTTPStatus: resp.StatusCode, RawResponse: raw}
		_ = json.Unmarshal(raw, apiErr)
		return raw, apiErr
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return raw, fmt.Errorf("ovo: decode response: %w", err)
		}
	}
	return raw, nil
}
