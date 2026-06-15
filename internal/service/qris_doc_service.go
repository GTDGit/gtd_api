package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/storage"
)

// maxDocBytes caps a single decoded document at 5 MiB. Onboarding photos/scans
// (KTP, selfie, business location) are well under this; the limit guards against
// oversized base64 payloads inflating memory.
const maxDocBytes = 5 << 20

// allowedDocMIME is the whitelist of accepted document content types. Nobu
// onboarding accepts photos and scanned PDFs only.
var allowedDocMIME = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"application/pdf": true,
}

// QRISDocInput is one document supplied by the client as base64.
type QRISDocInput struct {
	DocType  models.QRISDocType
	FileName string
	Base64   string // raw or data-URI base64 of the file bytes
}

// QRISDocServiceError carries an HTTP status for handler mapping.
type QRISDocServiceError struct {
	HTTPStatus int
	Message    string
	Err        error
}

func (e *QRISDocServiceError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

func (e *QRISDocServiceError) Unwrap() error { return e.Err }

func docErr(status int, msg string, err error) *QRISDocServiceError {
	return &QRISDocServiceError{HTTPStatus: status, Message: msg, Err: err}
}

// qrisDocRepository is the persistence contract the doc service needs.
type qrisDocRepository interface {
	CreateBundle(ctx context.Context, b *models.QRISDocBundle) (*models.QRISDocBundle, error)
	AddFile(ctx context.Context, f *models.QRISDocFile) (*models.QRISDocFile, error)
}

// QRISDocService decodes client-supplied base64 documents, validates them, and
// stores the bytes in private object storage with a bundle/file metadata trail.
// The same bundle later powers the files-qris portal (token-gated delivery).
type QRISDocService struct {
	repo      qrisDocRepository
	store     storage.Storage
	keyPrefix string
}

func NewQRISDocService(repo qrisDocRepository, store storage.Storage, keyPrefix string) *QRISDocService {
	return &QRISDocService{repo: repo, store: store, keyPrefix: strings.TrimSpace(keyPrefix)}
}

// CreateBundle decodes + validates every document, writes them to storage, and
// records a bundle with its files. merchantName labels the bundle in the portal;
// qrisMerchantID optionally links it to an existing merchant (nil at intake).
func (s *QRISDocService) CreateBundle(
	ctx context.Context,
	merchantName string,
	qrisMerchantID *int,
	createdBy string,
	docs []QRISDocInput,
) (*models.QRISDocBundle, []models.QRISDocFile, error) {
	if s.store == nil {
		return nil, nil, docErr(http.StatusServiceUnavailable, "document storage is not configured", nil)
	}
	if len(docs) == 0 {
		return nil, nil, docErr(http.StatusBadRequest, "at least one document is required", nil)
	}

	// Decode + validate all documents BEFORE writing anything, so a bad input
	// never leaves a half-populated bundle.
	type decoded struct {
		doc         QRISDocInput
		data        []byte
		contentType string
		checksum    string
	}
	prepared := make([]decoded, 0, len(docs))
	for i, d := range docs {
		raw, err := decodeBase64Document(d.Base64)
		if err != nil {
			return nil, nil, docErr(http.StatusBadRequest, fmt.Sprintf("document %d: invalid base64", i+1), err)
		}
		if len(raw) == 0 {
			return nil, nil, docErr(http.StatusBadRequest, fmt.Sprintf("document %d: empty file", i+1), nil)
		}
		if len(raw) > maxDocBytes {
			return nil, nil, docErr(http.StatusRequestEntityTooLarge, fmt.Sprintf("document %d exceeds %d bytes", i+1, maxDocBytes), nil)
		}
		ct := http.DetectContentType(raw)
		// DetectContentType may append charset; compare on the media type only.
		mediaType := strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
		if !allowedDocMIME[mediaType] {
			return nil, nil, docErr(http.StatusUnsupportedMediaType,
				fmt.Sprintf("document %d: unsupported type %q (allowed: jpeg, png, pdf)", i+1, mediaType), nil)
		}
		sum := sha256.Sum256(raw)
		prepared = append(prepared, decoded{
			doc:         d,
			data:        raw,
			contentType: mediaType,
			checksum:    hex.EncodeToString(sum[:]),
		})
	}

	// Create the bundle row (DB assigns token + id).
	bundle, err := s.repo.CreateBundle(ctx, &models.QRISDocBundle{
		MerchantName:   merchantName,
		QRISMerchantID: qrisMerchantID,
		Status:         models.QRISDocBundleActive,
		CreatedBy:      strPtrOrNil(createdBy),
	})
	if err != nil {
		return nil, nil, docErr(http.StatusInternalServerError, "failed to create document bundle", err)
	}

	files := make([]models.QRISDocFile, 0, len(prepared))
	for i, p := range prepared {
		docType := p.doc.DocType
		if docType == "" {
			docType = models.QRISDocExtra
		}
		key := s.storageKey(bundle.Token, i, docType)
		if err := s.store.Put(ctx, key, p.contentType, p.data); err != nil {
			return nil, nil, docErr(http.StatusInternalServerError, "failed to store document", err)
		}
		checksum := p.checksum
		fileRow, err := s.repo.AddFile(ctx, &models.QRISDocFile{
			BundleID:    bundle.ID,
			DocType:     docType,
			FileName:    sanitizeFileName(p.doc.FileName, docType, p.contentType),
			ContentType: p.contentType,
			SizeBytes:   int64(len(p.data)),
			StorageKey:  key,
			Checksum:    &checksum,
		})
		if err != nil {
			return nil, nil, docErr(http.StatusInternalServerError, "failed to record document", err)
		}
		files = append(files, *fileRow)
	}

	return bundle, files, nil
}

// storageKey builds the private object key for one document.
func (s *QRISDocService) storageKey(bundleToken string, index int, docType models.QRISDocType) string {
	prefix := s.keyPrefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return fmt.Sprintf("%sdocs/%s/%d_%s", prefix, bundleToken, index+1, docType)
}

// decodeBase64Document accepts a bare base64 string or a data URI
// ("data:image/png;base64,...."), tolerating whitespace and missing padding.
func decodeBase64Document(in string) ([]byte, error) {
	s := strings.TrimSpace(in)
	if s == "" {
		return nil, fmt.Errorf("empty base64")
	}
	if strings.HasPrefix(s, "data:") {
		if idx := strings.Index(s, ","); idx >= 0 {
			s = s[idx+1:]
		}
	}
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	// Try standard, then URL-safe, with/without padding.
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("not valid base64")
}

func sanitizeFileName(name string, docType models.QRISDocType, contentType string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		// Keep the basename only; drop any path components.
		if i := strings.LastIndexAny(name, "/\\"); i >= 0 {
			name = name[i+1:]
		}
		if name != "" {
			return name
		}
	}
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "application/pdf": ".pdf"}[contentType]
	return string(docType) + ext
}

func strPtrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}
