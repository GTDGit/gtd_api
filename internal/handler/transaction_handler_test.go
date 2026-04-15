package handler

import (
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
		{name: "prepaid processing", reqType: "prepaid", status: models.StatusProcessing, want: "Transaction processing"},
		{name: "prepaid failed", reqType: "prepaid", status: models.StatusFailed, want: "Transaction failed"},
		{name: "prepaid success", reqType: "prepaid", status: models.StatusSuccess, want: "Transaction success"},
		{name: "inquiry success", reqType: "inquiry", status: models.StatusSuccess, want: "Inquiry success"},
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
