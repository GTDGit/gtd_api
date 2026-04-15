package worker

import (
	"context"
	"testing"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
)

func TestStatusCheckWorkerUsesProviderSpecificAges(t *testing.T) {
	t.Parallel()

	w := &StatusCheckWorker{
		maxAge:         5 * time.Minute,
		kiosbankMinAge: 5 * time.Minute,
		kiosbankMaxAge: 72 * time.Hour,
	}

	kiosbankCode := string(models.ProviderKiosbank)
	kiosbankTrx := &models.Transaction{ProviderCode: &kiosbankCode}
	otherCode := "alterra"
	otherTrx := &models.Transaction{ProviderCode: &otherCode}

	if got := w.minAgeFor(kiosbankTrx); got != 5*time.Minute {
		t.Fatalf("minAgeFor(kiosbank) = %s, want 5m", got)
	}
	if got := w.maxAgeFor(kiosbankTrx); got != 72*time.Hour {
		t.Fatalf("maxAgeFor(kiosbank) = %s, want 72h", got)
	}
	if got := w.minAgeFor(otherTrx); got != 0 {
		t.Fatalf("minAgeFor(other) = %s, want 0", got)
	}
	if got := w.maxAgeFor(otherTrx); got != 5*time.Minute {
		t.Fatalf("maxAgeFor(other) = %s, want 5m", got)
	}
}

func TestStatusCheckWorkerSkipsYoungKiosbankTransactions(t *testing.T) {
	t.Parallel()

	kiosbankCode := string(models.ProviderKiosbank)
	refID := "760864752227"
	trx := &models.Transaction{
		TransactionID: "GRB-20260414-000001",
		Status:        models.StatusProcessing,
		CreatedAt:     time.Now().Add(-4 * time.Minute),
		ProviderCode:  &kiosbankCode,
		ProviderRefID: &refID,
	}

	w := &StatusCheckWorker{
		maxAge:         5 * time.Minute,
		kiosbankMinAge: 5 * time.Minute,
		kiosbankMaxAge: 72 * time.Hour,
	}

	w.checkTransaction(context.Background(), trx)

	if trx.Status != models.StatusProcessing {
		t.Fatalf("Status = %q, want %q", trx.Status, models.StatusProcessing)
	}
	if trx.ProcessedAt != nil {
		t.Fatalf("ProcessedAt = %v, want nil", trx.ProcessedAt)
	}
}
