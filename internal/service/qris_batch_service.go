package service

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/storage"
)

// nobuSheetName is the data worksheet inside the official Nobu template. We fill
// rows into this sheet rather than rendering our own, so the workbook Nobu
// receives is byte-for-byte their branded form (dropdowns, MCC/Kode Pos lookup
// sheets, merged header) with only the merchant rows added.
const nobuSheetName = "Formulir Pendaftaran NOBU QRIS"

// nobuDataStartRow is the first merchant row in the template (the header occupies
// rows 9-10, with the address sub-headers on row 10).
const nobuDataStartRow = 11

// nobuTemplateBytes is the official Nobu registration workbook, embedded so the
// rendered batch is the real form rather than a hand-built sheet. Sourced from
// docs/qris/nobu and copied verbatim into templates/.
//
//go:embed templates/nobu_qris_form.xlsx
var nobuTemplateBytes []byte

// QRISBatchService renders pending registrations into a Nobu-format Excel file,
// persists the file to object storage, records a qris_nobu_batches row, and
// flips the included registrations to "submitted" — all idempotently per slot.
type QRISBatchService struct {
	regRepo   *repository.QRISRegistrationRepository
	batchRepo *repository.QRISBatchRepository
	store     storage.Storage
	keyPrefix string
}

func NewQRISBatchService(
	regRepo *repository.QRISRegistrationRepository,
	batchRepo *repository.QRISBatchRepository,
	store storage.Storage,
	keyPrefix string,
) *QRISBatchService {
	return &QRISBatchService{regRepo: regRepo, batchRepo: batchRepo, store: store, keyPrefix: keyPrefix}
}

// GenerateBatch builds the Excel for one slot (date + seq) from all pending
// registrations. It is idempotent: if a batch already exists for the slot it
// returns that batch without regenerating. If there are no pending
// registrations it returns (nil, nil) — no empty file is produced.
func (s *QRISBatchService) GenerateBatch(ctx context.Context, batchDate time.Time, seq int) (*models.QRISNobuBatch, error) {
	if s.store == nil {
		return nil, fmt.Errorf("qris batch: storage not configured")
	}

	day := batchDate.Format("2006-01-02")
	if existing, err := s.batchRepo.GetBySlot(ctx, day, seq); err == nil && existing != nil {
		log.Info().Str("date", day).Int("seq", seq).Msg("qris batch already exists for slot; skipping")
		return existing, nil
	}

	pending, err := s.regRepo.ListPendingForBatch(ctx)
	if err != nil {
		return nil, fmt.Errorf("qris batch: list pending: %w", err)
	}
	if len(pending) == 0 {
		log.Info().Str("date", day).Int("seq", seq).Msg("qris batch: no pending registrations; nothing to render")
		return nil, nil
	}

	periodLabel := fmt.Sprintf("%s Batch %d", formatNobuDate(batchDate), seq)
	fileBytes, err := s.renderExcel(pending, periodLabel)
	if err != nil {
		return nil, fmt.Errorf("qris batch: render excel: %w", err)
	}

	// File name mirrors the official Nobu template's naming convention.
	fileName := fmt.Sprintf("Formulir Pendaftaran NOBU QRIS (NMID Level) - Batch %d - Periode %s.xlsx", seq, formatNobuDate(batchDate))
	storageKey := s.batchKey(day, seq, fileName)
	const xlsxMIME = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	if err := s.store.Put(ctx, storageKey, xlsxMIME, fileBytes); err != nil {
		return nil, fmt.Errorf("qris batch: store file: %w", err)
	}

	batch, err := s.batchRepo.Create(ctx, &models.QRISNobuBatch{
		BatchDate:         batchDate,
		BatchSeq:          seq,
		PeriodLabel:       &periodLabel,
		FileStorageKey:    storageKey,
		FileName:          fileName,
		RegistrationCount: len(pending),
		Status:            models.QRISBatchGenerated,
	})
	if err != nil {
		// A unique-violation means a concurrent run won the slot; fetch + reuse.
		if isUniqueViolation(err) {
			if existing, gerr := s.batchRepo.GetBySlot(ctx, day, seq); gerr == nil && existing != nil {
				_ = s.store.Delete(ctx, storageKey)
				return existing, nil
			}
		}
		return nil, fmt.Errorf("qris batch: record batch: %w", err)
	}

	ids := make([]int, 0, len(pending))
	for _, r := range pending {
		ids = append(ids, r.ID)
	}
	if _, err := s.regRepo.MarkSubmitted(ctx, ids, batch.ID); err != nil {
		return nil, fmt.Errorf("qris batch: mark submitted: %w", err)
	}

	log.Info().
		Str("date", day).
		Int("seq", seq).
		Int("count", len(pending)).
		Str("file", fileName).
		Msg("qris batch generated")
	return batch, nil
}

