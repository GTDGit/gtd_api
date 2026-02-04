package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/pkg/digiflazz"
)

// DigiflazzCallbackWorker processes unprocessed Digiflazz callbacks and reconciles transactions.
type DigiflazzCallbackWorker struct {
	callbackRepo *repository.CallbackRepository
	trxRepo      *repository.TransactionRepository
	trxSvc       *service.TransactionService
	callbackSvc  *service.CallbackService
	interval     time.Duration
}

// NewDigiflazzCallbackWorker constructs a DigiflazzCallbackWorker.
func NewDigiflazzCallbackWorker(
	callbackRepo *repository.CallbackRepository,
	trxRepo *repository.TransactionRepository,
	trxSvc *service.TransactionService,
	callbackSvc *service.CallbackService,
	interval time.Duration,
) *DigiflazzCallbackWorker {
	return &DigiflazzCallbackWorker{
		callbackRepo: callbackRepo,
		trxRepo:      trxRepo,
		trxSvc:       trxSvc,
		callbackSvc:  callbackSvc,
		interval:     interval,
	}
}

// Start begins the processing loop until context is canceled.
func (w *DigiflazzCallbackWorker) Start(ctx context.Context) {
	log.Info().Dur("interval", w.interval).Msg("Starting Digiflazz callback worker")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.run(ctx)
		case <-ctx.Done():
			log.Info().Msg("Digiflazz callback worker stopped")
			return
		}
	}
}

func (w *DigiflazzCallbackWorker) run(ctx context.Context) {
	callbacks, err := w.callbackRepo.GetUnprocessedCallbacks()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get unprocessed Digiflazz callbacks")
		return
	}
	if len(callbacks) == 0 {
		return
	}
	log.Info().Int("count", len(callbacks)).Msg("Processing Digiflazz callbacks")

	for i := range callbacks {
		select {
		case <-ctx.Done():
			return
		default:
			w.processCallback(ctx, &callbacks[i])
		}
	}
}

