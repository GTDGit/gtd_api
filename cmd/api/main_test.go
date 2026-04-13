package main

import (
	"testing"

	"github.com/GTDGit/gtd_api/internal/config"
)

func TestKiosbankClientConfigUsesDevelopmentOverrides(t *testing.T) {
	t.Parallel()

	cfg := config.KiosbankConfig{
		BaseURL:        "https://prod.example",
		MerchantID:     "PROD",
		CounterID:      "1",
		AccountID:      "A",
		Mitra:          "DJI",
		Username:       "prod-user",
		Password:       "prod-pass",
		DevelopmentURL: "https://dev.example",
		DevelopmentCreds: config.KiosbankCredentialConfig{
			MerchantID: "DEV",
			CounterID:  "2",
			AccountID:  "B",
			Mitra:      "KB",
			Username:   "dev-user",
			Password:   "dev-pass",
		},
	}

	prod := kiosbankClientConfig(cfg, false)
	dev := kiosbankClientConfig(cfg, true)

	if prod.BaseURL != "https://prod.example" || prod.Username != "prod-user" {
		t.Fatalf("unexpected prod config: %#v", prod)
	}
	if dev.BaseURL != "https://dev.example" || dev.Username != "dev-user" || dev.MerchantID != "DEV" {
		t.Fatalf("unexpected dev config: %#v", dev)
	}
}

func TestKiosbankClientConfigFallsBackToProductionCredentials(t *testing.T) {
	t.Parallel()

	cfg := config.KiosbankConfig{
		BaseURL:        "https://prod.example",
		DevelopmentURL: "https://dev.example",
		MerchantID:     "PROD",
		CounterID:      "1",
		AccountID:      "A",
		Mitra:          "DJI",
		Username:       "prod-user",
		Password:       "prod-pass",
	}

	dev := kiosbankClientConfig(cfg, true)
	if dev.Username != "prod-user" || dev.Password != "prod-pass" || dev.MerchantID != "PROD" {
		t.Fatalf("unexpected fallback dev config: %#v", dev)
	}
}
