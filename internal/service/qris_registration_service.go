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

	"github.com/google/uuid"
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

// QRISOwner groups the merchant owner's identity fields (Nobu e-KTP section).
type QRISOwner struct {
	FullName string `json:"fullName"`
	NIK      string `json:"nik"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
}

// QRISAddress groups the merchant's address fields (Nobu ALAMAT USAHA section).
type QRISAddress struct {
	Street     string `json:"street"`
	RT         string `json:"rt"`
	RW         string `json:"rw"`
	Kelurahan  string `json:"kelurahan"`
	Kecamatan  string `json:"kecamatan"`
	City       string `json:"city"`
	PostalCode string `json:"postalCode"`
}

// QRISBusiness groups the business profile fields (name, MCC, classification,
// estimates) the Nobu form requires.
type QRISBusiness struct {
	Name                 string `json:"name"`
	MCC                  string `json:"mcc"`
	HasPhysicalStore     *bool  `json:"hasPhysicalStore"`
	OmzetCategory        string `json:"omzetCategory"`
	RiskCategory         string `json:"riskCategory"`
	Website              string `json:"website"`
	EstimatedSalesVolume *int64 `json:"estimatedSalesVolume"`
	EstimatedTxCount     *int   `json:"estimatedTxCount"`
}

// QRISRegistrationRequest is the client-facing intake payload. Fields are grouped
// into nested objects (owner / address / business) mirroring the Payment API
// style. referenceId and qrisType are mandatory.
type QRISRegistrationRequest struct {
	ReferenceID string `json:"referenceId"` // mandatory idempotency key (unique per client)
	QRISType    string `json:"qrisType"`    // mandatory: static | dynamic | both

	Owner    QRISOwner    `json:"owner"`
	Address  QRISAddress  `json:"address"`
	Business QRISBusiness `json:"business"`

	Documents []QRISDocRequest `json:"documents"`
}

// QRISRegistrationResponse is the client-facing representation of a registration.
// It mirrors the Payment API: a public UUID `id`, the mandatory `referenceId`,
// nested owner/address/business objects, and WIB (+07:00) timestamps. The same
// mapper feeds the qris.merchant.activated webhook so API and webhook are identical.
type QRISRegistrationResponse struct {
	ID          string `json:"id"`          // public UUID v4
	ReferenceID string `json:"referenceId"` // client idempotency key
	QRISType    string `json:"qrisType"`
	Status      string `json:"status"`

	Owner    QRISOwnerResponse    `json:"owner"`
	Address  QRISAddressResponse  `json:"address"`
	Business QRISBusinessResponse `json:"business"`

	QRISMerchantID *int   `json:"qrisMerchantId,omitempty"`
	Note           string `json:"note,omitempty"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type QRISOwnerResponse struct {
	FullName string `json:"fullName"`
	NIK      string `json:"nik"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
}

type QRISAddressResponse struct {
	Street     string `json:"street"`
	RT         string `json:"rt,omitempty"`
	RW         string `json:"rw,omitempty"`
	Kelurahan  string `json:"kelurahan,omitempty"`
	Kecamatan  string `json:"kecamatan,omitempty"`
	City       string `json:"city"`
	PostalCode string `json:"postalCode,omitempty"`
}

type QRISBusinessResponse struct {
	Name                 string `json:"name"`
	MCC                  string `json:"mcc"`
	HasPhysicalStore     bool   `json:"hasPhysicalStore"`
	OmzetCategory        string `json:"omzetCategory"`
	RiskCategory         string `json:"riskCategory"`
	Website              string `json:"website,omitempty"`
	EstimatedSalesVolume *int64 `json:"estimatedSalesVolume,omitempty"`
	EstimatedTxCount     *int   `json:"estimatedTxCount,omitempty"`
}

// ToQRISRegistrationResponse maps the stored model onto the client-facing DTO.
func ToQRISRegistrationResponse(reg *models.QRISRegistration) QRISRegistrationResponse {
	return QRISRegistrationResponse{
		ID:          reg.RegistrationID,
		ReferenceID: reg.RegistrationRef,
		QRISType:    string(reg.QRISType),
		Status:      string(reg.Status),
		Owner: QRISOwnerResponse{
			FullName: reg.OwnerFullName,
			NIK:      reg.OwnerNIK,
			Phone:    reg.OwnerPhone,
			Email:    reg.Email,
		},
		Address: QRISAddressResponse{
			Street:     reg.AddressStreet,
			RT:         derefStr(reg.AddressRT),
			RW:         derefStr(reg.AddressRW),
			Kelurahan:  derefStr(reg.AddressKelurahan),
			Kecamatan:  derefStr(reg.AddressKecamatan),
			City:       reg.City,
			PostalCode: derefStr(reg.PostalCode),
		},
		Business: QRISBusinessResponse{
			Name:                 reg.BusinessName,
			MCC:                  reg.MCC,
			HasPhysicalStore:     reg.HasPhysicalStore,
			OmzetCategory:        reg.OmzetCategory,
			RiskCategory:         reg.RiskCategory,
			Website:              derefStr(reg.Website),
			EstimatedSalesVolume: reg.EstimatedSalesVolume,
			EstimatedTxCount:     reg.EstimatedTxCount,
		},
		QRISMerchantID: reg.QRISMerchantID,
		Note:           derefStr(reg.Note),
		CreatedAt:      formatPaymentTime(reg.CreatedAt),
		UpdatedAt:      formatPaymentTime(reg.UpdatedAt),
	}
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

	// Idempotency: a repeated referenceId for the same client returns 409 rather
	// than creating a duplicate (mirrors the Payment API).
	if existing, gerr := s.repo.GetByRef(ctx, clientID, reg.RegistrationRef); gerr == nil && existing != nil {
		return nil, regErr(http.StatusConflict, "DUPLICATE_REFERENCE",
			"a registration with this referenceId already exists", nil)
	} else if gerr != nil && !errors.Is(gerr, sql.ErrNoRows) {
		return nil, regErr(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to check referenceId", gerr)
	}

	created, err := s.repo.Create(ctx, reg)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, regErr(http.StatusConflict, "DUPLICATE_REFERENCE",
				"a registration with this referenceId already exists", err)
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

// Get returns a registration scoped to the client by its public UUID id.
func (s *QRISRegistrationService) Get(ctx context.Context, clientID int, registrationID string) (*models.QRISRegistration, error) {
	reg, err := s.repo.GetByRegistrationID(ctx, clientID, strings.TrimSpace(registrationID))
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
		// Reflect the activated state so the webhook payload matches a subsequent
		// GET of the registration (same mapper, same shape as the API).
		reg.Status = models.QRISRegActivated
		reg.QRISMerchantID = &created.ID
		mid := created.ID
		s.callbackSvc.Enqueue(ctx, *reg.ClientID, models.QRISEventMerchantActivated, &mid, nil, ToQRISRegistrationResponse(reg))
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

	ref := strings.TrimSpace(req.ReferenceID)
	if ref == "" {
		return bad("referenceId is required")
	}

	qrisType := models.QRISType(strings.ToLower(strings.TrimSpace(req.QRISType)))
	if !qrisType.Valid() {
		return bad("qrisType must be one of static, dynamic, both")
	}

	ownerName := strings.TrimSpace(req.Owner.FullName)
	if ownerName == "" {
		return bad("owner.fullName is required")
	}
	nik := strings.TrimSpace(req.Owner.NIK)
	if !nikPattern.MatchString(nik) {
		return bad("owner.nik must be exactly 16 digits")
	}
	phone := strings.TrimSpace(req.Owner.Phone)
	if phone == "" {
		return bad("owner.phone is required")
	}
	email := strings.TrimSpace(req.Owner.Email)
	if email == "" || !strings.Contains(email, "@") {
		return bad("a valid owner.email is required")
	}

	businessName := strings.TrimSpace(req.Business.Name)
	if businessName == "" {
		return bad("business.name is required")
	}
	if len([]rune(businessName)) > 25 {
		return bad("business.name must be at most 25 characters")
	}
	businessName = strings.ToUpper(businessName) // form requires capitalised

	mcc := strings.TrimSpace(req.Business.MCC)
	if !mccPattern.MatchString(mcc) {
		return bad("business.mcc must be a 4-digit code")
	}

	street := strings.TrimSpace(req.Address.Street)
	if street == "" {
		return bad("address.street is required")
	}
	city := strings.TrimSpace(req.Address.City)
	if city == "" {
		return bad("address.city is required")
	}

	omzet := strings.ToUpper(strings.TrimSpace(req.Business.OmzetCategory))
	if !omzetCategories[omzet] {
		return bad("business.omzetCategory must be one of UMI, UKE, UME, UBE, URE, PSO, BLU")
	}

	risk := normalizeRisk(req.Business.RiskCategory)
	if !riskCategories[risk] {
		return bad("business.riskCategory must be one of Low, Medium, High")
	}

	hasStore := true
	if req.Business.HasPhysicalStore != nil {
		hasStore = *req.Business.HasPhysicalStore
	}

	reg := &models.QRISRegistration{
		RegistrationID:       uuid.New().String(),
		ClientID:             &clientID,
		RegistrationRef:      ref,
		OwnerFullName:        ownerName,
		OwnerNIK:             nik,
		OwnerPhone:           phone,
		Email:                email,
		BusinessName:         businessName,
		MCC:                  mcc,
		AddressStreet:        street,
		AddressRT:            nilIfBlank(req.Address.RT),
		AddressRW:            nilIfBlank(req.Address.RW),
		AddressKelurahan:     nilIfBlank(req.Address.Kelurahan),
		AddressKecamatan:     nilIfBlank(req.Address.Kecamatan),
		City:                 city,
		PostalCode:           nilIfBlank(req.Address.PostalCode),
		HasPhysicalStore:     hasStore,
		OmzetCategory:        omzet,
		QRISType:             qrisType,
		RiskCategory:         risk,
		Website:              nilIfBlank(req.Business.Website),
		EstimatedSalesVolume: req.Business.EstimatedSalesVolume,
		EstimatedTxCount:     req.Business.EstimatedTxCount,
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
