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

// StatusCheckWorker re-checks status of Processing transactions by calling providers.
// For multi-provider transactions, it uses ProviderRouter to check the correct provider.
// For legacy Digiflazz transactions, it continues using direct Digiflazz calls.
type StatusCheckWorker struct {
	trxRepo         *repository.TransactionRepository
	skuRepo         *repository.SKURepository
	callbackSvc     *service.CallbackService
	digiProd        *digiflazz.Client
	digiDev         *digiflazz.Client
	providerRouter  *service.ProviderRouter
	providerRetrier service.ProviderFallbackRetrier
	interval        time.Duration
	staleAfter      time.Duration // How long to wait before re-checking (e.g., 10 seconds)
	maxAge          time.Duration // Generic max age before marking as failed
	kiosbankMinAge  time.Duration
	kiosbankMaxAge  time.Duration
}

// NewStatusCheckWorker constructs a StatusCheckWorker.
func NewStatusCheckWorker(
	trxRepo *repository.TransactionRepository,
	skuRepo *repository.SKURepository,
	callbackSvc *service.CallbackService,
	digiProd *digiflazz.Client,
	digiDev *digiflazz.Client,
	providerRouter *service.ProviderRouter,
	providerRetrier service.ProviderFallbackRetrier,
	interval time.Duration,
	staleAfter time.Duration,
	maxAge time.Duration,
	kiosbankMinAge time.Duration,
	kiosbankMaxAge time.Duration,
) *StatusCheckWorker {
	return &StatusCheckWorker{
		trxRepo:         trxRepo,
		skuRepo:         skuRepo,
		callbackSvc:     callbackSvc,
		digiProd:        digiProd,
		digiDev:         digiDev,
		providerRouter:  providerRouter,
		providerRetrier: providerRetrier,
		interval:        interval,
		staleAfter:      staleAfter,
		maxAge:          maxAge,
		kiosbankMinAge:  kiosbankMinAge,
		kiosbankMaxAge:  kiosbankMaxAge,
	}
}

// Start begins the periodic status check loop until context is canceled.
func (w *StatusCheckWorker) Start(ctx context.Context) {
	log.Info().
		Dur("interval", w.interval).
		Dur("stale_after", w.staleAfter).
		Dur("max_age", w.maxAge).
		Dur("kiosbank_min_age", w.kiosbankMinAge).
		Dur("kiosbank_max_age", w.kiosbankMaxAge).
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
	age := time.Since(trx.CreatedAt)
	if minAge := w.minAgeFor(trx); minAge > 0 && age < minAge {
		log.Debug().
			Str("transaction_id", trx.TransactionID).
			Str("provider_code", providerCode(trx)).
			Dur("age", age).
			Dur("required_age", minAge).
			Msg("Skipping status check because transaction has not reached provider min age")
		return
	}

	// Check if too old - mark as failed
	if maxAge := w.maxAgeFor(trx); maxAge > 0 && age > maxAge {
		log.Warn().
			Str("transaction_id", trx.TransactionID).
			Str("provider_code", providerCode(trx)).
			Dur("age", age).
			Msg("Transaction too old, marking as failed")
		w.markFailed(trx, "Transaction timeout - no response from provider")
		return
	}

	// Check if this is a multi-provider transaction
	if trx.ProviderCode != nil && trx.ProviderRefID != nil && *trx.ProviderCode != "" {
		w.checkMultiProviderTransaction(ctx, trx)
		return
	}

	// Legacy Digiflazz transaction
	w.checkDigiflazzTransaction(ctx, trx)
}

func providerCode(trx *models.Transaction) string {
	if trx == nil || trx.ProviderCode == nil {
		return ""
	}
	return *trx.ProviderCode
}

func (w *StatusCheckWorker) minAgeFor(trx *models.Transaction) time.Duration {
	if providerCode(trx) == string(models.ProviderKiosbank) {
		return w.kiosbankMinAge
	}
	return 0
}

