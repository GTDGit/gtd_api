// Package filesportal is a thin client for the GTD file-delivery portal
// (dev-files.gtd.co.id). The portal accepts an open (unauthenticated) upload of
// one or more base64 files and returns token-gated, private delivery URLs. It is
// used to hand QRIS onboarding documents to Nobu: the returned bundle URL is
// embedded in the Excel batch so Nobu can fetch the merchant's KTP/selfie/etc.
package filesportal

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

// Client posts uploads to the portal's open upload endpoint.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a portal client. A nil httpClient gets a 30s-timeout default.
// An empty baseURL disables the client (Enabled() reports false).
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), httpClient: httpClient}
}

// Enabled reports whether a base URL is configured.
func (c *Client) Enabled() bool { return c != nil && c.baseURL != "" }

// UploadFile is one file in an upload request (base64, raw or data-URI).
type UploadFile struct {
	FileName   string `json:"fileName"`
	DataBase64 string `json:"dataBase64"`
}

// UploadRequest is the portal's POST /api/upload body.
type UploadRequest struct {
	Title      string       `json:"title,omitempty"`
	Note       string       `json:"note,omitempty"`
	DocName    string       `json:"docName,omitempty"`
	AccessMode string       `json:"accessMode,omitempty"` // open | once
	OnceNote   string       `json:"onceNote,omitempty"`
	Files      []UploadFile `json:"files"`
}

// UploadResponseFile mirrors one delivered file in the portal response.
type UploadResponseFile struct {
	Token    string `json:"token"`
	FileName string `json:"fileName"`
	ViewURL  string `json:"viewUrl"`
}

// UploadResponse is the portal's POST /api/upload response.
type UploadResponse struct {
	Token      string               `json:"token"`
	Title      string               `json:"title"`
	AccessMode string               `json:"accessMode"`
	BundleURL  string               `json:"bundleUrl"`
	Files      []UploadResponseFile `json:"files"`
}

// Upload posts the files to the portal and returns the bundle delivery info.
func (c *Client) Upload(ctx context.Context, req UploadRequest) (*UploadResponse, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("filesportal: client not configured")
	}
	if len(req.Files) == 0 {
		return nil, fmt.Errorf("filesportal: at least one file is required")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("filesportal: marshal request: %w", err)
	}

	url := c.baseURL + "/api/upload"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("filesportal: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("filesportal: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("filesportal: upload returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out UploadResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("filesportal: decode response: %w", err)
	}
	return &out, nil
}
