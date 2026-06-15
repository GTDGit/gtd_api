package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/utils"
)

// ----------------------------------------------------------------------------
// Validation whitelists, taken verbatim from the Nobu QRIS registration form.
// ----------------------------------------------------------------------------

// omzetCategories: KATEGORI USAHA BERDASARKAN OMZET.
var omzetCategories = map[string]bool{
	"UMI": true, // Usaha Mikro
	"UKE": true, // Usaha Kecil
	"UME": true, // Usaha Menengah
	"UBE": true, // Usaha Besar
	"URE": true, // Usaha Reguler
	"PSO": true, // Public Service Obligation
	"BLU": true, // Badan Layanan Umum
}

// riskCategories: KATEGORI USAHA BERDASARKAN RISK.
var riskCategories = map[string]bool{
	"Low":    true,
	"Medium": true,
	"High":   true,
}

var (
	nikPattern = regexp.MustCompile(`^\d{16}$`)
	mccPattern = regexp.MustCompile(`^\d{4}$`)
)

// QRISRegistrationServiceError carries an HTTP status for handler mapping.
type QRISRegistrationServiceError struct {
	HTTPStatus int
	Code       string
	Message    string
	Err        error
}

func (e *QRISRegistrationServiceError) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return e.Message + ": " + e.Err.Error()
}

func (e *QRISRegistrationServiceError) Unwrap() error { return e.Err }

func regErr(status int, code, msg string, err error) *QRISRegistrationServiceError {
	return &QRISRegistrationServiceError{HTTPStatus: status, Code: code, Message: msg, Err: err}
}

// QRISDocRequest is one document in a registration request (client base64).
type QRISDocRequest struct {
	DocType  string `json:"docType"`  // ktp | selfie_ktp | business_location | extra
	FileName string `json:"fileName"` // original name (optional)
	Content  string `json:"content"`  // base64 (raw or data-URI)
}

// QRISRegistrationRequest is the client-facing intake payload. It carries every
// field the Nobu Excel form requires plus the onboarding documents.
type QRISRegistrationRequest struct {
	RegistrationRef string `json:"registrationRef"` // optional; generated if blank (idempotency key)

	OwnerFullName string `json:"ownerFullName"`
	OwnerNIK      string `json:"ownerNik"`
	OwnerPhone    string `json:"ownerPhone"`
	Email         string `json:"email"`

	BusinessName     string `json:"businessName"`
	MCC              string `json:"mcc"`
	AddressStreet    string `json:"addressStreet"`
	AddressRT        string `json:"addressRt"`
	AddressRW        string `json:"addressRw"`
	AddressKelurahan string `json:"addressKelurahan"`
	AddressKecamatan string `json:"addressKecamatan"`
	City             string `json:"city"`
	PostalCode       string `json:"postalCode"`
	HasPhysicalStore *bool  `json:"hasPhysicalStore"`

	OmzetCategory string `json:"omzetCategory"`
	QRISType      string `json:"qrisType"` // statis only for now
	RiskCategory  string `json:"riskCategory"`

	Website              string `json:"website"`
	EstimatedSalesVolume *int64 `json:"estimatedSalesVolume"`
	EstimatedTxCount     *int   `json:"estimatedTxCount"`

	Documents []QRISDocRequest `json:"documents"`
}

// QRISDocCreator is the doc-service surface the registration service depends on.
type QRISDocCreator interface {
	CreateBundle(ctx context.Context, merchantName string, qrisMerchantID *int, createdBy string, docs []QRISDocInput) (*models.QRISDocBundle, []models.QRISDocFile, error)
}

// QRISStaticQRGenerator abstracts the Nobu outbound generate call so the service
// stays free of the pkg/nobu import (an adapter is wired in main.go). It returns
// the EMVCo QR string for a provisioned merchant.
type QRISStaticQRGenerator interface {
	GenerateStaticQR(ctx context.Context, partnerReferenceNo, subMerchantID, storeID, terminalID, merchantName string) (qrString string, raw []byte, err error)
}