func (w *StatusCheckWorker) maxAgeFor(trx *models.Transaction) time.Duration {
	if providerCode(trx) == string(models.ProviderKiosbank) {
		if w.kiosbankMaxAge > 0 {
			return w.kiosbankMaxAge
		}
	}
	return w.maxAge
}

func (w *StatusCheckWorker) checkMultiProviderTransaction(ctx context.Context, trx *models.Transaction) {
	log.Info().
		Str("transaction_id", trx.TransactionID).
		Str("provider_code", *trx.ProviderCode).
		Str("provider_ref_id", *trx.ProviderRefID).
		Dur("age", time.Since(trx.CreatedAt)).
		Msg("Re-checking transaction status with provider")

	if w.providerRouter == nil {
		log.Error().
			Str("transaction_id", trx.TransactionID).
			Msg("ProviderRouter not configured, cannot check multi-provider transaction")
		return
	}

	// Get the provider adapter
	adapter := w.providerRouter.GetAdapter(*trx.ProviderCode)
	if adapter == nil {
		log.Error().
			Str("transaction_id", trx.TransactionID).
			Str("provider_code", *trx.ProviderCode).
			Msg("No adapter found for provider")
		return
	}

	// Check status with the provider
	result, err := adapter.CheckStatus(ctx, *trx.ProviderRefID)
	if err != nil {
		log.Warn().
			Err(err).
			Str("transaction_id", trx.TransactionID).
			Str("provider_code", *trx.ProviderCode).
			Msg("Error checking transaction status with provider, will retry later")
		return
	}

	now := time.Now()
	if len(result.RawResponse) > 0 {
		trx.ProviderResponse = models.NullableRawMessage(result.RawResponse)
	}
	if result.HTTPStatus > 0 {
		httpStatus := result.HTTPStatus
		trx.ProviderHTTPStatus = &httpStatus
	}

	switch {
	case result.Success:
		trx.Status = models.StatusSuccess
		if result.SerialNumber != "" {
			trx.SerialNumber = &result.SerialNumber
		}
		if result.Amount > 0 {
			trx.Amount = &result.Amount
		}
		trx.ProcessedAt = &now

		if err := w.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Failed to update transaction to success")
			return
		}

		go w.callbackSvc.SendCallback(trx, "transaction.success")
		log.Info().
			Str("transaction_id", trx.TransactionID).
			Str("provider_code", *trx.ProviderCode).
			Msg("Transaction updated to Success from multi-provider status check")

	case result.Pending:
		if err := w.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Failed to refresh pending transaction trace")
			return
		}
		// Still pending, will check again on next run
		log.Debug().
			Str("transaction_id", trx.TransactionID).
			Str("provider_code", *trx.ProviderCode).
			Msg("Transaction still pending from multi-provider status check")

	default:
		// Failed
		msg := result.Message
		rc := result.RC
		if w.providerRetrier != nil && trx.Type == models.TrxTypePrepaid {
			retried, handled, err := w.providerRetrier.RetryWithNextProvider(ctx, trx, rc, msg)
			if err != nil {
				log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Failed to retry transaction with next provider")
				return
			}
			if handled {
				if retried != nil && retried.Status == models.StatusFailed {
					log.Info().
						Str("transaction_id", retried.TransactionID).
						Str("provider_code", providerCode(retried)).
						Str("rc", valueOrEmpty(retried.FailedCode)).
						Msg("Transaction finalized after prepaid provider fallback")
				}
				return
			}
		}
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
			Str("provider_code", *trx.ProviderCode).
			Str("rc", rc).
			Msg("Transaction updated to Failed from multi-provider status check")
	}
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (w *StatusCheckWorker) checkDigiflazzTransaction(ctx context.Context, trx *models.Transaction) {
	if w.digiProd == nil && w.digiDev == nil {
		log.Warn().
			Str("transaction_id", trx.TransactionID).
			Msg("Digiflazz clients not configured, skipping legacy status check")
		return
	}

	if trx.DigiRefID == nil || *trx.DigiRefID == "" {
		log.Error().
			Str("transaction_id", trx.TransactionID).
			Msg("Transaction has no DigiRefID, cannot check status")
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
