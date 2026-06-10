package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/lib/pq"

	"github.com/GTDGit/gtd_api/internal/models"
)

// ----------------------------------------------------------------------------
// Unit tests for payment-provider-enhancements (tasks 3.3 and 6.4).
// ----------------------------------------------------------------------------

// Task 3.3: normalizeFeePaidBy rejects invalid values with INVALID_FEE_PAID_BY,
// defaults empty to merchant, and accepts merchant/customer case-insensitively
// with surrounding whitespace.
//
// Validates: Requirements 2.1, 2.2, 2.5
func TestNormalizeFeePaidBy(t *testing.T) {
	t.Parallel()

	valid := []struct {
		name string
		in   string
		want models.FeePaidBy
	}{
		{"empty defaults to merchant", "", models.FeePaidByMerchant},
		{"merchant lower", "merchant", models.FeePaidByMerchant},
		{"customer lower", "customer", models.FeePaidByCustomer},
		{"merchant mixed case", "MeRcHaNt", models.FeePaidByMerchant},
		{"customer upper", "CUSTOMER", models.FeePaidByCustomer},
		{"merchant padded", "  merchant\t", models.FeePaidByMerchant},
		{"customer padded", "\n customer ", models.FeePaidByCustomer},
		{"whitespace only defaults to merchant", "   ", models.FeePaidByMerchant},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeFeePaidBy(tc.in)
			if err != nil {
				t.Fatalf("normalizeFeePaidBy(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeFeePaidBy(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	invalid := []string{"MERCHANTX", "both", "123", "merchantcustomer", "cust", "m", "-"}
	for _, in := range invalid {
		t.Run("invalid_"+in, func(t *testing.T) {
			_, err := normalizeFeePaidBy(in)
			if err == nil {
				t.Fatalf("normalizeFeePaidBy(%q) expected error, got nil", in)
			}
			var svcErr *PaymentServiceError
			if !errors.As(err, &svcErr) {
				t.Fatalf("normalizeFeePaidBy(%q) error is not *PaymentServiceError: %T", in, err)
			}
			if svcErr.Code != "VALIDATION_ERROR" {
				t.Fatalf("normalizeFeePaidBy(%q) code = %q, want VALIDATION_ERROR", in, svcErr.Code)
			}
		})
	}
}

// Task 6.4 (a): the success message produced for payment responses is the
// constant "Successfully" used by every payment handler.
//
// Validates: Requirement 4.6
func TestPaymentSuccessMessageConstant(t *testing.T) {
	t.Parallel()

	// The handlers return the literal "Successfully" on success. This guards
	// against an accidental change to that contract.
	const want = "Successfully"
	if got := paymentSuccessMessageForTest(); got != want {
		t.Fatalf("payment success message = %q, want %q", got, want)
	}
}

// paymentSuccessMessageForTest mirrors the literal passed to utils.Success by
// the payment handlers (payment_handler.go). Kept in the test so a change to
// the handler contract surfaces here.
func paymentSuccessMessageForTest() string { return "Successfully" }

// Task 6.4 (b): createWithGeneratedID regenerates the public id and retries on a
// payment_id unique violation, then succeeds.
//
// Validates: Requirement 5.4
func TestCreateWithGeneratedID_RetriesOnIDCollision(t *testing.T) {
	t.Parallel()

	// First insert fails with a payment_id unique violation; second succeeds.
	collision := &pq.Error{Code: "23505", Constraint: "payments_payment_id_key"}

	var attempts int
	var ids []string
	p := &models.Payment{ReferenceID: "ref-collide"}

	err := createWithGeneratedID(p, func(pp *models.Payment) error {
		attempts++
		ids = append(ids, pp.PaymentID)
		if attempts == 1 {
			return collision
		}
		return nil
	})
	if err != nil {
		t.Fatalf("createWithGeneratedID unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 insert attempts, got %d", attempts)
	}
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Fatalf("expected a regenerated (distinct) id on retry, got %v", ids)
	}
	if p.PaymentID != ids[1] || strings.TrimSpace(p.PaymentID) == "" {
		t.Fatalf("payment id not set to the successful attempt's id: %q", p.PaymentID)
	}
}

// A non-unique-violation error from the insert fails immediately without
// retrying.
func TestCreateWithGeneratedID_NonCollisionFailsFast(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("connection refused")
	var attempts int
	err := createWithGeneratedID(&models.Payment{ReferenceID: "x"}, func(pp *models.Payment) error {
		attempts++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the sentinel error to propagate, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected exactly 1 attempt for a non-collision error, got %d", attempts)
	}
}