// QRISActivationCallback is the webhook-enqueue surface (implemented by
// QRISCallbackService) used to notify a client that its merchant is live.
type QRISActivationCallback interface {
	Enqueue(ctx context.Context, clientID int, event string, merchantID, paymentID *int, data any)
}

// QRISDocPortalUploader uploads onboarding documents to the GTD file-delivery
// portal and returns a token-gated bundle URL (embedded later in the Nobu Excel
// batch). Implemented by an adapter over pkg/filesportal wired in main.go. The
// registration service stays free of the portal import. Best-effort: a failure
// must not fail the registration.
type QRISDocPortalUploader interface {
	Upload(ctx context.Context, businessName string, docs []QRISDocRequest) (bundleURL, token string, err error)
}

// QRISActivateInput carries the identifiers Nobu returns out-of-band (via the
// WhatsApp group) once it has provisioned a merchant from an Excel batch.
type QRISActivateInput struct {
	SubMerchantID string // Nobu MID
	StoreID       string // NMID (merchant identification key on webhooks)
	TerminalID    string // TID
	QRISString    string // optional manual paste; used as fallback / override
}

// QRISRegistrationService handles client static-QRIS registration intake. Nobu
// onboarding is via Excel batch (no register API), so this records the request
// as pending_batch; the batch worker renders it and activation happens later.
type QRISRegistrationService struct {
	repo         *repository.QRISRegistrationRepository
	merchantRepo *repository.QRISMerchantRepository
	docSvc       QRISDocCreator
	generator    QRISStaticQRGenerator
	callbackSvc  QRISActivationCallback
	portal       QRISDocPortalUploader
}

func NewQRISRegistrationService(repo *repository.QRISRegistrationRepository, docSvc QRISDocCreator) *QRISRegistrationService {
	return &QRISRegistrationService{repo: repo, docSvc: docSvc}
}

// WithActivation wires the dependencies needed for the admin activation path.
// Kept separate from the constructor so the client-facing registration flow can
// be built even when Nobu generate credentials are absent.
func (s *QRISRegistrationService) WithActivation(
	merchantRepo *repository.QRISMerchantRepository,
	generator QRISStaticQRGenerator,
	callbackSvc QRISActivationCallback,
) *QRISRegistrationService {
	s.merchantRepo = merchantRepo
	s.generator = generator
	s.callbackSvc = callbackSvc
	return s
}

// WithDocPortal wires the optional file-delivery portal uploader. When set, the
// intake path uploads documents to the portal (alongside the canonical S3 copy)
// and records the returned bundle URL/token on the registration.
func (s *QRISRegistrationService) WithDocPortal(portal QRISDocPortalUploader) *QRISRegistrationService {
	s.portal = portal
	return s
}

// Register validates the request, stores any documents, and inserts a
// pending_batch registration scoped to the client. Returns the persisted row.
func (s *QRISRegistrationService) Register(ctx context.Context, clientID int, req QRISRegistrationRequest) (*models.QRISRegistration, error) {
	reg, docs, err := s.validate(clientID, req)
	if err != nil {
		return nil, err
	}

	// Store documents first (if any) so the bundle id can be linked.
	if len(docs) > 0 {
		if s.docSvc == nil {
			return nil, regErr(http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "document storage is not configured", nil)
		}
		bundle, _, derr := s.docSvc.CreateBundle(ctx, reg.BusinessName, nil, "client:"+strconv.Itoa(clientID), docs)
		if derr != nil {
			var dse *QRISDocServiceError
			if errors.As(derr, &dse) {
				return nil, regErr(dse.HTTPStatus, "DOCUMENT_ERROR", dse.Message, dse.Err)
			}
			return nil, regErr(http.StatusInternalServerError, "DOCUMENT_ERROR", "failed to store documents", derr)
		}
		reg.DocBundleID = &bundle.ID
	}

	created, err := s.repo.Create(ctx, reg)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, regErr(http.StatusConflict, "DUPLICATE_REGISTRATION",
				"a registration with this registrationRef already exists", err)
		}
		return nil, regErr(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create registration", err)
	}

	// Best-effort: upload the documents to the file-delivery portal so the Nobu
	// Excel batch can embed a token-gated bundle link. The canonical copy already
	// lives in S3 (above); a portal failure must not fail the registration.
	if s.portal != nil && len(req.Documents) > 0 {
		bundleURL, token, perr := s.portal.Upload(ctx, created.BusinessName, req.Documents)
		if perr != nil {
			log.Warn().Err(perr).
				Int("registration_id", created.ID).
				Str("registration_ref", created.RegistrationRef).
				Msg("qris registration: files-portal upload failed; continuing without portal link")
		} else {
			if err := s.repo.SetDocPortal(ctx, created.ID, bundleURL, token); err != nil {
				log.Warn().Err(err).Int("registration_id", created.ID).
					Msg("qris registration: failed to persist portal link")
			} else {
				created.DocPortalURL = nilIfBlank(bundleURL)
				created.DocPortalToken = nilIfBlank(token)
			}
		}
	}

	log.Info().
		Int("client_id", clientID).
		Str("registration_ref", created.RegistrationRef).
		Str("business_name", created.BusinessName).
		Msg("qris registration created")
	return created, nil
}

