package service

import (
	"net/http"
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
