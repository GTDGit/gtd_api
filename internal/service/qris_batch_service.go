package service

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/storage"
)

// nobuSheetName is the worksheet name in the rendered batch file. It mirrors the
// official Nobu form's primary sheet.
const nobuSheetName = "Formulir Pendaftaran NOBU QRIS"

// nobuColumns are the headers of the Nobu registration form, in order. The
// official template is a heavily merged spreadsheet with dropdown source sheets;
// we emit a clean one-row-per-merchant sheet carrying the same fields, which is
// the data Nobu consumes. An operator can paste these rows into the branded
// template if Nobu requires the exact workbook.
var nobuColumns = []string{
	"NO",
	"NAMA LENGKAP PEMILIK USAHA (sesuai e-KTP)",
	"NIK E-KTP PEMILIK USAHA (16 Digit)",
	"NO HANDPHONE PEMILIK USAHA (WhatsApp)",
	"ALAMAT EMAIL (Perusahaan/PIC)",
	"NAMA USAHA (Maks. 25 karakter & Kapital)",
	"MCC - JENIS USAHA",
	"ALAMAT USAHA (jalan, no, RT/RW, kelurahan, kecamatan)",
	"KOTA / KABUPATEN",
	"KODE POS",
	"APAKAH MEMILIKI TOKO FISIK ? (Ya/Tidak)",
	"KATEGORI USAHA BERDASARKAN OMZET",
	"TIPE QRIS (Dinamis/statis/booth)",
	"WEBSITE",
	"KATEGORI USAHA BERDASARKAN RISK",
	"ESTIMASI SALES VOLUME",
	"ESTIMASI TRANSAKSI",
	"LINK DOKUMEN",
}

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

	periodLabel := fmt.Sprintf("%s Batch %d", day, seq)
	fileBytes, err := s.renderExcel(pending, periodLabel)
	if err != nil {
		return nil, fmt.Errorf("qris batch: render excel: %w", err)
	}

	fileName := fmt.Sprintf("nobu-qris-batch-%s-b%d.xlsx", day, seq)
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

// renderExcel builds the workbook bytes for the given registrations.
func (s *QRISBatchService) renderExcel(regs []models.QRISRegistration, periodLabel string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	idx, err := f.NewSheet(nobuSheetName)
	if err != nil {
		return nil, err
	}
	f.SetActiveSheet(idx)
	_ = f.DeleteSheet("Sheet1")

	// Title + period rows.
	_ = f.SetCellValue(nobuSheetName, "A1", "FORMULIR PENDAFTARAN NOBU QRIS")
	_ = f.SetCellValue(nobuSheetName, "A2", "Periode Pendaftaran: "+periodLabel)

	// Header row at row 4.
	const headerRow = 4
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"D9E1F2"}},
	})
	for i, h := range nobuColumns {
		cell, _ := excelize.CoordinatesToCellName(i+1, headerRow)
		_ = f.SetCellValue(nobuSheetName, cell, h)
		_ = f.SetCellStyle(nobuSheetName, cell, cell, headerStyle)
	}

	// Data rows.
	for ri, r := range regs {
		row := headerRow + 1 + ri
		vals := []any{
			ri + 1,
			r.OwnerFullName,
			// Force NIK as text so leading zeros / 16 digits survive.
			r.OwnerNIK,
			r.OwnerPhone,
			r.Email,
			r.BusinessName,
			r.MCC,
			composeNobuAddress(r),
			r.City,
			derefStr(r.PostalCode),
			boolToYaTidak(r.HasPhysicalStore),
			r.OmzetCategory,
			r.QRISType,
			derefStr(r.Website),
			r.RiskCategory + " Risk",
			int64OrEmpty(r.EstimatedSalesVolume),
			intOrEmpty(r.EstimatedTxCount),
			derefStr(r.DocPortalURL),
		}
		for ci, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(ci+1, row)
			_ = f.SetCellValue(nobuSheetName, cell, v)
		}
		// NIK + phone as text to preserve digits.
		nikCell, _ := excelize.CoordinatesToCellName(3, row)
		_ = f.SetCellStr(nobuSheetName, nikCell, r.OwnerNIK)
		phoneCell, _ := excelize.CoordinatesToCellName(4, row)
		_ = f.SetCellStr(nobuSheetName, phoneCell, r.OwnerPhone)
	}

	// Reasonable column widths.
	_ = f.SetColWidth(nobuSheetName, "A", "A", 5)
	_ = f.SetColWidth(nobuSheetName, "B", "B", 28)
	_ = f.SetColWidth(nobuSheetName, "C", "E", 24)
	_ = f.SetColWidth(nobuSheetName, "F", "H", 30)
	_ = f.SetColWidth(nobuSheetName, "I", "Q", 18)
	_ = f.SetColWidth(nobuSheetName, "R", "R", 48)

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

// composeNobuAddress folds the structured address parts into the single
// free-text address cell the Nobu form expects.
func composeNobuAddress(r models.QRISRegistration) string {
	parts := []string{r.AddressStreet}
	if rt := derefStr(r.AddressRT); rt != "" {
		rw := derefStr(r.AddressRW)
		if rw != "" {
			parts = append(parts, "RT/RW "+rt+"/"+rw)
		} else {
			parts = append(parts, "RT "+rt)
		}
	}
	if kel := derefStr(r.AddressKelurahan); kel != "" {
		parts = append(parts, "Kel. "+kel)
	}
	if kec := derefStr(r.AddressKecamatan); kec != "" {
		parts = append(parts, "Kec. "+kec)
	}
	out := ""
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i > 0 && out != "" {
			out += ", "
		}
		out += p
	}
	return out
}

func boolToYaTidak(b bool) string {
	if b {
		return "Ya"
	}
	return "Tidak"
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func int64OrEmpty(v *int64) any {
	if v == nil {
		return ""
	}
	return *v
}

func intOrEmpty(v *int) any {
	if v == nil {
		return ""
	}
	return *v
}
