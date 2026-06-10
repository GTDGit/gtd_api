package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/GTDGit/gtd_api/internal/models"
)

// ----------------------------------------------------------------------------
// Shared generators (payment-provider-enhancements property suite)
//
// These generators constrain inputs to the valid space for each property:
// canonical payment types/codes, valid fee bearers, non-negative amounts, and
// realistic provider bindings. rapid runs >= 100 iterations per property by
// default (override with -rapid.checks).
// ----------------------------------------------------------------------------

// genPaymentType draws one of the four canonical payment types.
func genPaymentType(t *rapid.T) models.PaymentType {
	return rapid.SampledFrom([]models.PaymentType{
		models.PaymentTypeVA,
		models.PaymentTypeEwallet,
		models.PaymentTypeQRIS,
		models.PaymentTypeRetail,
	}).Draw(t, "paymentType")
}

// genPaymentCode draws a representative method code from the design matrix.
func genPaymentCode(t *rapid.T) string {
	return rapid.SampledFrom([]string{
		"MPM", "CPM", "ALFAMART", "INDOMARET",
		"DANA", "GOPAY", "OVO", "LINKAJA", "SHOPEEPAY", "ASTRAPAY",
		"014", "002", "009", "008",
	}).Draw(t, "paymentCode")
}

// genPaymentStatus draws any payment status.
func genPaymentStatus(t *rapid.T) models.PaymentStatus {
	return rapid.SampledFrom([]models.PaymentStatus{
		models.PaymentStatusPending,
		models.PaymentStatusSuccess,
		models.PaymentStatusExpired,
		models.PaymentStatusCancelled,
		models.PaymentStatusFailed,
	}).Draw(t, "status")
}

// genFeePaidBy draws a valid fee bearer.
func genFeePaidBy(t *rapid.T) models.FeePaidBy {
	return rapid.SampledFrom([]models.FeePaidBy{
		models.FeePaidByMerchant,
		models.FeePaidByCustomer,
	}).Draw(t, "feePaidBy")
}

// genProvider draws an internal provider. The provider must NEVER leak into the
// marshaled Standard_Response, so we randomize it to make the absence robust.
func genProvider(t *rapid.T) models.PaymentProvider {
	return rapid.SampledFrom([]models.PaymentProvider{
		models.ProviderPakailink,
		models.ProviderDanaDirect,
		models.ProviderMidtrans,
		models.ProviderXendit,
		models.ProviderOVODirect,
	}).Draw(t, "provider")
}

// genPayment builds a fully populated persisted payment with random values,
// keeping the fee-bearer/total invariant consistent.
func genPayment(t *rapid.T) *models.Payment {
	feePaidBy := genFeePaidBy(t)
	subtotal := rapid.Int64Range(0, 1_000_000_000).Draw(t, "subtotal")
	fee := rapid.Int64Range(0, 1_000_000).Draw(t, "fee")
	total := computeTotal(subtotal, fee, feePaidBy)

	providerRef := rapid.StringMatching(`[A-Za-z0-9]{0,20}`).Draw(t, "providerRef")
	// customerName includes unicode/empty per the testing-strategy edge cases.
	customerName := rapid.SampledFrom([]string{
		"", "Budi", "Tëst Üser", "山田太郎", "O'Brien",
	}).Draw(t, "customerName")

	p := &models.Payment{
		PaymentID:   uuid.New().String(),
		ReferenceID: rapid.StringMatching(`[A-Za-z0-9_-]{1,24}`).Draw(t, "referenceId"),
		PaymentType: genPaymentType(t),
		PaymentCode: genPaymentCode(t),
		Provider:    genProvider(t),
		Amount:      subtotal,
		Fee:         fee,
		TotalAmount: total,
		FeePaidBy:   feePaidBy,
		Status:      genPaymentStatus(t),
		// payment_detail always carries a (possibly empty) JSON object.
		PaymentDetail: models.NullableRawMessage(`{}`),
	}
	if providerRef != "" {
		p.ProviderRef = &providerRef
	}
	if customerName != "" {
		v := customerName
		p.CustomerName = &v
	}
	return p
}

// ----------------------------------------------------------------------------
// Property 2: Fee-bearer determines the total
// ----------------------------------------------------------------------------

