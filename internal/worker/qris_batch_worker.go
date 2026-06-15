package worker

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/service"
)

// qrisSlot is one configured daily batch time, in WIB.
type qrisSlot struct {
	seq    int // 1-based slot sequence (1 = first batch of the day)
	hour   int
	minute int
}

// QRISBatchWorker renders the Nobu registration Excel on a daily schedule. It
// fires at each configured WIB slot (default 10:00 & 15:00) on weekdays only.
// Weekend registrations are not lost — they stay pending and fold into Monday's
// first batch, because generation simply skips Saturday/Sunday.
//
// The worker ticks once a minute and, for every slot whose time has already
// passed today, asks the batch service to generate that slot. Generation is
// idempotent per (date, seq) via a unique DB constraint, so re-attempts after a
// restart or a missed tick are safe no-ops.
type QRISBatchWorker struct {
	batchSvc *service.QRISBatchService
	loc      *time.Location
	slots    []qrisSlot
}

// NewQRISBatchWorker builds the worker. batchTimes are "HH:MM" strings in tz
// order; tz is an IANA name (default Asia/Jakarta on parse failure).
func NewQRISBatchWorker(batchSvc *service.QRISBatchService, batchTimes []string, tz string) *QRISBatchWorker {
	loc, err := time.LoadLocation(strings.TrimSpace(tz))
	if err != nil || loc == nil {
		log.Warn().Str("tz", tz).Msg("qris batch worker: invalid timezone; defaulting to Asia/Jakarta")
		loc, err = time.LoadLocation("Asia/Jakarta")
		if err != nil {
			loc = time.FixedZone("WIB", 7*3600)
		}
	}

	slots := make([]qrisSlot, 0, len(batchTimes))
	for i, t := range batchTimes {
		h, m, ok := parseHHMM(t)
		if !ok {
			log.Warn().Str("time", t).Msg("qris batch worker: skipping invalid batch time")
			continue
		}
		slots = append(slots, qrisSlot{seq: i + 1, hour: h, minute: m})
	}

	return &QRISBatchWorker{batchSvc: batchSvc, loc: loc, slots: slots}
}

func (w *QRISBatchWorker) Start(ctx context.Context) {
	if w.batchSvc == nil || len(w.slots) == 0 {
		log.Info().Msg("qris batch worker: disabled (no service or no valid slots)")
		return
	}
	log.Info().Int("slots", len(w.slots)).Str("tz", w.loc.String()).Msg("starting qris batch worker")

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Run once on startup to catch up any slot already passed today.
	w.runDue(ctx)
	for {
		select {
		case <-ticker.C:
			w.runDue(ctx)
		case <-ctx.Done():
			log.Info().Msg("qris batch worker stopped")
			return
		}
	}
}

// runDue generates every slot whose WIB time has already elapsed today, on
// weekdays only. Idempotency lives in the batch service.
func (w *QRISBatchWorker) runDue(ctx context.Context) {
	now := time.Now().In(w.loc)
	if wd := now.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return // weekend pending registrations fold into Monday's first batch
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, w.loc)
	for _, s := range w.slots {
		slotTime := time.Date(now.Year(), now.Month(), now.Day(), s.hour, s.minute, 0, 0, w.loc)
		if now.Before(slotTime) {
			continue // not yet time for this slot today
		}
		if _, err := w.batchSvc.GenerateBatch(ctx, today, s.seq); err != nil && err != context.Canceled {
			log.Error().Err(err).Int("seq", s.seq).Msg("qris batch worker: generate failed")
		}
	}
}

// parseHHMM parses "HH:MM" into hour/minute with range checks.
func parseHHMM(s string) (int, int, bool) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}
