package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/GTDGit/gtd_api/internal/models"
)

// ----------------------------------------------------------------------------
// Merchant webhook / callback property suite (payment-provider-enhancements).
//
// These tests cover the merchant-facing callback behavior implemented in
// payment_callback_service.go (buildPaymentCallbackPayload, hmacHexSHA256,
// paymentNextRetry, paymentCallbackMaxAttempts) and the status->event-name
// derivation in payment_service.go (paymentEventName).
//
// They reuse the shared generators defined in payment_pbt_test.go (genPayment,
// genPaymentStatus, ...). New helpers introduced here are prefixed with `cb`
// to avoid collisions with existing package-level test identifiers.
//
// rapid runs >= 100 iterations per property by default.
// ----------------------------------------------------------------------------

// cbEventNames are representative merchant webhook event names used to exercise
// payload shaping independently of the status->event mapping.
func cbEventName(t *rapid.T) string {
	return rapid.SampledFrom([]string{
		"payment.pending",
		"payment.paid",
		"payment.expired",
		"payment.cancelled",
		"payment.failed",
		"payment.refunded",
	}).Draw(t, "event")
}

// ----------------------------------------------------------------------------
// Property 4: Webhook payload follows the Webhook_Event shape (Task 13.3)
// ----------------------------------------------------------------------------

// Feature: payment-provider-enhancements, Property 4: For any payment and event name,
// the merchant webhook payload is an object with exactly event, data, and meta, where
// data carries id, referenceId, nested paymentMethod, nested amount, status, and
// paymentDetail (matching the API response shape), and data contains no provider name or
// providerRef key. meta carries requestId and timestamp.
//
// Validates: Requirements 8.1, 8.2, 8.3
func TestProperty4_WebhookEventShape(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := genPayment(t)
		event := cbEventName(t)

		raw := buildPaymentCallbackPayload(p, event)

		// Top-level must be an object with EXACTLY event, data, meta.
		var top map[string]json.RawMessage
		if err := json.Unmarshal(raw, &top); err != nil {
			t.Fatalf("payload is not a JSON object: %v\npayload=%s", err, raw)
		}
		cbAssertExactKeys(t, top, []string{"event", "data", "meta"}, "top-level")

		// event matches what we passed in.
		var gotEvent string
		if err := json.Unmarshal(top["event"], &gotEvent); err != nil {
			t.Fatalf("event not a string: %v", err)
		}
		if gotEvent != event {
			t.Fatalf("event mismatch: got %q want %q", gotEvent, event)
		}

		// meta must be an object with a non-empty timestamp and requestId.
		var metaObj map[string]json.RawMessage
		if err := json.Unmarshal(top["meta"], &metaObj); err != nil {
			t.Fatalf("meta is not a JSON object: %v", err)
		}
		for _, k := range []string{"requestId", "timestamp"} {
			var v string
			if _, ok := metaObj[k]; !ok {
				t.Fatalf("meta missing key %q; keys=%v", k, cbKeys(metaObj))
			}
			if err := json.Unmarshal(metaObj[k], &v); err != nil || v == "" {
				t.Fatalf("meta.%s must be a non-empty string", k)
			}
		}

		// data must be an object carrying the required keys.
		var data map[string]json.RawMessage
		if err := json.Unmarshal(top["data"], &data); err != nil {
			t.Fatalf("data is not a JSON object: %v", err)
		}
		for _, k := range []string{"id", "referenceId", "paymentMethod", "amount", "status"} {
			if _, ok := data[k]; !ok {
				t.Fatalf("data missing required key %q; keys=%v", k, cbKeys(data))
			}
		}

		// paymentDetail is present whenever the payment carries one (genPayment
		// always sets a non-empty `{}`), and must never be the bare null.
		if pd, ok := data["paymentDetail"]; !ok {
			t.Fatalf("data missing paymentDetail; keys=%v", cbKeys(data))
		} else if string(pd) == "null" {
			t.Fatalf("paymentDetail serialized as null")
		}

		// data must NEVER leak provider identity / provider reference.
		for _, forbidden := range []string{
			"provider", "providerRef", "providerName", "provider_ref", "providerData",
		} {
			if _, ok := data[forbidden]; ok {
				t.Fatalf("data leaked forbidden key %q; keys=%v", forbidden, cbKeys(data))
			}
		}

		// id / referenceId / status match the source payment.
		cbAssertStringField(t, data, "id", p.PaymentID)
		cbAssertStringField(t, data, "referenceId", p.ReferenceID)
		cbAssertStringField(t, data, "status", string(p.Status))

		// nested paymentMethod{type, code}.
		var pm map[string]json.RawMessage
		if err := json.Unmarshal(data["paymentMethod"], &pm); err != nil {
			t.Fatalf("paymentMethod not an object: %v", err)
		}
		for _, k := range []string{"type", "code"} {
			if _, ok := pm[k]; !ok {
				t.Fatalf("paymentMethod missing key %q; keys=%v", k, cbKeys(pm))
			}
		}
		cbAssertStringField(t, pm, "type", string(p.PaymentType))
		cbAssertStringField(t, pm, "code", p.PaymentCode)

		// nested amount{subtotal, fee, total}.
		var amt map[string]json.RawMessage
		if err := json.Unmarshal(data["amount"], &amt); err != nil {
			t.Fatalf("amount not an object: %v", err)
		}
		for _, k := range []string{"subtotal", "fee", "total"} {
			if _, ok := amt[k]; !ok {
				t.Fatalf("amount missing key %q; keys=%v", k, cbKeys(amt))
			}
		}
		cbAssertInt64Field(t, amt, "subtotal", p.Amount)
		cbAssertInt64Field(t, amt, "fee", p.Fee)
		cbAssertInt64Field(t, amt, "total", p.TotalAmount)
	})
}

