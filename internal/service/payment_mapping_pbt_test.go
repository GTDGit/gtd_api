package service

import (
	"encoding/json"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// ----------------------------------------------------------------------------
// Mapping property suite (payment-provider-enhancements).
//
// These tests cover the pure wire-only mapping helpers:
//   - pakailinkEmoneyProductCode (pakailink_provider_client.go)  -> Property 1
//   - PaymentDetailNormalized serialization (payment_provider.go) -> Property 8
//   - xenditEwalletChannelCode (xendit_provider_client.go)        -> Property 12
//
// They reuse the shared generators from payment_pbt_test.go where relevant.
// New helpers here are prefixed `mp` to avoid collisions with existing
// package-level test identifiers. rapid runs >= 100 iterations per property by
// default (override with -rapid.checks).
// ----------------------------------------------------------------------------

// mpRandomCase returns s with each ASCII letter independently upper/lower-cased
// according to random draws, exercising the case-insensitivity of the mappings.
func mpRandomCase(t *rapid.T, s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range s {
		if rapid.Bool().Draw(t, "upper") {
			b.WriteString(strings.ToUpper(string(r)))
		} else {
			b.WriteString(strings.ToLower(string(r)))
		}
		_ = i
	}
	return b.String()
}

// ----------------------------------------------------------------------------
// Property 1: Pakailink product code mapping preserves the canonical code (Task 8.2)
// ----------------------------------------------------------------------------

// Feature: payment-provider-enhancements, Property 1: For any valid e-wallet code in
// {DANA, GOPAY, LINKAJA, OVO, SHOPEEPAY} (in any case, with or without the PAY prefix),
// mapping to the Pakailink product code yields exactly {PAYDANA, PAYGOPAY, PAYLINKAJA,
// PAYOVO, PAYSHOPEE} respectively, with no error.
//
// Validates: Requirements 1.1, 1.2, 1.3, 1.4, 1.5, 1.6
func TestProperty1_PakailinkProductCodeMapping(t *testing.T) {
	// Canonical plain code -> expected Pakailink product code.
	want := map[string]string{
		"DANA":      "PAYDANA",
		"GOPAY":     "PAYGOPAY",
		"LINKAJA":   "PAYLINKAJA",
		"OVO":       "PAYOVO",
		"SHOPEEPAY": "PAYSHOPEE",
	}
	plainCodes := make([]string, 0, len(want))
	for k := range want {
		plainCodes = append(plainCodes, k)
	}

	rapid.Check(t, func(t *rapid.T) {
		plain := rapid.SampledFrom(plainCodes).Draw(t, "plainCode")

		// Optionally use the PAY* product-code form; the function accepts both
		// the plain canonical code and its PAY* product code (e.g. SHOPEEPAY or
		// PAYSHOPEE), so the prefixed form is the product code itself.
		input := plain
		if rapid.Bool().Draw(t, "withPayPrefix") {
			input = want[plain]
		}
		// Randomize letter case across the whole string.
		input = mpRandomCase(t, input)
		// Optionally pad with surrounding whitespace (TrimSpace is applied).
		if rapid.Bool().Draw(t, "padded") {
			input = "  " + input + "\t"
		}

		got, err := pakailinkEmoneyProductCode(input)
		if err != nil {
			t.Fatalf("pakailinkEmoneyProductCode(%q) returned error: %v", input, err)
		}
		if got != want[plain] {
			t.Fatalf("pakailinkEmoneyProductCode(%q) = %q, want %q", input, got, want[plain])
		}
		// The product code is always one of the PAY* forms (never the plain code).
		if !strings.HasPrefix(got, "PAY") {
			t.Fatalf("product code %q is not PAY-prefixed", got)
		}
	})
}

// ----------------------------------------------------------------------------
// Property 8: paymentDetail field set is uniform per payment type (Task 9.3)
// ----------------------------------------------------------------------------

// mpAllDetailKeys is the full set of JSON keys across every type group. Any key
// not in the type's own group must be absent from the serialized output.
var mpAllDetailKeys = []string{
	"bankCode", "bankName", "vaNumber", "accountName", // VA
	"checkoutUrl", "mobileWebUrl", "deeplink", "qrCodeUrl", // EWALLET
	"qrString", "qrImageUrl", // QRIS
	"retailName", "paymentCode", // RETAIL
}

// mpDetailGroups maps a payment type to the JSON keys it is allowed to emit.
var mpDetailGroups = map[string][]string{
	"VA":      {"bankCode", "bankName", "vaNumber", "accountName"},
	"EWALLET": {"checkoutUrl", "mobileWebUrl", "deeplink", "qrCodeUrl"},
	"QRIS":    {"qrString", "qrImageUrl"},
	"RETAIL":  {"retailName", "paymentCode"},
}

// mpNonEmpty draws a random non-empty ASCII string for a populated field.
func mpNonEmpty(t *rapid.T, label string) string {
	return rapid.StringMatching(`[A-Za-z0-9]{1,16}`).Draw(t, label)
}

// Feature: payment-provider-enhancements, Property 8: For any provider response
// normalized for a given payment type, the serialized paymentDetail contains only fields
// from that type's allowed set (VA, EWALLET, QRIS, or RETAIL group), with irrelevant
// fields omitted (omitempty) and shared field names unchanged across providers.
//
// Validates: Requirements 4.5, 4.8
func TestProperty8_PaymentDetailUniformPerType(t *testing.T) {
	groupKeys := make([]string, 0, len(mpDetailGroups))
	for k := range mpDetailGroups {
		groupKeys = append(groupKeys, k)
	}

	rapid.Check(t, func(t *rapid.T) {
		typeKey := rapid.SampledFrom(groupKeys).Draw(t, "type")
		allowed := mpDetailGroups[typeKey]
		allowedSet := map[string]bool{}
		for _, k := range allowed {
			allowedSet[k] = true
		}

		// Build a normalized detail with ONLY this type's group populated, each
		// field to a random non-empty value.
		var norm PaymentDetailNormalized
		switch typeKey {
		case "VA":
			norm.BankCode = mpNonEmpty(t, "bankCode")
			norm.BankName = mpNonEmpty(t, "bankName")
			norm.VANumber = mpNonEmpty(t, "vaNumber")
			norm.AccountName = mpNonEmpty(t, "accountName")
		case "EWALLET":
			norm.CheckoutURL = mpNonEmpty(t, "checkoutUrl")
			norm.MobileWebURL = mpNonEmpty(t, "mobileWebUrl")
			norm.Deeplink = mpNonEmpty(t, "deeplink")
			norm.QRCodeURL = mpNonEmpty(t, "qrCodeUrl")
		case "QRIS":
			norm.QRString = mpNonEmpty(t, "qrString")
			norm.QRImageURL = mpNonEmpty(t, "qrImageUrl")
		case "RETAIL":
			norm.RetailName = mpNonEmpty(t, "retailName")
			norm.PaymentCode = mpNonEmpty(t, "paymentCode")
		}

		raw, err := json.Marshal(norm)
		if err != nil {
			t.Fatalf("marshal paymentDetail: %v", err)
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatalf("unmarshal paymentDetail: %v", err)
		}

		// Every key present must belong to this type's allowed group.
		for k := range obj {
			if !allowedSet[k] {
				t.Fatalf("type %s emitted disallowed key %q: %s", typeKey, k, raw)
			}
		}

		// No key from a different group may appear (subset assertion).
		for _, k := range mpAllDetailKeys {
			if allowedSet[k] {
				continue
			}
			if _, found := obj[k]; found {
				t.Fatalf("type %s leaked foreign key %q: %s", typeKey, k, raw)
			}
		}

		// Since every group field was set to a non-empty value, the populated
		// set must equal the allowed group exactly (no omitempty drops).
		if len(obj) != len(allowed) {
			t.Fatalf("type %s emitted %d keys, want %d: %s", typeKey, len(obj), len(allowed), raw)
		}
	})
}

// ----------------------------------------------------------------------------
// Property 12: Xendit e-wallet channel mapping (Task 9.4)
// ----------------------------------------------------------------------------

// Feature: payment-provider-enhancements, Property 12: For any e-wallet code supported by
// Xendit (OVO, SHOPEEPAY, LINKAJA, ASTRAPAY), the channel-code mapping returns the
// matching Xendit channel, and returns empty (forcing fallback) for codes Xendit does not
// support (e.g. GOPAY).
//
// Validates: Requirements 11.5
func TestProperty12_XenditEwalletChannelMapping(t *testing.T) {
	// Supported plain code -> expected Xendit channel.
	supported := map[string]string{
		"OVO":       "OVO",
		"SHOPEEPAY": "SHOPEEPAY",
		"LINKAJA":   "LINKAJA",
		"ASTRAPAY":  "ASTRAPAY",
	}
	supportedCodes := make([]string, 0, len(supported))
	for k := range supported {
		supportedCodes = append(supportedCodes, k)
	}

	// Codes Xendit does not support must map to "" (forces provider fallback).
	// GOPAY is the named example; the rest are arbitrary unknowns.
	unsupported := []string{"GOPAY", "DANA", "PAYGOPAY", "QRIS", "BCA", "FOO", "123"}

	rapid.Check(t, func(t *rapid.T) {
		if rapid.Bool().Draw(t, "useSupported") {
			plain := rapid.SampledFrom(supportedCodes).Draw(t, "supportedCode")
			input := plain
			if rapid.Bool().Draw(t, "withPayPrefix") {
				input = "PAY" + plain
			}
			input = mpRandomCase(t, input)
			if rapid.Bool().Draw(t, "padded") {
				input = " " + input + "  "
			}
			got := xenditEwalletChannelCode(input)
			if got != supported[plain] {
				t.Fatalf("xenditEwalletChannelCode(%q) = %q, want %q", input, got, supported[plain])
			}
			if got == "" {
				t.Fatalf("supported code %q mapped to empty channel", input)
			}
		} else {
			// Draw a random unknown string that is NOT a supported code (after
			// normalization and PAY-prefix stripping).
			var input string
			if rapid.Bool().Draw(t, "fromList") {
				input = rapid.SampledFrom(unsupported).Draw(t, "unsupportedCode")
			} else {
				input = rapid.StringMatching(`[A-Za-z0-9]{1,12}`).Draw(t, "randomCode")
			}
			input = mpRandomCase(t, input)

			// Skip if the random string happens to be a supported code/PAY form.
			norm := strings.ToUpper(strings.TrimSpace(input))
			if mpIsXenditSupported(norm) {
				return
			}

			got := xenditEwalletChannelCode(input)
			if got != "" {
				t.Fatalf("xenditEwalletChannelCode(%q) = %q, want \"\" (fallback)", input, got)
			}
		}
	})
}

// mpIsXenditSupported reports whether an already-upper-cased/trimmed code maps
// to a non-empty Xendit channel, mirroring the cases in xenditEwalletChannelCode.
func mpIsXenditSupported(norm string) bool {
	switch norm {
	case "OVO", "PAYOVO",
		"SHOPEEPAY", "PAYSHOPEE", "PAYSHOPEEPAY",
		"LINKAJA", "PAYLINKAJA",
		"ASTRAPAY", "PAYASTRAPAY":
		return true
	default:
		return false
	}
}