// Feature: payment-provider-enhancements, Property 2: For any non-negative subtotal and
// fee, the computed amount.total equals subtotal+fee when feePaidBy=customer and equals
// subtotal when feePaidBy=merchant; subtotal and fee are unchanged by the bearer choice.
//
// Validates: Requirements 2.3, 2.4, 16.3
func TestProperty2_FeeBearerDeterminesTotal(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		subtotal := rapid.Int64Range(0, 1_000_000_000_000).Draw(t, "subtotal")
		fee := rapid.Int64Range(0, 1_000_000_000).Draw(t, "fee")

		merchantTotal := computeTotal(subtotal, fee, models.FeePaidByMerchant)
		customerTotal := computeTotal(subtotal, fee, models.FeePaidByCustomer)

		if merchantTotal != subtotal {
			t.Fatalf("merchant total = %d, want subtotal %d", merchantTotal, subtotal)
		}
		if customerTotal != subtotal+fee {
			t.Fatalf("customer total = %d, want subtotal+fee %d", customerTotal, subtotal+fee)
		}

		// subtotal and fee themselves must be unaffected by the bearer choice:
		// computeTotal is pure and takes them by value, so re-deriving with the
		// opposite bearer must still agree with the formula on the same inputs.
		if computeTotal(subtotal, fee, models.FeePaidByMerchant) != subtotal {
			t.Fatalf("merchant total not stable for subtotal=%d fee=%d", subtotal, fee)
		}
		if computeTotal(subtotal, fee, models.FeePaidByCustomer) != subtotal+fee {
			t.Fatalf("customer total not stable for subtotal=%d fee=%d", subtotal, fee)
		}
	})
}

// ----------------------------------------------------------------------------
// Property 3: Create/get response follows the Standard_Response shape
// ----------------------------------------------------------------------------

// Feature: payment-provider-enhancements, Property 3: For any persisted payment, the
// marshaled create/get response contains a nested paymentMethod{type,code}, a nested
// amount{subtotal,fee,total}, a feePaidBy field equal to the transaction value, and
// contains no flat paymentType/paymentCode/totalAmount/paymentId keys and no provider
// name or providerRef key.
//
// Validates: Requirements 2.6, 4.1, 4.2, 4.4, 16.2
func TestProperty3_StandardResponseShape(t *testing.T) {
	svc := &PaymentService{}

	rapid.Check(t, func(t *rapid.T) {
		p := genPayment(t)

		resp := svc.buildResponse(p)
		raw, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}

		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}

		// Nested paymentMethod{type,code}.
		pmRaw, ok := obj["paymentMethod"]
		if !ok {
			t.Fatalf("response missing nested paymentMethod: %s", raw)
		}
		var pm map[string]json.RawMessage
		if err := json.Unmarshal(pmRaw, &pm); err != nil {
			t.Fatalf("paymentMethod is not an object: %v", err)
		}
		for _, k := range []string{"type", "code"} {
			if _, ok := pm[k]; !ok {
				t.Fatalf("paymentMethod missing %s: %s", k, pmRaw)
			}
		}

		// Nested amount{subtotal,fee,total}.
		amtRaw, ok := obj["amount"]
		if !ok {
			t.Fatalf("response missing nested amount: %s", raw)
		}
		var amt map[string]json.RawMessage
		if err := json.Unmarshal(amtRaw, &amt); err != nil {
			t.Fatalf("amount is not an object: %v", err)
		}
		for _, k := range []string{"subtotal", "fee", "total"} {
			if _, ok := amt[k]; !ok {
				t.Fatalf("amount missing %s: %s", k, amtRaw)
			}
		}

		// feePaidBy present and equal to the transaction value.
		var fpb string
		fpbRaw, ok := obj["feePaidBy"]
		if !ok {
			t.Fatalf("response missing feePaidBy: %s", raw)
		}
		if err := json.Unmarshal(fpbRaw, &fpb); err != nil {
			t.Fatalf("feePaidBy not a string: %v", err)
		}
		if fpb != string(p.FeePaidBy) {
			t.Fatalf("feePaidBy mismatch: got %q want %q", fpb, p.FeePaidBy)
		}

		// No flat keys.
		for _, forbidden := range []string{"paymentType", "paymentCode", "totalAmount", "paymentId"} {
			if _, found := obj[forbidden]; found {
				t.Fatalf("response contains forbidden flat key %q: %s", forbidden, raw)
			}
		}
		// No provider name and no providerRef anywhere in the top-level response.
		for _, forbidden := range []string{"provider", "providerRef"} {
			if _, found := obj[forbidden]; found {
				t.Fatalf("response contains forbidden provider key %q: %s", forbidden, raw)
			}
		}
		// Belt-and-suspenders: the raw bytes must not mention providerRef at all.
		if strings.Contains(string(raw), "providerRef") {
			t.Fatalf("response leaks providerRef: %s", raw)
		}
	})
}

