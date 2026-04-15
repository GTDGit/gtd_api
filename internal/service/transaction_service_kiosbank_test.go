package service

import (
	"testing"

	"github.com/GTDGit/gtd_api/internal/models"
)

func TestBuildProviderLogRequestIncludesKiosbankWireRequest(t *testing.T) {
	t.Parallel()

	req := &ProviderRequest{
		RefID:      "760864752227",
		SKUCode:    "550031",
		CustomerNo: "08123456789",
		Amount:     149000,
		Type:       ProviderTrxPayment,
		Extra: map[string]any{
			"admin":       2500,
			"noHandphone": "08123",
			"nama":        "(2101TN) Spotv Timnas",
			"kode":        "2101TN",
		},
	}
	opt := &models.ProviderOption{
		ProviderCode:    models.ProviderKiosbank,
		ProviderSKUCode: "550031",
		Admin:           2500,
	}

	logRequest := buildProviderLogRequest(opt, req)
	wireRequest, ok := logRequest["wire_request"].(map[string]any)
	if !ok {
		t.Fatalf("wire_request missing: %#v", logRequest)
	}
	if wireRequest["referenceID"] != "760864752227" {
		t.Fatalf("referenceID = %#v", wireRequest["referenceID"])
	}
	if wireRequest["tagihan"] != 149000 {
		t.Fatalf("tagihan = %#v", wireRequest["tagihan"])
	}
	if wireRequest["admin"] != 2500 {
		t.Fatalf("admin = %#v", wireRequest["admin"])
	}
	if wireRequest["total"] != 151500 {
		t.Fatalf("total = %#v", wireRequest["total"])
	}
	if wireRequest["noHanphone"] != "08123" {
		t.Fatalf("noHanphone = %#v", wireRequest["noHanphone"])
	}
}
