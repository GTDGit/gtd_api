package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/service"
)

// PaymentExpiryWorker marks Pending payments as Expired once their
// expired_at is in the past.
type PaymentExpiryWorker struct {
	paymentSvc *service.PaymentService
	interval   time.Duration
	batchSize  int
}

func NewPaymentExpiryWorker(paymentSvc *service.PaymentService, interval time.Duration, batchSize int) *PaymentExpiryWorker {
	return &PaymentExpiryWorker{
		paymentSvc: paymentSvc,
		interval:   interval,
		batchSize:  batchSize,
	}
}

func (w *PaymentExpiryWorker) Start(ctx context.Context) {
	if w.paymentSvc == nil {
		return
	}
	log.Info().
		Dur("interval", w.interval).
		Int("batch_size", w.batchSize).
		Msg("starting payment expiry worker")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := w.paymentSvc.ExpirePendingPayments(ctx, w.batchSize); err != nil && err != context.Canceled {
				log.Error().Err(err).Msg("payment expiry worker tick failed")
			}
		case <-ctx.Done():
			log.Info().Msg("payment expiry worker stopped")
			return
		}
	}
}