func (w *DigiflazzCallbackWorker) processCallback(ctx context.Context, cb *models.DigiflazzCallback) {
	log.Info().
		Int("id", cb.ID).
		Str("digi_ref_id", cb.DigiRefID).
		Msg("Processing Digiflazz callback")

	// Find transaction by digi_ref_id
	trx, err := w.trxRepo.GetByDigiRefID(cb.DigiRefID)
	if err != nil {
		log.Warn().Err(err).Str("digi_ref_id", cb.DigiRefID).Msg("[CALLBACK WORKER] GetByDigiRefID failed, trying alternatives")
		// Try to find by transaction_id prefix (ref_id might have suffix like GRB-20250124-000001-1)
		baseRefID := extractBaseRefID(cb.DigiRefID)
		if baseRefID != cb.DigiRefID {
			trx, err = w.trxRepo.GetByDigiRefID(baseRefID)
			if err != nil {
				log.Warn().Err(err).Str("base_ref_id", baseRefID).Msg("[CALLBACK WORKER] GetByDigiRefID with base failed")
			}
		}
		if err != nil {
			// Also try direct transaction_id lookup
			trx, err = w.trxRepo.GetByTransactionID(baseRefID)
			if err != nil {
				log.Warn().Err(err).Str("transaction_id", baseRefID).Msg("[CALLBACK WORKER] GetByTransactionID failed")
			}
		}
	}

	if err != nil || trx == nil {
		log.Warn().
			Str("digi_ref_id", cb.DigiRefID).
			Msg("Transaction not found for Digiflazz callback")
		// Mark as processed with error
		if err := w.callbackRepo.MarkProcessedWithError(cb.ID, "transaction not found"); err != nil {
			log.Error().Err(err).Msg("Failed to mark callback as processed")
		}
		return
	}

	// Skip if transaction is already in final state
	if trx.Status == models.StatusSuccess || trx.Status == models.StatusFailed {
		log.Debug().
			Str("transaction_id", trx.TransactionID).
			Str("status", string(trx.Status)).
			Msg("Transaction already in final state, skipping callback")
		if err := w.callbackRepo.MarkProcessed(cb.ID); err != nil {
			log.Error().Err(err).Msg("Failed to mark callback as processed")
		}
		return
	}

	// Get RC from callback
	rc := ""
	if cb.RC != nil {
		rc = *cb.RC
	}

	now := time.Now()

	// Process based on RC code
	switch {
	case digiflazz.IsSuccess(rc):
		trx.Status = models.StatusSuccess
		if cb.SerialNumber != nil && *cb.SerialNumber != "" {
			trx.SerialNumber = cb.SerialNumber
		}
		trx.ProcessedAt = &now
		trx.CallbackSent = false // Will be sent below

		if err := w.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Failed to update transaction to success")
			return
		}

		// Send callback to client
		go w.callbackSvc.SendCallback(trx, "transaction.success")
		log.Info().Str("transaction_id", trx.TransactionID).Msg("Transaction updated to Success from Digiflazz callback")

	case digiflazz.IsFatal(rc):
		// Fatal RC - no retry possible, mark as failed immediately
		trx.Status = models.StatusFailed
		if cb.Message != nil {
			trx.FailedReason = cb.Message
		}
		trx.FailedCode = cb.RC
		trx.ProcessedAt = &now

		if err := w.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Failed to update transaction to failed")
			return
		}

		go w.callbackSvc.SendCallback(trx, "transaction.failed")
		log.Info().Str("transaction_id", trx.TransactionID).Str("rc", rc).Msg("Transaction updated to Failed from Digiflazz callback (fatal RC)")

	case digiflazz.IsRetryable(rc):
		// Retryable RC - try with next SKU using same logic as initial transaction
		failedMsg := ""
		if cb.Message != nil {
			failedMsg = *cb.Message
		}

		log.Info().
			Str("transaction_id", trx.TransactionID).
			Str("rc", rc).
			Str("message", failedMsg).
			Msg("Retryable RC from callback, attempting retry with next SKU")

		result, shouldMarkFailed, err := w.trxSvc.RetryWithNextSKU(ctx, trx, rc, failedMsg)
		if err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Error during retry with next SKU")
			return // Don't mark as processed, will retry on next worker run
		}

		if shouldMarkFailed {
			// All SKUs exhausted - transaction already updated and callback sent by tryAllSKUs/handleAllSKUsFailed
			log.Info().Str("transaction_id", trx.TransactionID).Msg("All SKUs exhausted after retry from callback")
		} else {
			// Retry completed (success/pending/fatal) - transaction already handled by tryAllSKUs
			log.Info().
				Str("transaction_id", trx.TransactionID).
				Str("status", string(result.Status)).
				Msg("Retry with next SKU completed")
		}

	case digiflazz.IsPending(rc):
		// Still pending, keep Processing status
		log.Debug().Str("transaction_id", trx.TransactionID).Msg("Transaction still pending from Digiflazz callback")

	default:
		// Unknown RC - treat as failed to be safe
		log.Warn().
			Str("transaction_id", trx.TransactionID).
			Str("rc", rc).
			Msg("Unknown RC code in Digiflazz callback, treating as failed")

		trx.Status = models.StatusFailed
		if cb.Message != nil {
			trx.FailedReason = cb.Message
		}
		trx.FailedCode = cb.RC
		trx.ProcessedAt = &now

		if err := w.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Failed to update transaction to failed")
			return
		}

		go w.callbackSvc.SendCallback(trx, "transaction.failed")
	}

	// Mark callback as processed
	if err := w.callbackRepo.MarkProcessed(cb.ID); err != nil {
		log.Error().Err(err).Msg("Failed to mark callback as processed")
	}
}

// extractBaseRefID extracts the base transaction ID from a ref_id that might have a suffix.
// Example: "GRB-20250124-000001-1" -> "GRB-20250124-000001"
func extractBaseRefID(refID string) string {
	// Count dashes - standard format is GRB-YYYYMMDD-NNNNNN (2 dashes)
	// With suffix it becomes GRB-YYYYMMDD-NNNNNN-N (3 dashes)
	dashCount := 0
	lastDashPos := -1
	for i, c := range refID {
		if c == '-' {
			dashCount++
			if dashCount == 3 {
				lastDashPos = i
				break
			}
		}
	}
	if lastDashPos > 0 {
		return refID[:lastDashPos]
	}
	return refID
}
