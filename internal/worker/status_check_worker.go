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

// StatusCheckWorker re-checks status of Processing transactions by calling Digiflazz again.
// Digiflazz is idempotent - calling with same ref_id returns current status, not creating new transaction.
type StatusCheckWorker struct {
	trxRepo     *repository.TransactionRepository
	skuRepo     *repository.SKURepository
	callbackSvc *service.CallbackService
	digiProd    *digiflazz.Client
	digiDev     *digiflazz.Client
	interval    time.Duration
	staleAfter  time.Duration // How long to wait before re-checking (e.g., 10 seconds)
	maxAge      time.Duration // Max age before marking as failed (e.g., 5 minutes)
}

// NewStatusCheckWorker constructs a StatusCheckWorker.
func NewStatusCheckWorker(
	trxRepo *repository.TransactionRepository,
	skuRepo *repository.SKURepository,
	callbackSvc *service.CallbackService,
	digiProd *digiflazz.Client,
	digiDev *digiflazz.Client,
	interval time.Duration,
	staleAfter time.Duration,
	maxAge time.Duration,
) *StatusCheckWorker {
	return &StatusCheckWorker{
		trxRepo:     trxRepo,
		skuRepo:     skuRepo,
		callbackSvc: callbackSvc,
		digiProd:    digiProd,
		digiDev:     digiDev,
		interval:    interval,
		staleAfter:  staleAfter,
		maxAge:      maxAge,
	}
}

// Start begins the periodic status check loop until context is canceled.
func (w *StatusCheckWorker) Start(ctx context.Context) {
	log.Info().
		Dur("interval", w.interval).
		Dur("stale_after", w.staleAfter).
		Dur("max_age", w.maxAge).
		Msg("Starting status check worker")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.run(ctx)
		case <-ctx.Done():
			log.Info().Msg("Status check worker stopped")
			return
		}
	}
}

func (w *StatusCheckWorker) run(ctx context.Context) {
	// Get Processing transactions that haven't received callback
	stale, err := w.trxRepo.GetStaleProcessingTransactions(w.staleAfter)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get stale processing transactions")
		return
	}
	if len(stale) == 0 {
		return
	}

	log.Info().Int("count", len(stale)).Msg("Re-checking stale Processing transactions")

	for i := range stale {
		select {
		case <-ctx.Done():
			return
		default:
			w.checkTransaction(ctx, &stale[i])
		}
	}
}

func (w *StatusCheckWorker) checkTransaction(ctx context.Context, trx *models.Transaction) {
	// Check if too old - mark as failed
	if time.Since(trx.CreatedAt) > w.maxAge {
		log.Warn().
			Str("transaction_id", trx.TransactionID).
			Dur("age", time.Since(trx.CreatedAt)).
			Msg("Transaction too old, marking as failed")
		w.markFailed(trx, "Transaction timeout - no response from provider")
		return
	}

	log.Info().
		Str("transaction_id", trx.TransactionID).
		Str("digi_ref_id", *trx.DigiRefID).
		Dur("age", time.Since(trx.CreatedAt)).
		Msg("Re-checking transaction status with Digiflazz")

	// Get SKU to get the digi_sku_code
	if trx.SkuID == nil {
		log.Error().Str("transaction_id", trx.TransactionID).Msg("Transaction has no SKU ID")
		return
	}

	sku, err := w.skuRepo.GetByID(*trx.SkuID)
	if err != nil {
		log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Failed to get SKU")
		return
	}

	// Call Digiflazz with same ref_id - this will return current status
	digi := w.digiProd
	if trx.IsSandbox {
		digi = w.digiDev
	}

	// Use the stored digi_ref_id (same as original request)
	resp, err := digi.Topup(ctx, sku.DigiSkuCode, trx.CustomerNo, *trx.DigiRefID, trx.IsSandbox)
	if err != nil {
		log.Warn().
			Err(err).
			Str("transaction_id", trx.TransactionID).
			Msg("Network error checking transaction status, will retry later")
		return // Don't fail, will retry on next run
	}

	log.Info().
		Str("transaction_id", trx.TransactionID).
		Str("rc", resp.RC).
		Str("status", resp.Status).
		Str("sn", resp.SN).
		Msg("Status check response from Digiflazz")

	// Process response
	now := time.Now()

	switch {
	case digiflazz.IsSuccess(resp.RC):
		trx.Status = models.StatusSuccess
		if resp.SN != "" {
			trx.SerialNumber = &resp.SN
		}
		trx.Amount = &resp.Price
		trx.ProcessedAt = &now

		if err := w.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Failed to update transaction to success")
			return
		}

		go w.callbackSvc.SendCallback(trx, "transaction.success")
		log.Info().Str("transaction_id", trx.TransactionID).Msg("Transaction updated to Success from status check")

	case digiflazz.IsFatal(resp.RC):
		msg := resp.Message
		rc := resp.RC
		trx.Status = models.StatusFailed
		trx.FailedReason = &msg
		trx.FailedCode = &rc
		trx.ProcessedAt = &now

		if err := w.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Failed to update transaction to failed")
			return
		}

		go w.callbackSvc.SendCallback(trx, "transaction.failed")
		log.Info().
			Str("transaction_id", trx.TransactionID).
			Str("rc", resp.RC).
			Msg("Transaction updated to Failed from status check (fatal RC)")

	case digiflazz.IsRetryable(resp.RC):
		// Retryable RC - this means the transaction failed at Digiflazz
		// Unlike callback worker, we don't retry here - just mark as failed
		// because status check is for transactions already sent, not for new attempts
		msg := resp.Message
		rc := resp.RC
		trx.Status = models.StatusFailed
		trx.FailedReason = &msg
		trx.FailedCode = &rc
		trx.ProcessedAt = &now

		if err := w.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Failed to update transaction to failed")
			return
		}

		go w.callbackSvc.SendCallback(trx, "transaction.failed")
		log.Info().
			Str("transaction_id", trx.TransactionID).
			Str("rc", resp.RC).
			Msg("Transaction updated to Failed from status check (retryable RC)")

	case digiflazz.IsPending(resp.RC):
		// Still pending, will check again on next run
		log.Debug().
			Str("transaction_id", trx.TransactionID).
			Msg("Transaction still pending from status check")

	default:
		// Unknown RC, log but don't change status
		log.Warn().
			Str("transaction_id", trx.TransactionID).
			Str("rc", resp.RC).
			Msg("Unknown RC from status check, keeping as Processing")
	}
}

func (w *StatusCheckWorker) markFailed(trx *models.Transaction, reason string) {
	now := time.Now()
	trx.Status = models.StatusFailed
	trx.FailedReason = &reason
	trx.ProcessedAt = &now

	if err := w.trxRepo.Update(trx); err != nil {
		log.Error().
			Err(err).
			Str("transaction_id", trx.TransactionID).
			Msg("Failed to mark transaction as failed")
		return
	}

	go w.callbackSvc.SendCallback(trx, "transaction.failed")
	log.Info().
		Str("transaction_id", trx.TransactionID).
		Str("reason", reason).
		Msg("Transaction marked as failed")
}
