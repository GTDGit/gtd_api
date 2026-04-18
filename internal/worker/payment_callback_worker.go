package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/service"
)

// PaymentCallbackWorker retries pending outbound client webhook deliveries.
type PaymentCallbackWorker struct {
	callbackSvc *service.PaymentCallbackService
	interval    time.Duration
	batchSize   int
}

func NewPaymentCallbackWorker(callbackSvc *service.PaymentCallbackService, interval time.Duration, batchSize int) *PaymentCallbackWorker {
	return &PaymentCallbackWorker{
		callbackSvc: callbackSvc,
		interval:    interval,
		batchSize:   batchSize,
	}
}

func (w *PaymentCallbackWorker) Start(ctx context.Context) {
	if w.callbackSvc == nil {
		return
	}
	log.Info().
		Dur("interval", w.interval).
		Int("batch_size", w.batchSize).
		Msg("starting payment callback worker")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := w.callbackSvc.RetryPendingCallbacks(ctx, w.batchSize); err != nil && err != context.Canceled {
				log.Error().Err(err).Msg("payment callback worker tick failed")
			}
		case <-ctx.Done():
			log.Info().Msg("payment callback worker stopped")
			return
		}
	}
}
