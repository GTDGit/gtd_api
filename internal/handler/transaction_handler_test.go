package handler

import (
	"net/http"
	"testing"

	"github.com/GTDGit/gtd_api/internal/models"
)

func TestTransactionCreateMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		reqType string
		status  models.TransactionStatus
		want    string
	}{
		{name: "prepaid processing", reqType: "prepaid", status: models.StatusProcessing, want: "Transaction is being processed"},
		{name: "prepaid failed", reqType: "prepaid", status: models.StatusFailed, want: "Transaction failed"},
		{name: "prepaid success", reqType: "prepaid", status: models.StatusSuccess, want: "Transaction success"},
		{name: "inquiry success", reqType: "inquiry", status: models.StatusSuccess, want: "Inquiry success"},
		{name: "inquiry pending", reqType: "inquiry", status: models.StatusPending, want: "Inquiry is being processed"},
		{name: "inquiry failed", reqType: "inquiry", status: models.StatusFailed, want: "Inquiry failed"},
		{name: "payment failed", reqType: "payment", status: models.StatusFailed, want: "Payment failed"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := transactionCreateMessage(tc.reqType, tc.status); got != tc.want {
				t.Fatalf("transactionCreateMessage(%q, %q) = %q, want %q", tc.reqType, tc.status, got, tc.want)
			}
		})
	}
}

func TestTransactionCreateHTTPCode(t *testing.T) {
	t.Parallel()

	failedCode := "PROVIDER_TIMEOUT"
	tests := []struct {
		name    string
		reqType string
		trx     *models.Transaction
		want    int
	}{
		{
			name:    "inquiry success",
			reqType: "inquiry",
			trx:     &models.Transaction{Status: models.StatusSuccess},
			want:    http.StatusOK,
		},
		{
			name:    "prepaid success",
			reqType: "prepaid",
			trx:     &models.Transaction{Status: models.StatusSuccess},
			want:    http.StatusCreated,
		},
		{
			name:    "payment pending",
			reqType: "payment",
			trx:     &models.Transaction{Status: models.StatusProcessing},
			want:    http.StatusAccepted,
		},
		{
			name:    "failed canonical timeout",
			reqType: "prepaid",
			trx:     &models.Transaction{Status: models.StatusFailed, FailedCode: &failedCode},
			want:    http.StatusGatewayTimeout,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := transactionCreateHTTPCode(tc.reqType, tc.trx); got != tc.want {
				t.Fatalf("transactionCreateHTTPCode(%q, %+v) = %d, want %d", tc.reqType, tc.trx, got, tc.want)
			}
		})
	}
}