// ListBatches returns rendered batches newest first (admin view).
func (s *QRISBatchService) ListBatches(ctx context.Context, page, limit int) ([]models.QRISNobuBatch, int, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.batchRepo.List(ctx, limit, (page-1)*limit)
}

// GetBatchFile returns the rendered Excel bytes + filename for download.
func (s *QRISBatchService) GetBatchFile(ctx context.Context, id int) ([]byte, string, error) {
	if s.store == nil {
		return nil, "", fmt.Errorf("qris batch: storage not configured")
	}
	batch, err := s.batchRepo.GetByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	data, _, err := s.store.Get(ctx, batch.FileStorageKey)
	if err != nil {
		return nil, "", err
	}
	return data, batch.FileName, nil
}

// MarkBatchSent flips a batch to "sent" after an operator delivers it to Nobu.
func (s *QRISBatchService) MarkBatchSent(ctx context.Context, id int) error {
	return s.batchRepo.MarkSent(ctx, id)
}

// renderExcel fills the official Nobu template (embedded) with one row per
// registration, starting at the template's first data row. The branded header,
// merged cells, MCC / Kode Pos dropdown lookup sheets, and styling are left
// untouched — Nobu receives their exact form with only the merchant rows added.
func (s *QRISBatchService) renderExcel(regs []models.QRISRegistration, periodLabel string) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(nobuTemplateBytes))
	if err != nil {
		return nil, fmt.Errorf("open nobu template: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.GetSheetIndex(nobuSheetName); err != nil {
		return nil, fmt.Errorf("nobu template: sheet %q not found: %w", nobuSheetName, err)
	}

	// Period metadata cell (merged target for "Periode Pendaftaran :").
	_ = f.SetCellStr(nobuSheetName, "D6", periodLabel)

	// One merchant row per registration, mapped onto the template's columns.
	// Address is split across I..M (jalan/RT/RW/Kelurahan/Kecamatan); the
	// token-gated document bundle link goes in the "KELENGKAPAN DOKUMEN USAHA"
	// column (U). Text columns (NIK, phone, MCC, postal) are written as strings
	// to preserve leading zeros and exact digits.
	for i, r := range regs {
		row := nobuDataStartRow + i
		setText := func(col, val string) {
			_ = f.SetCellStr(nobuSheetName, fmt.Sprintf("%s%d", col, row), val)
		}
		setNum := func(col string, v any) {
			_ = f.SetCellValue(nobuSheetName, fmt.Sprintf("%s%d", col, row), v)
		}

		setNum("B", i+1)
		setText("C", r.OwnerFullName)
		setText("D", r.OwnerNIK)
		setText("E", r.OwnerPhone)
		setText("F", r.Email)
		setText("G", r.BusinessName)
		setText("H", r.MCC)
		setText("I", r.AddressStreet)
		setText("J", derefStr(r.AddressRT))
		setText("K", derefStr(r.AddressRW))
		setText("L", derefStr(r.AddressKelurahan))
		setText("M", derefStr(r.AddressKecamatan))
		setText("N", r.City)
		setText("O", derefStr(r.PostalCode))
		setText("P", boolToYaTidak(r.HasPhysicalStore))
		setText("Q", r.OmzetCategory)
		setText("R", nobuQRISType(r.QRISType))
		setText("U", derefStr(r.DocPortalURL))
		setText("W", derefStr(r.Website))
		setText("Y", r.RiskCategory+" Risk")
		if r.EstimatedSalesVolume != nil {
			setNum("AA", *r.EstimatedSalesVolume)
		}
		if r.EstimatedTxCount != nil {
			setNum("AB", *r.EstimatedTxCount)
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *QRISBatchService) batchKey(day string, seq int, fileName string) string {
	prefix := s.keyPrefix
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	return prefix + "batches/" + day + "/b" + strconv.Itoa(seq) + "/" + fileName
}

func boolToYaTidak(b bool) string {
	if b {
		return "Ya"
	}
	return "Tidak"
}

// nobuMonths maps month number to its Indonesian name for the period label.
var nobuMonths = [...]string{
	"", "Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

// formatNobuDate renders a date as "15 Juni 2026" (WIB), matching the Nobu form.
func formatNobuDate(t time.Time) string {
	wib := time.FixedZone("WIB", 7*3600)
	t = t.In(wib)
	return fmt.Sprintf("%d %s %d", t.Day(), nobuMonths[t.Month()], t.Year())
}

// nobuQRISType maps the API enum (static|dynamic|both) onto the Nobu form's
// Indonesian labels in the "TIPE QRIS" column.
func nobuQRISType(t models.QRISType) string {
	switch t {
	case models.QRISTypeStatic:
		return "Statis"
	case models.QRISTypeDynamic:
		return "Dinamis"
	case models.QRISTypeBoth:
		return "Statis & Dinamis"
	default:
		return string(t)
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
