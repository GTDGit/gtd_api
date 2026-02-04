package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/service"
)

// RetryWorker cleans up any lingering pending transactions.
// With the new logic, transactions fail immediately when all SKUs are exhausted,
// so this worker mainly serves as a cleanup mechanism for edge cases.
type RetryWorker struct {
	trxRepo     *repository.TransactionRepository
	callbackSvc *service.CallbackService
	interval    time.Duration
}

// NewRetryWorker constructs a RetryWorker.
func NewRetryWorker(
	trxRepo *repository.TransactionRepository,
	callbackSvc *service.CallbackService,
	interval time.Duration,
) *RetryWorker {
	return &RetryWorker{
		trxRepo:     trxRepo,
		callbackSvc: callbackSvc,
		interval:    interval,
	}
}

// Start begins the periodic retry loop until context is canceled.
func (w *RetryWorker) Start(ctx context.Context) {
	log.Info().Dur("interval", w.interval).Msg("Starting retry worker")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.run(ctx)
		case <-ctx.Done():
			log.Info().Msg("Retry worker stopped")
			return
		}
	}
}

func (w *RetryWorker) run(ctx context.Context) {
	// Mark any lingering pending transactions as failed
	// Note: With the new logic, prepaid transactions should not enter Pending state
	// (they fail immediately when all SKUs are exhausted). This is a safety net
	// for any edge cases or legacy data.
	w.markExpiredTransactionsFailed(ctx)
}

// markExpiredTransactionsFailed marks any lingering Pending transactions as Failed.
// With the new logic, transactions should fail immediately when all SKUs are exhausted,
// so this is mainly a cleanup for edge cases.
func (w *RetryWorker) markExpiredTransactionsFailed(ctx context.Context) {
	// Get any pending transactions (shouldn't be many with new logic)
	pending, err := w.trxRepo.GetAllPendingTransactions()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get pending transactions")
		return
	}
	if len(pending) == 0 {
		return
	}

	log.Info().Int("count", len(pending)).Msg("Marking lingering pending transactions as failed")

	for i := range pending {
		select {
		case <-ctx.Done():
			return
		default:
			trx := &pending[i]
			now := time.Now()
			reason := "Transaction expired in pending state"
			trx.Status = models.StatusFailed
			trx.FailedReason = &reason
			trx.ProcessedAt = &now
			trx.NextRetryAt = nil

			if err := w.trxRepo.Update(trx); err != nil {
				log.Error().
					Err(err).
					Str("transaction_id", trx.TransactionID).
					Msg("Failed to mark transaction as failed")
				continue
			}

			// Send callback to client
			go w.callbackSvc.SendCallback(trx, "transaction.failed")

			log.Info().
				Str("transaction_id", trx.TransactionID).
				Msg("Pending transaction marked as failed")
		}
	}
}