// ----------------------------------------------------------------------------
// Property 5: All generated identifiers are UUIDv4
// ----------------------------------------------------------------------------

// isUUIDv4 reports whether s parses as a UUID with version 4.
func isUUIDv4(s string) bool {
	u, err := uuid.Parse(s)
	if err != nil {
		return false
	}
	return u.Version() == 4
}

// Feature: payment-provider-enhancements, Property 5: For any created payment the
// generated id is a syntactically valid UUIDv4, and for any outbound webhook delivery the
// X-GTD-Request-Id value is a syntactically valid UUIDv4.
//
// Validates: Requirements 4.3, 5.1, 5.2
func TestProperty5_GeneratedIdentifiersAreUUIDv4(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw an unused value so each iteration is a distinct rapid run.
		_ = rapid.IntRange(0, 1_000_000).Draw(t, "n")

		paymentID := newPaymentPublicID("")
		if !isUUIDv4(paymentID) {
			t.Fatalf("payment id is not a UUIDv4: %q", paymentID)
		}

		reqID := genPaymentRequestID()
		if !isUUIDv4(reqID) {
			t.Fatalf("webhook request id is not a UUIDv4: %q", reqID)
		}
	})
}

// ----------------------------------------------------------------------------
// Property 6: Provider selection respects priority and health
// ----------------------------------------------------------------------------

// stubProviderAdapter is a controllable PaymentProviderClient used to drive the
// ProviderSelector health logic (registered + Available()) in tests.
type stubProviderAdapter struct {
	code      models.PaymentProvider
	available bool
}

func (s *stubProviderAdapter) Code() models.PaymentProvider { return s.code }
func (s *stubProviderAdapter) Available() bool              { return s.available }
func (s *stubProviderAdapter) CreatePayment(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	return &PaymentCreateResponse{}, nil
}
func (s *stubProviderAdapter) InquiryPayment(ctx context.Context, payment *models.Payment) (*PaymentInquiryResult, error) {
	return &PaymentInquiryResult{}, nil
}
func (s *stubProviderAdapter) CancelPayment(ctx context.Context, payment *models.Payment, reason string) (*PaymentCancelResult, error) {
	return &PaymentCancelResult{}, nil
}

// allProvidersForSelector is the fixed provider universe used to build random
// bindings; each may independently be registered/available.
var allProvidersForSelector = []models.PaymentProvider{
	models.ProviderPakailink,
	models.ProviderDanaDirect,
	models.ProviderXendit,
	models.ProviderMidtrans,
	models.ProviderOVODirect,
}

