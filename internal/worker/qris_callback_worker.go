package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/service"
)

// QRISCallbackWorker retries pending outbound QRIS client webhook deliveries
// (merchant.activated, payment.success).
type QRISCallbackWorker struct {
	callbackSvc *service.QRISCallbackService
	interval    time.Duration
	batchSize   int
}

func NewQRISCallbackWorker(callbackSvc *service.QRISCallbackService, interval time.Duration, batchSize int) *QRISCallbackWorker {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if batchSize <= 0 {
		batchSize = 50
	}
	return &QRISCallbackWorker{
		callbackSvc: callbackSvc,
		interval:    interval,
		batchSize:   batchSize,
	}
}

func (w *QRISCallbackWorker) Start(ctx context.Context) {
	if w.callbackSvc == nil {
		return
	}
	log.Info().
		Dur("interval", w.interval).
		Int("batch_size", w.batchSize).
		Msg("starting qris callback worker")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := w.callbackSvc.RetryDue(ctx, w.batchSize); err != nil && err != context.Canceled {
				log.Error().Err(err).Msg("qris callback worker tick failed")
			}
		case <-ctx.Done():
			log.Info().Msg("qris callback worker stopped")
			return
		}
	}
}
