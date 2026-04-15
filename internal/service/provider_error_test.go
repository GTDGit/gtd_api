package service

import (
	"errors"
	"testing"

	"github.com/GTDGit/gtd_api/internal/models"
)

func TestProviderResponseFromErrorUsesAlterraHTTPCode(t *testing.T) {
	t.Parallel()

	resp := providerResponseFromError(string(models.ProviderAlterra), errors.New("http error: 403"))
	if resp == nil {
		t.Fatal("expected provider response")
	}
	if resp.RC != "403" {
		t.Fatalf("RC = %q, want %q", resp.RC, "403")
	}
	if resp.Message != "HTTP error 403" {
		t.Fatalf("Message = %q, want %q", resp.Message, "HTTP error 403")
	}
	if resp.Pending {
		t.Fatal("expected failed response, got pending")
	}
}

func TestProviderResponseFromErrorKeepsKiosbankTransportPending(t *testing.T) {
	t.Parallel()

	resp := providerResponseFromError(string(models.ProviderKiosbank), errors.New("request failed: tls handshake timeout"))
	if resp == nil {
		t.Fatal("expected provider response")
	}
	if !resp.Pending {
		t.Fatal("expected pending response")
	}
	if resp.Message != "No response from Kiosbank" {
		t.Fatalf("Message = %q, want %q", resp.Message, "No response from Kiosbank")
	}
}