// Get returns a registration scoped to the client.
func (s *QRISRegistrationService) Get(ctx context.Context, clientID int, ref string) (*models.QRISRegistration, error) {
	reg, err := s.repo.GetByRef(ctx, clientID, strings.TrimSpace(ref))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, regErr(http.StatusNotFound, "NOT_FOUND", "registration not found", nil)
		}
		return nil, regErr(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load registration", err)
	}
	return reg, nil
}

// List returns the client's registrations, newest first, with total count.
func (s *QRISRegistrationService) List(ctx context.Context, clientID, page, limit int, status string) ([]models.QRISRegistration, int, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	items, total, err := s.repo.List(ctx, repository.QRISRegistrationFilter{
		ClientID: &clientID,
		Status:   strings.TrimSpace(status),
		Limit:    limit,
		Offset:   (page - 1) * limit,
	})
	if err != nil {
		return nil, 0, regErr(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list registrations", err)
	}
	return items, total, nil
}

// AdminList returns all clients' registrations (admin view), newest first.
func (s *QRISRegistrationService) AdminList(ctx context.Context, page, limit int, status string) ([]models.QRISRegistration, int, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	items, total, err := s.repo.List(ctx, repository.QRISRegistrationFilter{
		Status: strings.TrimSpace(status),
		Limit:  limit,
		Offset: (page - 1) * limit,
	})
	if err != nil {
		return nil, 0, regErr(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list registrations", err)
	}
	return items, total, nil
}

// Activate provisions the qris_merchants row for a registration after Nobu has
// returned its identifiers (subMerchantId/storeId/terminalId), generates the
// static QR string (Nobu API, with manual-paste fallback), flips the
// registration to activated, and enqueues the qris.merchant.activated webhook.
// This is the admin path (no client scoping): regID is the registration PK.
func (s *QRISRegistrationService) Activate(ctx context.Context, regID int, in QRISActivateInput) (*models.QRISMerchant, error) {
	if s.merchantRepo == nil {
		return nil, regErr(http.StatusServiceUnavailable, "ACTIVATION_UNAVAILABLE", "merchant activation is not configured", nil)
	}
	storeID := strings.TrimSpace(in.StoreID)
	if storeID == "" {
		return nil, regErr(http.StatusBadRequest, "INVALID_REQUEST", "storeId (NMID) is required", nil)
	}

	reg, err := s.repo.GetByID(ctx, regID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, regErr(http.StatusNotFound, "NOT_FOUND", "registration not found", nil)
		}
		return nil, regErr(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load registration", err)
	}
	if reg.Status == models.QRISRegActivated && reg.QRISMerchantID != nil {
		// Idempotent: already activated — return the existing merchant.
		m, mErr := s.merchantRepo.GetByID(ctx, *reg.QRISMerchantID)
		if mErr == nil {
			return m, nil
		}
	}

	// Resolve the QR string: prefer Nobu generate, fall back to manual paste.
	qrString := strings.TrimSpace(in.QRISString)
	var rawProviderResp []byte
	if qrString == "" && s.generator != nil {
		partnerRef := reg.RegistrationRef
		qr, raw, gErr := s.generator.GenerateStaticQR(ctx, partnerRef, strings.TrimSpace(in.SubMerchantID), storeID, strings.TrimSpace(in.TerminalID), reg.BusinessName)
		if gErr != nil {
			log.Warn().Err(gErr).Int("registration_id", regID).
				Msg("qris activate: Nobu generate failed; manual paste required")
			return nil, regErr(http.StatusBadGateway, "GENERATE_FAILED",
				"Nobu QR generation failed; provide qrisString manually to activate", gErr)
		}
		qrString = strings.TrimSpace(qr)
		rawProviderResp = raw
	}
	if qrString == "" {
		return nil, regErr(http.StatusBadRequest, "QRIS_STRING_REQUIRED",
			"could not generate a QR string; provide qrisString manually", nil)
	}

	// Parse descriptive fields out of the EMVCo string (best-effort).
	var merchantName, merchantCity, mcc, nmid, terminalID *string
	if info, perr := utils.ParseQRIS(qrString); perr == nil {
		merchantName = nilIfBlank(info.MerchantName)
		merchantCity = nilIfBlank(info.MerchantCity)
		mcc = nilIfBlank(info.MerchantCategoryCode)
		nmid = nilIfBlank(info.NMID)
		terminalID = nilIfBlank(info.TerminalID)
	}
	// storeId/terminalId from Nobu take precedence as the identification keys.
	if nmid == nil {
		nmid = nilIfBlank(storeID)
	}
	if tid := strings.TrimSpace(in.TerminalID); tid != "" {
		terminalID = &tid
	}
	if merchantName == nil {
		merchantName = nilIfBlank(reg.BusinessName)
	}

	merchant := &models.QRISMerchant{
		ClientID:             reg.ClientID,
		Provider:             models.QRISProviderNobu,
		MerchantName:         merchantName,
		MerchantCity:         merchantCity,
		MerchantCategoryCode: mcc,
		NMID:                 nmid,
		StoreID:              storeID,
		TerminalID:           terminalID,
		QRISString:           &qrString,
		Status:               models.QRISMerchantActive,
		SubMerchantID:        nilIfBlank(in.SubMerchantID),
		RegistrationID:       &reg.ID,
		RawProviderResponse:  models.NullableRawMessage(rawProviderResp),
	}
	created, err := s.merchantRepo.Create(ctx, merchant)
	if err != nil {
		return nil, regErr(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create merchant", err)
	}

	if err := s.repo.Activate(ctx, reg.ID, created.ID); err != nil {
		log.Warn().Err(err).Int("registration_id", reg.ID).Int("merchant_id", created.ID).
			Msg("qris activate: merchant created but registration update failed")
	}

	if s.callbackSvc != nil && reg.ClientID != nil {
		mid := created.ID
		s.callbackSvc.Enqueue(ctx, *reg.ClientID, models.QRISEventMerchantActivated, &mid, nil, created)
	}

	log.Info().
		Int("registration_id", reg.ID).
		Int("merchant_id", created.ID).
		Str("store_id", storeID).
		Msg("qris merchant activated")
	return created, nil
}

// validate enforces the Nobu form rules and builds the model + decoded docs.
func (s *QRISRegistrationService) validate(clientID int, req QRISRegistrationRequest) (*models.QRISRegistration, []QRISDocInput, error) {
	bad := func(msg string) (*models.QRISRegistration, []QRISDocInput, error) {
		return nil, nil, regErr(http.StatusBadRequest, "INVALID_REQUEST", msg, nil)
	}

	ownerName := strings.TrimSpace(req.OwnerFullName)
	if ownerName == "" {
		return bad("ownerFullName is required")
	}
	nik := strings.TrimSpace(req.OwnerNIK)
	if !nikPattern.MatchString(nik) {
		return bad("ownerNik must be exactly 16 digits")
	}
	phone := strings.TrimSpace(req.OwnerPhone)
	if phone == "" {
		return bad("ownerPhone is required")
	}
	email := strings.TrimSpace(req.Email)
	if email == "" || !strings.Contains(email, "@") {
		return bad("a valid email is required")
	}

	businessName := strings.TrimSpace(req.BusinessName)
	if businessName == "" {
		return bad("businessName is required")
	}
	if len([]rune(businessName)) > 25 {
		return bad("businessName must be at most 25 characters")
	}
	businessName = strings.ToUpper(businessName) // form requires capitalised

	mcc := strings.TrimSpace(req.MCC)
	if !mccPattern.MatchString(mcc) {
		return bad("mcc must be a 4-digit code")
	}

	street := strings.TrimSpace(req.AddressStreet)
	if street == "" {
		return bad("addressStreet is required")
	}
	city := strings.TrimSpace(req.City)
	if city == "" {
		return bad("city is required")
	}

	omzet := strings.ToUpper(strings.TrimSpace(req.OmzetCategory))
	if !omzetCategories[omzet] {
		return bad("omzetCategory must be one of UMI, UKE, UME, UBE, URE, PSO, BLU")
	}

	qrisType := strings.ToLower(strings.TrimSpace(req.QRISType))
	if qrisType == "" {
		qrisType = "statis"
	}
	if qrisType != "statis" {
		return bad("qrisType must be 'statis' (only static QRIS is supported)")
	}

	risk := normalizeRisk(req.RiskCategory)
	if !riskCategories[risk] {
		return bad("riskCategory must be one of Low, Medium, High")
	}

	hasStore := true
	if req.HasPhysicalStore != nil {
		hasStore = *req.HasPhysicalStore
	}

	ref := strings.TrimSpace(req.RegistrationRef)
	if ref == "" {
		ref = fmt.Sprintf("QRISREG-%d-%d", clientID, time.Now().UnixNano())
	}

	reg := &models.QRISRegistration{
		ClientID:             &clientID,
		RegistrationRef:      ref,
		OwnerFullName:        ownerName,
		OwnerNIK:             nik,
		OwnerPhone:           phone,
		Email:                email,
		BusinessName:         businessName,
		MCC:                  mcc,
		AddressStreet:        street,
		AddressRT:            nilIfBlank(req.AddressRT),
		AddressRW:            nilIfBlank(req.AddressRW),
		AddressKelurahan:     nilIfBlank(req.AddressKelurahan),
		AddressKecamatan:     nilIfBlank(req.AddressKecamatan),
		City:                 city,
		PostalCode:           nilIfBlank(req.PostalCode),
		HasPhysicalStore:     hasStore,
		OmzetCategory:        omzet,
		QRISType:             qrisType,
		RiskCategory:         risk,
		Website:              nilIfBlank(req.Website),
		EstimatedSalesVolume: req.EstimatedSalesVolume,
		EstimatedTxCount:     req.EstimatedTxCount,
		Status:               models.QRISRegPendingBatch,
	}

	docs := make([]QRISDocInput, 0, len(req.Documents))
	for i, d := range req.Documents {
		if strings.TrimSpace(d.Content) == "" {
			return nil, nil, regErr(http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("document %d: content (base64) is required", i+1), nil)
		}
		docs = append(docs, QRISDocInput{
			DocType:  normalizeDocType(d.DocType),
			FileName: strings.TrimSpace(d.FileName),
			Base64:   d.Content,
		})
	}

	return reg, docs, nil
}

func normalizeRisk(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSuffix(s, " risk")
	switch s {
	case "low":
		return "Low"
	case "medium", "med":
		return "Medium"
	case "high":
		return "High"
	default:
		return strings.TrimSpace(s)
	}
}

func normalizeDocType(s string) models.QRISDocType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ktp":
		return models.QRISDocKTP
	case "selfie_ktp", "selfie", "selfie-ktp":
		return models.QRISDocSelfieKTP
	case "business_location", "business-location", "location", "store":
		return models.QRISDocBusinessLocation
	default:
		return models.QRISDocExtra
	}
}
