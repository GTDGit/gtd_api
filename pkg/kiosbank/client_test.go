package kiosbank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
)

func TestGenerateReferenceID(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{})
	for i := 0; i < 64; i++ {
		refID := GenerateReferenceID()
		if !IsNumericReferenceID(refID) {
			t.Fatalf("reference ID %q is not 12-digit numeric", refID)
		}
		if _, ok := seen[refID]; ok {
			t.Fatalf("duplicate reference ID generated: %q", refID)
		}
		seen[refID] = struct{}{}
	}
}

func TestGetPriceListPulsaRequestsAllPrefixes(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		prefixes []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="test", nonce="nonce", opaque="opaque", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/Services/SignOn":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rc":"00","sessionID":"SESSION"}`))
		case "/Services/getPulsa-Prabayar":
			var req PriceListRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			mu.Lock()
			prefixes = append(prefixes, req.PrefixID)
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rc":"00","record":[]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:    server.URL,
		MerchantID: "MERCHANT",
		CounterID:  "1",
		AccountID:  "ACC",
		Mitra:      "DJI",
		Username:   "user",
		Password:   "pass",
	})

	if _, err := client.GetPriceListPulsa(context.Background()); err != nil {
		t.Fatalf("GetPriceListPulsa() error = %v", err)
	}

	sort.Strings(prefixes)
	expected := []string{"11", "21", "31", "41", "51", "81"}
	if len(prefixes) != len(expected) {
		t.Fatalf("prefix count = %d, want %d (%v)", len(prefixes), len(expected), prefixes)
	}
	for i, prefix := range expected {
		if prefixes[i] != prefix {
			t.Fatalf("prefixes[%d] = %q, want %q", i, prefixes[i], prefix)
		}
	}
}
