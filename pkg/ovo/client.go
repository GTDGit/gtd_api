// Package ovo provides a placeholder OVO Direct integration.
// Full docs: https://www.ovo.id/partner-integration/payment-api/tech-doc
// Requires OVO partner credentials and RSA key exchange.
package ovo

import "fmt"

// Config holds OVO Direct partner credentials.
type Config struct {
	BaseURL        string
	MerchantID     string
	AppID          string
	PrivateKeyPath string
	PrivateKeyPEM  string
}

// Client is the OVO Direct API client (stub — not yet implemented).
type Client struct {
	cfg Config
}

// NewClient creates a new OVO Client stub.
func NewClient(cfg Config) (*Client, error) {
	if cfg.MerchantID == "" {
		return nil, fmt.Errorf("ovo: merchantID is required")
	}
	return &Client{cfg: cfg}, nil
}
