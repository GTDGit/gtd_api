package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/service"
)

// PaymentStatusWorker periodically re-inquiries provider state for Pending
// payments whose updated_at has drifted beyond staleAfter.
type PaymentStatusWorker struct {
	paymentSvc *service.PaymentService
	interval   time.Duration
	staleAfter time.Duration
	batchSize  int
}

func NewPaymentStatusWorker(paymentSvc *service.PaymentService, interval, staleAfter time.Duration, batchSize int) *PaymentStatusWorker {
	return &PaymentStatusWorker{
		paymentSvc: paymentSvc,
		interval:   interval,
		staleAfter: staleAfter,
		batchSize:  batchSize,
	}
}

func (w *PaymentStatusWorker) Start(ctx context.Context) {
	if w.paymentSvc == nil {
		return
	}
	log.Info().
		Dur("interval", w.interval).
		Dur("stale_after", w.staleAfter).
		Int("batch_size", w.batchSize).
		Msg("starting payment status worker")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := w.paymentSvc.ProcessPendingPayments(ctx, w.staleAfter, w.batchSize); err != nil && err != context.Canceled {
				log.Error().Err(err).Msg("payment status worker tick failed")
			}
		case <-ctx.Done():
			log.Info().Msg("payment status worker stopped")
			return
		}
	}
}
