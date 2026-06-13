package service

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/pakailink"
)

// Shared helpers for the payout/disbursement and connector layers. These were
// previously defined on the old transfer service; they live here now so both
// the payout service and the inbound provider connectors can use them.

func stringPtr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func intPtr(v int) *int { return &v }

func nonEmptyOrDefault(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

func isNumericString(v string) bool {
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return v != ""
}

func normalizeComparableString(v string) string {
	return strings.ToUpper(strings.Join(strings.Fields(strings.TrimSpace(v)), " "))
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

func isReferenceUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	if pqErr.Code != "23505" {
		return false
	}
	return strings.Contains(strings.ToLower(pqErr.Constraint), "reference")
}

func randomDigits(length int) int64 {
	if length <= 0 {
		return 0
	}
	max := int64(1)
	for i := 0; i < length; i++ {
		max *= 10
	}
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		return time.Now().UnixNano() % max
	}
	return n.Int64()
}

// newPayoutPublicID builds a human-readable public id like PAY-20260612-000123.
func newPayoutPublicID(prefix string) string {
	now := time.Now().In(time.FixedZone("WIB", 7*3600))
	return fmt.Sprintf("%s-%s-%06d", prefix, now.Format("20060102"), randomDigits(6))
}

func formatPayoutTime(t time.Time) string {
	wib := time.FixedZone("WIB", 7*3600)
	return t.In(wib).Format("2006-01-02T15:04:05+07:00")
}

// mergePayoutProviderData merges a keyed payload into the JSON provider_data
// blob, preserving prior keys (inquiry/submit/status/callback/*_error).
func mergePayoutProviderData(current models.NullableRawMessage, key string, payload any) models.NullableRawMessage {
	merged := map[string]any{}
	if len(current) > 0 {
		_ = json.Unmarshal(current, &merged)
	}
	if strings.TrimSpace(key) != "" && payload != nil {
		merged[key] = payload
	}
	if len(merged) == 0 {
		return nil
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return current
	}
	return models.NullableRawMessage(raw)
}

// payoutErrorPayload renders an error into a JSON-storable map, extracting SNAP
// fields when present.
func payoutErrorPayload(err error) map[string]any {
	payload := map[string]any{"message": err.Error()}
	if info, ok := extractSNAPError(err); ok {
		payload["httpStatus"] = info.HTTPStatus
		payload["responseCode"] = info.ResponseCode
		payload["responseMessage"] = info.ResponseMessage
		if len(info.RawResponse) > 0 {
			payload["rawResponse"] = json.RawMessage(info.RawResponse)
		}
	}
	return payload
}

// PakailinkFeeFromResponse extracts feeAmount.value from a PakaiLink Service 43
// response so callers can store the actual fee charged.
func PakailinkFeeFromResponse(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var p struct {
		FeeAmount pakailink.Amount `json:"feeAmount"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return 0
	}
	v, _ := pakailink.ParseWebhookAmount(p.FeeAmount)
	return v
}
