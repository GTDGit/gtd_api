package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/kiosbank"
)

func TestParseKiosbankDataBPJSInquiry(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"idPelanggan":"8888800020839792",
		"nama":"NURHASANAH",
		"premi":"25500",
		"adminBank":"2500",
		"total":"79000"
	}`)

	parsed := parseKiosbankData(raw)
	if parsed.CustomerName != "NURHASANAH" {
		t.Fatalf("CustomerName = %q", parsed.CustomerName)
	}
	if parsed.Admin != 2500 {
		t.Fatalf("Admin = %d, want %d", parsed.Admin, 2500)
	}
	if parsed.Amount != 76500 {
		t.Fatalf("Amount = %d, want %d", parsed.Amount, 76500)
	}
}

func TestConvertKiosbankPaymentResponseFallsBackToRequestedAmount(t *testing.T) {
	t.Parallel()

	provider := &KiosbankProviderClient{}
	resp := &kiosbank.PaymentResponse{
		BaseResponse: kiosbank.BaseResponse{RC: kiosbank.RCSuccess},
		ProductID:    "100302",
		Data: json.RawMessage(`{
			"AB":"000000003000",
			"TT":"000000003000",
			"TK":"66806385904760518886",
			"NM":"JAMALUDDIN"
		}`),
	}

	converted := provider.convertPaymentResponse(resp, "123456789012", 20000, 3000, time.Second)
	if converted.Amount != 20000 {
		t.Fatalf("Amount = %d, want %d", converted.Amount, 20000)
	}
	if converted.Admin != 3000 {
		t.Fatalf("Admin = %d, want %d", converted.Admin, 3000)
	}
	if converted.SerialNumber != "66806385904760518886" {
		t.Fatalf("SerialNumber = %q", converted.SerialNumber)
	}
	if converted.ProviderRefID != "123456789012" {
		t.Fatalf("ProviderRefID = %q", converted.ProviderRefID)
	}
}

func TestBuildKiosbankCheckStatusInputUsesLoggedRequestData(t *testing.T) {
	t.Parallel()

	logRequest := json.RawMessage(`{
		"provider":"kiosbank",
		"ref_id":"760864752227",
		"amount":149000,
		"admin":0,
		"extra":{
			"noHanphone":"08123",
			"nama":"(2101TN) Spotv Timnas",
			"kode":"2101TN"
		}
	}`)

	trx := &models.Transaction{
		TransactionID: "GRB-20260413-000001",
		ProviderRefID: func() *string { s := "760864752227"; return &s }(),
		CreatedAt:     time.Now(),
	}
	logs := []models.TransactionLog{{Request: logRequest}}

	input := buildKiosbankCheckStatusInput(trx, logs)
	if input.ReferenceID != "760864752227" {
		t.Fatalf("ReferenceID = %q", input.ReferenceID)
	}
	if input.Tagihan != 149000 || input.Admin != 0 || input.Total != 149000 {
		t.Fatalf("unexpected amounts: %#v", input)
	}
	if input.NoHandphone != "08123" || input.Nama != "(2101TN) Spotv Timnas" || input.Kode != "2101TN" {
		t.Fatalf("unexpected extra fields: %#v", input)
	}
}
