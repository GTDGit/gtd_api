package service

import (
	"errors"
	"testing"

	"github.com/GTDGit/gtd_api/internal/models"
)

func TestNormalizeMethodKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inType    string
		inCode    string
		wantType  models.PaymentType
		wantCode  string
		wantErr   bool
		wantErrCd string
	}{
		{name: "lowercase type is upper-cased", inType: "va", inCode: "014", wantType: models.PaymentTypeVA, wantCode: "014"},
		{name: "ewallet trims code", inType: "EWALLET", inCode: "  OVO ", wantType: models.PaymentTypeEwallet, wantCode: "OVO"},
		{name: "qris cpm", inType: "qris", inCode: "CPM", wantType: models.PaymentTypeQRIS, wantCode: "CPM"},
		{name: "retail alfamart", inType: "Retail", inCode: "ALFAMART", wantType: models.PaymentTypeRetail, wantCode: "ALFAMART"},
		{name: "unknown type rejected", inType: "CRYPTO", inCode: "BTC", wantErr: true, wantErrCd: "INVALID_PARAM"},
		{name: "empty code rejected", inType: "VA", inCode: "   ", wantErr: true, wantErrCd: "MISSING_FIELD"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotType, gotCode, err := normalizeMethodKey(tt.inType, tt.inCode)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				var pe *PaymentServiceError
				if !errors.As(err, &pe) {
					t.Fatalf("expected *PaymentServiceError, got %T", err)
				}
				if pe.Code != tt.wantErrCd {
					t.Fatalf("expected error code %q, got %q", tt.wantErrCd, pe.Code)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotType != tt.wantType {
				t.Fatalf("type: want %q, got %q", tt.wantType, gotType)
			}
			if gotCode != tt.wantCode {
				t.Fatalf("code: want %q, got %q", tt.wantCode, gotCode)
			}
		})
	}
}