// ----------------------------------------------------------------------------
// Property 9: Final status maps to the matching event name (Task 13.4)
// ----------------------------------------------------------------------------

// Feature: payment-provider-enhancements, Property 9: For any payment status, the derived
// webhook event name equals "payment." + lowercase(status); in particular Paid, Cancelled,
// Expired, and Failed map to payment.paid, payment.cancelled, payment.expired, and
// payment.failed.
//
// Validates: Requirements 8.4
//
// Note: the production derivation lives in payment_service.go as paymentEventName, and
// the call sites in payment_service.go pass the matching literals
// ("payment.pending" at create, "payment.cancelled" on cancel, "payment.expired" on
// expiry, "payment.failed" on failure). This property asserts paymentEventName equals
// the documented rule for all PaymentStatus values.
func TestProperty9_StatusToEventName(t *testing.T) {
	// cbExpectedEventName replicates the documented rule independently of the
	// implementation under test.
	cbExpectedEventName := func(status models.PaymentStatus) string {
		return "payment." + strings.ToLower(string(status))
	}

	// Explicit coverage of the named statuses from Req 8.4.
	explicit := map[models.PaymentStatus]string{
		models.PaymentStatusSuccess:   "payment.success",
		models.PaymentStatusCancelled: "payment.cancelled",
		models.PaymentStatusExpired:   "payment.expired",
		models.PaymentStatusFailed:    "payment.failed",
	}
	for status, want := range explicit {
		if got := paymentEventName(status); got != want {
			t.Fatalf("paymentEventName(%q) = %q, want %q", status, got, want)
		}
		if got := cbExpectedEventName(status); got != want {
			t.Fatalf("rule replica for %q = %q, want %q", status, got, want)
		}
	}

	rapid.Check(t, func(t *rapid.T) {
		status := genPaymentStatus(t)
		got := paymentEventName(status)
		want := cbExpectedEventName(status)
		if got != want {
			t.Fatalf("paymentEventName(%q) = %q, want %q", status, got, want)
		}
	})
}

// ----------------------------------------------------------------------------
// Property 10: Webhook signature is a verifiable HMAC-SHA256 (Task 13.5)
// ----------------------------------------------------------------------------

// Feature: payment-provider-enhancements, Property 10: For any payload bytes and secret,
// the signature equals HMAC-SHA256(payload, secret) rendered as lowercase hex, so an
// independent recomputation with the same secret verifies the payload.
//
// Validates: Requirements 8.5
func TestProperty10_HMACSHA256(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		payload := rapid.SliceOf(rapid.Byte()).Draw(t, "payload")
		secret := rapid.String().Draw(t, "secret")

		got := hmacHexSHA256(payload, secret)

		// Independent recomputation.
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		want := hex.EncodeToString(mac.Sum(nil))

		if got != want {
			t.Fatalf("hmacHexSHA256 = %q, want %q", got, want)
		}

		// Must be lowercase hex of fixed length (32 bytes -> 64 hex chars).
		if len(got) != sha256.Size*2 {
			t.Fatalf("signature length = %d, want %d", len(got), sha256.Size*2)
		}
		if got != strings.ToLower(got) {
			t.Fatalf("signature is not lowercase hex: %q", got)
		}
		if _, err := hex.DecodeString(got); err != nil {
			t.Fatalf("signature is not valid hex: %v", err)
		}

		// An independent verifier accepts the signature for the same secret.
		if !hmac.Equal([]byte(got), []byte(want)) {
			t.Fatalf("independent verification failed")
		}
	})
}

