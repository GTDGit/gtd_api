package worker

import (
    "context"
    "time"

    "github.com/rs/zerolog/log"

    "github.com/GTDGit/gtd_api/internal/service"
)

// CallbackWorker retries failed callbacks on a fixed interval.
type CallbackWorker struct {
    callbackService *service.CallbackService
    interval        time.Duration
}

// NewCallbackWorker constructs a CallbackWorker.
func NewCallbackWorker(callbackService *service.CallbackService, interval time.Duration) *CallbackWorker {
    return &CallbackWorker{
        callbackService: callbackService,
        interval:        interval,
    }
}

// Start begins the retry loop and listens for context cancellation.
func (w *CallbackWorker) Start(ctx context.Context) {
    log.Info().Dur("interval", w.interval).Msg("Starting callback worker")

    ticker := time.NewTicker(w.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            w.run(ctx)
        case <-ctx.Done():
            log.Info().Msg("Callback worker stopped")
            return
        }
    }
}

func (w *CallbackWorker) run(ctx context.Context) {
    if err := w.callbackService.RetryPendingCallbacks(); err != nil {
        log.Error().Err(err).Msg("Failed to process pending callbacks")
    }
}
