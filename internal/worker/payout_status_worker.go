package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/service"
)

// PayoutStatusWorker periodically reconciles pending payouts with their
// provider and retries undelivered client callbacks.
type PayoutStatusWorker struct {
	payoutService *service.PayoutService
	interval      time.Duration
	staleAfter    time.Duration
	maxAge        time.Duration
	batchSize     int
}

func NewPayoutStatusWorker(
	payoutService *service.PayoutService,
	interval time.Duration,
	staleAfter time.Duration,
	maxAge time.Duration,
	batchSize int,
) *PayoutStatusWorker {
	return &PayoutStatusWorker{
		payoutService: payoutService,
		interval:      interval,
		staleAfter:    staleAfter,
		maxAge:        maxAge,
		batchSize:     batchSize,
	}
}

func (w *PayoutStatusWorker) Start(ctx context.Context) {
	if w.payoutService == nil {
		return
	}

	log.Info().
		Dur("interval", w.interval).
		Dur("stale_after", w.staleAfter).
		Dur("max_age", w.maxAge).
		Int("batch_size", w.batchSize).
		Msg("starting payout status worker")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.run(ctx)
		case <-ctx.Done():
			log.Info().Msg("payout status worker stopped")
			return
		}
	}
}

func (w *PayoutStatusWorker) run(ctx context.Context) {
	if !w.payoutService.Available() {
		return
	}

	if err := w.payoutService.ProcessPendingPayouts(ctx, w.staleAfter, w.maxAge, w.batchSize); err != nil && err != context.Canceled {
		log.Error().Err(err).Msg("failed to process pending payouts")
	}
	if err := w.payoutService.RetryPendingCallbacks(ctx, w.batchSize); err != nil && err != context.Canceled {
		log.Error().Err(err).Msg("failed to retry pending payout callbacks")
	}
}
