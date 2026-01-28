package worker

import (
    "context"
    "time"

    "github.com/rs/zerolog/log"

    "github.com/GTDGit/gtd_api/internal/models"
    "github.com/GTDGit/gtd_api/internal/repository"
    "github.com/GTDGit/gtd_api/internal/service"
)

// RetryWorker processes pending transactions and retries them periodically.
type RetryWorker struct {
    trxService *service.TransactionService
    trxRepo    *repository.TransactionRepository
    interval   time.Duration
}

// NewRetryWorker constructs a RetryWorker.
func NewRetryWorker(
    trxService *service.TransactionService,
    trxRepo *repository.TransactionRepository,
    interval time.Duration,
) *RetryWorker {
    return &RetryWorker{
        trxService: trxService,
        trxRepo:    trxRepo,
        interval:   interval,
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
    transactions, err := w.trxRepo.GetPendingForRetry()
    if err != nil {
        log.Error().Err(err).Msg("Failed to get pending transactions")
        return
    }
    if len(transactions) == 0 {
        return
    }
    log.Info().Int("count", len(transactions)).Msg("Processing pending transactions")

    for i := range transactions {
        // Respect cancellation between items
        select {
        case <-ctx.Done():
            return
        default:
            w.processTransaction(ctx, &transactions[i])
        }
    }
}

func (w *RetryWorker) processTransaction(ctx context.Context, trx *models.Transaction) {
    log.Info().
        Str("transaction_id", trx.TransactionID).
        Int("retry_count", trx.RetryCount).
        Msg("Retrying transaction")

    if _, err := w.trxService.RetryTransaction(ctx, trx); err != nil {
        log.Error().
            Err(err).
            Str("transaction_id", trx.TransactionID).
            Msg("Failed to retry transaction")
    }
}
