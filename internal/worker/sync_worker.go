package worker

import (
    "context"
    "time"

    "github.com/rs/zerolog/log"

    "github.com/GTDGit/gtd_api/internal/service"
)

// SyncWorker periodically syncs pricelists from Digiflazz.
type SyncWorker struct {
    syncService *service.SyncService
    interval    time.Duration
}

// NewSyncWorker constructs a SyncWorker.
func NewSyncWorker(syncService *service.SyncService, interval time.Duration) *SyncWorker {
    return &SyncWorker{
        syncService: syncService,
        interval:    interval,
    }
}

// Start begins the periodic sync loop and listens for context cancellation.
func (w *SyncWorker) Start(ctx context.Context) {
    log.Info().Dur("interval", w.interval).Msg("Starting sync worker")

    // Run immediately on start
    w.run(ctx)

    ticker := time.NewTicker(w.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            w.run(ctx)
        case <-ctx.Done():
            log.Info().Msg("Sync worker stopped")
            return
        }
    }
}

func (w *SyncWorker) run(ctx context.Context) {
    log.Info().Msg("Syncing pricelist from Digiflazz...")

    start := time.Now()
    if err := w.syncService.SyncPricelist(ctx); err != nil {
        log.Error().Err(err).Msg("Failed to sync pricelist")
        return
    }

    log.Info().Dur("duration", time.Since(start)).Msg("Pricelist sync completed")
}