// Feature: payment-provider-enhancements, Property 6: For any set of method-provider
// bindings, Select returns the binding with the lowest priority among those that are
// active, not in maintenance, have a registered adapter, and whose adapter reports
// available; if none qualify it returns PAYMENT_METHOD_UNAVAILABLE.
//
// Validates: Requirements 3.2, 6.16, 6.17, 13.3
func TestProperty6_ProviderSelectionRespectsPriorityAndHealth(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		router := NewPaymentProviderRouter()

		// Decide per-provider registration + adapter availability.
		registered := map[models.PaymentProvider]bool{}
		available := map[models.PaymentProvider]bool{}
		for _, prov := range allProvidersForSelector {
			isReg := rapid.Bool().Draw(t, "registered_"+string(prov))
			isAvail := rapid.Bool().Draw(t, "available_"+string(prov))
			registered[prov] = isReg
			available[prov] = isAvail
			if isReg {
				router.Register(&stubProviderAdapter{code: prov, available: isAvail})
			}
		}

		selector := NewProviderSelector(nil, router)

		// Build a random set of bindings (subset of providers, no duplicates),
		// each with random priority and health flags.
		n := rapid.IntRange(0, len(allProvidersForSelector)).Draw(t, "numBindings")
		perm := rapid.Permutation(allProvidersForSelector).Draw(t, "perm")
		bindings := make([]models.MethodProviderBinding, 0, n)
		for i := 0; i < n; i++ {
			prov := perm[i]
			bindings = append(bindings, models.MethodProviderBinding{
				ID:            i + 1,
				Provider:      prov,
				Priority:      rapid.IntRange(0, 1000).Draw(t, "priority_"+string(prov)),
				IsActive:      rapid.Bool().Draw(t, "active_"+string(prov)),
				IsMaintenance: rapid.Bool().Draw(t, "maintenance_"+string(prov)),
			})
		}

		// The selector expects bindings ordered by priority ASC (as the repo
		// would return them). Sort a copy to mirror that contract.
		sorted := make([]models.MethodProviderBinding, len(bindings))
		copy(sorted, bindings)
		sortBindingsByPriority(sorted)

		group := &models.MethodGroup{
			Type:      models.PaymentTypeEwallet,
			Code:      "OVO",
			Providers: sorted,
		}

		// Independently compute the expected winner: the healthy binding with
		// the lowest priority (ties broken by the sorted order, i.e. first).
		healthy := func(b models.MethodProviderBinding) bool {
			return b.IsActive && !b.IsMaintenance && registered[b.Provider] && available[b.Provider]
		}
		var want *models.MethodProviderBinding
		for i := range sorted {
			if healthy(sorted[i]) {
				want = &sorted[i]
				break
			}
		}

		got, err := selector.Select(group)
		if want == nil {
			if err == nil {
				t.Fatalf("expected METHOD_UNAVAILABLE, got binding %+v", got)
			}
			var pe *PaymentServiceError
			if !asPaymentError(err, &pe) || pe.Code != "METHOD_UNAVAILABLE" {
				t.Fatalf("expected METHOD_UNAVAILABLE error, got %v", err)
			}
			return
		}
		if err != nil {
			t.Fatalf("unexpected error: %v (expected binding for %s)", err, want.Provider)
		}
		if got.Provider != want.Provider || got.Priority != want.Priority {
			t.Fatalf("selected %s(p=%d), want %s(p=%d)", got.Provider, got.Priority, want.Provider, want.Priority)
		}
		// The chosen binding must itself be healthy.
		if !healthy(*got) {
			t.Fatalf("selected an unhealthy binding: %+v", got)
		}
	})
}

// sortBindingsByPriority sorts bindings by Priority ASC (stable), mirroring the
// repository's "ORDER BY priority ASC" contract.
func sortBindingsByPriority(b []models.MethodProviderBinding) {
	for i := 1; i < len(b); i++ {
		for j := i; j > 0 && b[j-1].Priority > b[j].Priority; j-- {
			b[j-1], b[j] = b[j], b[j-1]
		}
	}
}

// asPaymentError is a tiny errors.As wrapper kept local to avoid importing
// errors in multiple test files.
func asPaymentError(err error, target **PaymentServiceError) bool {
	return errors.As(err, target)
}

// ----------------------------------------------------------------------------
// Property 7: Method list is de-duplicated by (type, code)
// ----------------------------------------------------------------------------

// Feature: payment-provider-enhancements, Property 7: For any set of active payment
// methods, the de-duplicated list contains at most one entry per (type, code) pair.
//
// Validates: Requirements 6.18
func TestProperty7_MethodListDeduplicatedByTypeCode(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 40).Draw(t, "numMethods")
		methods := make([]models.PaymentMethod, 0, n)
		for i := 0; i < n; i++ {
			methods = append(methods, models.PaymentMethod{
				ID:       i + 1,
				Type:     genPaymentType(t),
				Code:     genPaymentCode(t),
				Provider: genProvider(t),
			})
		}

		out := dedupMethodsByTypeCode(methods)

		// At most one entry per (type, code).
		seen := map[string]bool{}
		for _, m := range out {
			key := string(m.Type) + "\x00" + m.Code
			if seen[key] {
				t.Fatalf("duplicate (type,code) in output: %s/%s", m.Type, m.Code)
			}
			seen[key] = true
		}

		// Every distinct input (type, code) is represented exactly once.
		want := map[string]bool{}
		for _, m := range methods {
			want[string(m.Type)+"\x00"+m.Code] = true
		}
		if len(seen) != len(want) {
			t.Fatalf("output covers %d distinct keys, want %d", len(seen), len(want))
		}
	})
}