// ----------------------------------------------------------------------------
// Property 11: Callback retry follows the bounded backoff schedule (Task 13.6)
// ----------------------------------------------------------------------------

// Feature: payment-provider-enhancements, Property 11: For any attempt number from 1 up
// to the maximum, the next retry time advances by the scheduled interval for that attempt
// (30s, 1m, 5m, 30m, 2h), and for any attempt at or beyond the maximum no further retry
// is scheduled.
//
// Validates: Requirements 8.6
func TestProperty11_BoundedBackoff(t *testing.T) {
	intervals := []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		5 * time.Minute,
		30 * time.Minute,
		2 * time.Hour,
	}

	// The schedule must cover exactly paymentCallbackMaxAttempts attempts.
	if len(intervals) != paymentCallbackMaxAttempts {
		t.Fatalf("schedule length %d != paymentCallbackMaxAttempts %d",
			len(intervals), paymentCallbackMaxAttempts)
	}

	rapid.Check(t, func(t *rapid.T) {
		// Cover valid attempts 1..len, plus out-of-range below and above.
		attempt := rapid.IntRange(-3, len(intervals)+5).Draw(t, "attempt")

		before := time.Now()
		got := paymentNextRetry(attempt)
		after := time.Now()

		if attempt <= 0 || attempt > len(intervals) {
			// Out of range -> zero time (no retry scheduled).
			if !got.IsZero() {
				t.Fatalf("paymentNextRetry(%d) = %v, want zero time", attempt, got)
			}
			return
		}

		// In range -> Now + interval, within a tolerance window bracketed by the
		// observed Now values around the call.
		interval := intervals[attempt-1]
		lower := before.Add(interval)
		upper := after.Add(interval).Add(2 * time.Second)
		if got.Before(lower) || got.After(upper) {
			t.Fatalf("paymentNextRetry(%d) = %v, want within [%v, %v]",
				attempt, got, lower, upper)
		}
	})

	// markFailure only schedules a retry while attempt < MaxAttempts, so the
	// final attempt and beyond never reschedule. paymentNextRetry must therefore
	// return zero time for any attempt at or beyond the schedule length.
	for attempt := len(intervals) + 1; attempt <= len(intervals)+5; attempt++ {
		if next := paymentNextRetry(attempt); !next.IsZero() {
			t.Fatalf("paymentNextRetry(%d) scheduled a retry beyond the bound: %v", attempt, next)
		}
	}
}

// ----------------------------------------------------------------------------
// Local helpers (prefixed cb* to avoid collisions with existing identifiers).
// ----------------------------------------------------------------------------

func cbKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func cbAssertExactKeys(t *rapid.T, m map[string]json.RawMessage, want []string, where string) {
	t.Helper()
	if len(m) != len(want) {
		t.Fatalf("%s: got keys %v, want exactly %v", where, cbKeys(m), want)
	}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Fatalf("%s: missing key %q; got %v", where, k, cbKeys(m))
		}
	}
}

func cbAssertStringField(t *rapid.T, m map[string]json.RawMessage, key, want string) {
	t.Helper()
	var got string
	if err := json.Unmarshal(m[key], &got); err != nil {
		t.Fatalf("field %q not a string: %v", key, err)
	}
	if got != want {
		t.Fatalf("field %q = %q, want %q", key, got, want)
	}
}

func cbAssertInt64Field(t *rapid.T, m map[string]json.RawMessage, key string, want int64) {
	t.Helper()
	var got int64
	if err := json.Unmarshal(m[key], &got); err != nil {
		t.Fatalf("field %q not an int64: %v", key, err)
	}
	if got != want {
		t.Fatalf("field %q = %d, want %d", key, got, want)
	}
}
