package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/service"
)

type TransferStatusWorker struct {
	transferService *service.TransferService
	interval        time.Duration
	staleAfter      time.Duration
	maxAge          time.Duration
	batchSize       int
}

func NewTransferStatusWorker(
	transferService *service.TransferService,
	interval time.Duration,
	staleAfter time.Duration,
	maxAge time.Duration,
	batchSize int,
) *TransferStatusWorker {
	return &TransferStatusWorker{
		transferService: transferService,
		interval:        interval,
		staleAfter:      staleAfter,
		maxAge:          maxAge,
		batchSize:       batchSize,
	}
}

func (w *TransferStatusWorker) Start(ctx context.Context) {
	if w.transferService == nil {
		return
	}

	log.Info().
		Dur("interval", w.interval).
		Dur("stale_after", w.staleAfter).
		Dur("max_age", w.maxAge).
		Int("batch_size", w.batchSize).
		Msg("starting transfer status worker")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.run(ctx)
		case <-ctx.Done():
			log.Info().Msg("transfer status worker stopped")
			return
		}
	}
}

func (w *TransferStatusWorker) run(ctx context.Context) {
	if !w.transferService.Available() {
		return
	}

	if err := w.transferService.ProcessPendingTransfers(ctx, w.staleAfter, w.maxAge, w.batchSize); err != nil && err != context.Canceled {
		log.Error().Err(err).Msg("failed to process pending transfers")
	}
	if err := w.transferService.RetryPendingCallbacks(ctx, w.batchSize); err != nil && err != context.Canceled {
		log.Error().Err(err).Msg("failed to retry pending transfer callbacks")
	}
}
