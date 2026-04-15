package service

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/GTDGit/gtd_api/pkg/alterra"
)

func TestAlterraResponseCodePrefersBusinessCode(t *testing.T) {
	t.Parallel()

	resp := &alterra.TransactionResponse{
		ResponseCode: "99",
		Error: &alterra.ErrorDetail{
			Code:    "406",
			Message: "Invalid parameter",
		},
		HTTPStatus: http.StatusNotAcceptable,
	}

	if got := alterraResponseCode(resp); got != "99" {
		t.Fatalf("alterraResponseCode() = %q, want %q", got, "99")
	}
}

func TestAlterraResponseCodeFallsBackToErrorCode(t *testing.T) {
	t.Parallel()

	resp := &alterra.TransactionResponse{
		Error: &alterra.ErrorDetail{
			Code:    "403",
			Message: "Product unavailable",
		},
		HTTPStatus: http.StatusForbidden,
	}

	if got := alterraResponseCode(resp); got != "403" {
		t.Fatalf("alterraResponseCode() = %q, want %q", got, "403")
	}
}

func TestAlterraResponseMessageFallsBackToRCDescription(t *testing.T) {
	t.Parallel()

	resp := &alterra.TransactionResponse{
		ResponseCode: "99",
	}

	if got := alterraResponseMessage(resp, alterraResponseCode(resp)); got != "General Error" {
		t.Fatalf("alterraResponseMessage() = %q, want %q", got, "General Error")
	}
}

func TestSanitizeAlterraExtraStripsInternalPricingFields(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"reference_no":   "REF-123",
		"payment_period": "02",
		"admin":          2500,
		"commission":     500,
	}

	got := sanitizeAlterraExtra(input)
	want := map[string]any{
		"reference_no":   "REF-123",
		"payment_period": "02",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sanitizeAlterraExtra() = %#v, want %#v", got, want)
	}
}
