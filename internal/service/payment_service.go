package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/sse"
)

// ----------------------------------------------------------------------------
// Errors
// ----------------------------------------------------------------------------

type PaymentServiceError struct {
	HTTPStatus int
	Code       string
	Message    string
	Err        error
}

func (e *PaymentServiceError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Code + ": " + e.Message
	}
	return e.Code + ": " + e.Message + ": " + e.Err.Error()
}

func (e *PaymentServiceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newPaymentError(httpStatus int, code, message string, err error) *PaymentServiceError {
	return &PaymentServiceError{HTTPStatus: httpStatus, Code: code, Message: message, Err: err}
}

// ----------------------------------------------------------------------------
// Request/response DTOs
// ----------------------------------------------------------------------------

// PaymentMethodRequest is the nested paymentMethod object in the create request.
type PaymentMethodRequest struct {
	Type string `json:"type"`
	Code string `json:"code"`
}

// CustomerRequest is the nested customer object in the create request.
type CustomerRequest struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// UrlRequest groups callback and return URLs.
type UrlRequest struct {
	Callback string `json:"callback,omitempty"`
	Return   string `json:"return,omitempty"`
}

// CreatePaymentRequest is the unified payment creation payload.
// Supports both new nested format and legacy flat fields for backward compat.
type CreatePaymentRequest struct {
	ReferenceID string                `json:"referenceId"`
	PaymentMethod *PaymentMethodRequest `json:"paymentMethod,omitempty"`
	Customer    *CustomerRequest      `json:"customer,omitempty"`
	Url         *UrlRequest           `json:"url,omitempty"`
	Amount      int64                 `json:"amount"`
	FeePaidBy   string                `json:"feePaidBy,omitempty"` // "merchant" (default) or "customer"
	ScanData    string                `json:"scanData,omitempty"`  // CPM QRIS: QR code from customer's app
	Description string                `json:"description,omitempty"`
	ExpiredAt   string                `json:"expiredAt,omitempty"`
	Metadata    json.RawMessage       `json:"metadata,omitempty"`

	// Legacy flat fields — kept for backward compatibility.
	// If paymentMethod is set, these are ignored.
	PaymentType   string `json:"paymentType,omitempty"`
	PaymentCode   string `json:"paymentCode,omitempty"`
	CustomerName  string `json:"customerName,omitempty"`
	CustomerEmail string `json:"customerEmail,omitempty"`
	CustomerPhone string `json:"customerPhone,omitempty"`
	CallbackURL   string `json:"callbackUrl,omitempty"`
	ReturnURL     string `json:"returnUrl,omitempty"`
}

// resolvePaymentTypeCode resolves the payment type and code from either the
// new nested paymentMethod field or the legacy flat fields.
func (r *CreatePaymentRequest) resolvePaymentTypeCode() (paymentType, paymentCode string) {
	if r.PaymentMethod != nil && r.PaymentMethod.Type != "" {
		return r.PaymentMethod.Type, r.PaymentMethod.Code
	}
	return r.PaymentType, r.PaymentCode
}

// resolveCustomer returns customer fields from either nested or flat fields.
func (r *CreatePaymentRequest) resolveCustomer() (name, email, phone string) {
	if r.Customer != nil {
		return r.Customer.Name, r.Customer.Email, r.Customer.Phone
	}
	return r.CustomerName, r.CustomerEmail, r.CustomerPhone
}

// resolveURL returns callback and return URLs preferring the nested url object.
func (r *CreatePaymentRequest) resolveURL() (callback, returnURL string) {
	if r.Url != nil {
		return r.Url.Callback, r.Url.Return
	}
	return r.CallbackURL, r.ReturnURL
}

// validateRequiredFields validates required fields based on payment method type/code.
func (r *CreatePaymentRequest) validateRequiredFields(paymentType, paymentCode string) error {
	// OVO requires customer.phone
	if paymentType == "EWALLET" && strings.EqualFold(paymentCode, "OVO") {
		_, _, phone := r.resolveCustomer()
		if strings.TrimSpace(phone) == "" {
			return newPaymentError(400, "MISSING_REQUIRED_FIELD", "customer.phone is required for EWALLET/OVO", nil)
		}
	}
	// AstraPay requires url.return
	if paymentType == "EWALLET" && strings.EqualFold(paymentCode, "ASTRAPAY") {
		_, returnURL := r.resolveURL()
		if strings.TrimSpace(returnURL) == "" {
			return newPaymentError(400, "MISSING_RETURN_URL", "url.return is required for EWALLET/ASTRAPAY", nil)
		}
	}
	// QRIS CPM requires scanData
	if paymentType == "QRIS" && strings.EqualFold(paymentCode, "CPM") {
		if strings.TrimSpace(r.ScanData) == "" {
			return newPaymentError(400, "MISSING_SCAN_DATA", "scanData is required for QRIS/CPM", nil)
		}
	}
	return nil
}

// isValidUUIDv4 checks if string is a valid UUID version 4.
func isValidUUIDv4(s string) bool {
	id, err := uuid.Parse(s)
	if err != nil {
		return false
	}
	return id.Version() == 4
}

// CustomerResponse is the nested customer object in the response.
type CustomerResponse struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// PaymentMethodResponse is the nested paymentMethod object in the response.
type PaymentMethodResponse struct {
	Type string `json:"type"`
	Code string `json:"code"`
}

// AmountResponse is the nested amount object in the response.
type AmountResponse struct {
	Subtotal int64 `json:"subtotal"` // original amount before fee
	Fee      int64 `json:"fee"`
	Total    int64 `json:"total"` // subtotal + fee
}

// PaymentResponse is the shape returned on create/get endpoints.
type PaymentResponse struct {
	ID                 string                 `json:"id"`
	ReferenceID        string                 `json:"referenceId"`
	PaymentMethod      PaymentMethodResponse  `json:"paymentMethod"`
	Amount             AmountResponse         `json:"amount"`
	FeePaidBy          string                 `json:"feePaidBy"`
	Status             string                 `json:"status"`
	PaymentDetail      json.RawMessage        `json:"paymentDetail,omitempty"`
	PaymentInstruction json.RawMessage        `json:"paymentInstruction,omitempty"`
	Customer           *CustomerResponse      `json:"customer,omitempty"`
	Description        string                 `json:"description,omitempty"`
	ExpiredAt          string                 `json:"expiredAt"`
	PaidAt             string                 `json:"paidAt,omitempty"`
	CancelledAt        string                 `json:"cancelledAt,omitempty"`
	CreatedAt          string                 `json:"createdAt"`
}

// MethodsResponse groups active methods by payment type for the list endpoint.
type MethodsResponse struct {
	VA      []MethodEntry `json:"va"`
	Ewallet []MethodEntry `json:"ewallet"`
	QRIS    []MethodEntry `json:"qris"`
	Retail  []MethodEntry `json:"retail"`
}

type MethodEntry struct {
	ID              int    `json:"id"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	FeeType         string `json:"feeType"`
	FeeFlat         int    `json:"feeFlat"`
	FeePercent      float64 `json:"feePercent"`
	FeeMin          int    `json:"feeMin"`
	FeeMax          int    `json:"feeMax"`
	MinAmount       int    `json:"minAmount"`
	MaxAmount       int    `json:"maxAmount"`
	ExpiredDuration int    `json:"expiredDuration"`
	LogoURL         string `json:"logoUrl,omitempty"`
	IsMaintenance   bool   `json:"isMaintenance"`
}

// ----------------------------------------------------------------------------
// Service
// ----------------------------------------------------------------------------

type PaymentService struct {
	paymentRepo *repository.PaymentRepository
	clientRepo  *repository.ClientRepository
	router      *PaymentProviderRouter
	selector    *ProviderSelector
	callbackSvc *PaymentCallbackService
	notifier    sse.PaymentNotifier
}

func NewPaymentService(
	paymentRepo *repository.PaymentRepository,
	clientRepo *repository.ClientRepository,
	router *PaymentProviderRouter,
	callbackSvc *PaymentCallbackService,
) *PaymentService {
	return &PaymentService{
		paymentRepo: paymentRepo,
		clientRepo:  clientRepo,
		router:      router,
		selector:    NewProviderSelector(paymentRepo, router),
		callbackSvc: callbackSvc,
	}
}

func (s *PaymentService) SetNotifier(notifier sse.PaymentNotifier) {
	s.notifier = notifier
}

// ----------------------------------------------------------------------------
// Method listing
// ----------------------------------------------------------------------------

func (s *PaymentService) ListMethods(ctx context.Context) (*MethodsResponse, error) {
	methods, err := s.paymentRepo.ListActiveMethods(ctx)
	if err != nil {
		return nil, err
	}
	// De-duplicate by (type, code) so a logical method served by several
	// providers shows up once on the public list endpoint (Req 6.18).
	methods = dedupMethodsByTypeCode(methods)
	resp := &MethodsResponse{}
	for i := range methods {
		m := methods[i]
		entry := MethodEntry{
			ID:              m.ID,
			Code:            m.Code,
			Name:            m.Name,
			Provider:        string(m.Provider),
			FeeType:         string(m.FeeType),
			FeeFlat:         m.FeeFlat,
			FeePercent:      m.FeePercent,
			FeeMin:          m.FeeMin,
			FeeMax:          m.FeeMax,
			MinAmount:       m.MinAmount,
			MaxAmount:       m.MaxAmount,
			ExpiredDuration: m.ExpiredDuration,
			IsMaintenance:   m.IsMaintenance,
		}
		if m.LogoURL != nil {
			entry.LogoURL = *m.LogoURL
		}
		switch m.Type {
		case models.PaymentTypeVA:
			resp.VA = append(resp.VA, entry)
		case models.PaymentTypeEwallet:
			resp.Ewallet = append(resp.Ewallet, entry)
		case models.PaymentTypeQRIS:
			resp.QRIS = append(resp.QRIS, entry)
		case models.PaymentTypeRetail:
			resp.Retail = append(resp.Retail, entry)
		}
	}
	return resp, nil
}

// ----------------------------------------------------------------------------
// Create
// ----------------------------------------------------------------------------

func (s *PaymentService) CreatePayment(ctx context.Context, req *CreatePaymentRequest, client *models.Client, isSandbox bool) (*PaymentResponse, error) {
	if req == nil {
		return nil, newPaymentError(400, "MISSING_FIELD", "Invalid request body", nil)
	}
	if client == nil {
		return nil, newPaymentError(401, "INVALID_TOKEN", "Unauthorized", nil)
	}

	req.ReferenceID = strings.TrimSpace(req.ReferenceID)

	// Resolve payment type/code from nested or flat fields.
	rawType, rawCode := req.resolvePaymentTypeCode()
	rawType = strings.ToUpper(strings.TrimSpace(rawType))
	rawCode = strings.TrimSpace(rawCode)

	// Resolve customer fields from nested or flat fields.
	customerName, customerEmail, customerPhone := req.resolveCustomer()
	callbackURL, returnURL := req.resolveURL()

	if req.ReferenceID == "" {
		return nil, newPaymentError(400, "MISSING_REQUIRED_FIELD", "referenceId is required", nil)
	}
	if !isValidUUIDv4(req.ReferenceID) {
		return nil, newPaymentError(400, "INVALID_FIELD_VALUE", "referenceId must be a valid UUIDv4", nil)
	}
	if rawType == "" || rawCode == "" {
		return nil, newPaymentError(400, "MISSING_REQUIRED_FIELD", "paymentMethod.type and paymentMethod.code are required", nil)
	}

	// Validate required fields based on payment method (Fix #4)
	if err := req.validateRequiredFields(rawType, rawCode); err != nil {
		return nil, err
	}
	if req.Amount <= 0 {
		return nil, newPaymentError(400, "INVALID_AMOUNT", "amount must be positive", nil)
	}

	// Basic validation for nested customer when provided (Req 4.2).
	if req.Customer != nil {
		if strings.TrimSpace(req.Customer.Name) == "" && strings.TrimSpace(req.CustomerEmail) == "" {
			// Accept legacy flat customerName as fallback
			if strings.TrimSpace(req.CustomerName) == "" {
				return nil, newPaymentError(400, "MISSING_FIELD", "customer.name is required when customer object is provided", nil)
			}
		}
	}

	// Resolve and validate feePaidBy (default merchant; rejects invalid values).
	// Done before idempotency so a malformed feePaidBy is rejected even for a
	// reference that does not yet exist (design "Validation Order" step 2).
	feePaidBy, err := normalizeFeePaidBy(req.FeePaidBy)
	if err != nil {
		return nil, err
	}

	// Idempotency: return existing record for identical referenceId.
	if existing, err := s.paymentRepo.GetByReferenceID(ctx, client.ID, req.ReferenceID); err == nil && existing != nil {
		return s.buildResponse(existing), nil
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Method resolution: load the canonical (type, code) method and apply
	// method-level health checks (NOT_FOUND / inactive / maintenance).
	method, err := s.paymentRepo.GetMethodByTypeCode(ctx, models.PaymentType(rawType), rawCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYMENT_METHOD_NOT_FOUND", "Payment method not found", nil)
		}
		return nil, err
	}
	if !method.IsActive {
		return nil, newPaymentError(400, "PAYMENT_METHOD_INACTIVE", "Payment method is not active", nil)
	}
	if method.IsMaintenance {
		msg := "Payment method is under maintenance"
		if method.MaintenanceMessage != nil && *method.MaintenanceMessage != "" {
			msg = *method.MaintenanceMessage
		}
		return nil, newPaymentError(503, "PAYMENT_METHOD_MAINTENANCE", msg, nil)
	}

	// Amount bounds.
	if method.MinAmount > 0 && req.Amount < int64(method.MinAmount) {
		return nil, newPaymentError(400, "AMOUNT_TOO_LOW", fmt.Sprintf("amount must be >= %d", method.MinAmount), nil)
	}
	if method.MaxAmount > 0 && req.Amount > int64(method.MaxAmount) {
		return nil, newPaymentError(400, "AMOUNT_TOO_HIGH", fmt.Sprintf("amount must be <= %d", method.MaxAmount), nil)
	}

	// CPM QRIS requires the QR content scanned from the customer's app.
	if method.Type == models.PaymentTypeQRIS && strings.EqualFold(strings.TrimSpace(method.Code), "CPM") &&
		strings.TrimSpace(req.ScanData) == "" {
		return nil, newPaymentError(400, "MISSING_FIELD",
			"scanData is required for QRIS CPM", nil)
	}

	// Provider selection via the Method_Provider_Mapping: resolve the method
	// group and pick the highest-priority healthy provider. Returns
	// PAYMENT_METHOD_UNAVAILABLE when no provider qualifies (Req 6.16, 6.17).
	group, err := s.selector.Resolve(ctx, method.Type, method.Code)
	if err != nil {
		return nil, err
	}
	binding, err := s.selector.Select(group)
	if err != nil {
		return nil, err
	}

	fee := method.CalculateFee(req.Amount)
	// total depends on who bears the fee:
	//   customer -> total = subtotal + fee
	//   merchant -> total = subtotal
	totalAmount := computeTotal(req.Amount, fee, feePaidBy)
	expiredAt := resolveExpiredAt(req.ExpiredAt, method.ExpiredDuration)

	metadata := models.NullableRawMessage(req.Metadata)

	payment := &models.Payment{
		ReferenceID:     req.ReferenceID,
		ClientID:        client.ID,
		PaymentMethodID: method.ID,
		IsSandbox:       isSandbox,
		PaymentType:     method.Type,
		PaymentCode:     method.Code,
		Provider:        binding.Provider,
		Amount:          req.Amount,
		Fee:             fee,
		TotalAmount:     totalAmount,
		FeePaidBy:       feePaidBy,
		Status:          models.PaymentStatusPending,
		ExpiredAt:       expiredAt,
		Metadata:        metadata,
		// payment_detail has NOT NULL constraint with default '{}' in DB.
		// Set explicitly so Go driver does not send NULL.
		PaymentDetail: models.NullableRawMessage(`{}`),
	}
	if customerName != "" {
		v := customerName
		payment.CustomerName = &v
	}
	if customerEmail != "" {
		v := customerEmail
		payment.CustomerEmail = &v
	}
	if customerPhone != "" {
		v := customerPhone
		payment.CustomerPhone = &v
	}
	if req.Description != "" {
		v := req.Description
		payment.Description = &v
	}

	if err := s.createPaymentWithGeneratedID(ctx, payment); err != nil {
		return nil, err
	}

	// Attempt creation against the chosen provider, advancing to the next
	// healthy provider on a retryable failure (bounded fallback). Non-retryable
	// rejections fail fast. Each attempt writes a payment_logs row (Req 16.7).
	var providerResp *PaymentCreateResponse
	for binding != nil {
		payment.Provider = binding.Provider

		bankCode := method.Code
		if binding.ProviderBankCode != nil && strings.TrimSpace(*binding.ProviderBankCode) != "" {
			bankCode = strings.TrimSpace(*binding.ProviderBankCode)
		}
		providerReq := &PaymentCreateRequest{
			Type:          method.Type,
			Code:          method.Code,
			BankCode:      bankCode,
			PartnerRef:    payment.PaymentID,
			Amount:        req.Amount,
			Fee:           fee,
			TotalAmount:   totalAmount,
			ExpiredAt:     expiredAt,
			Description:   req.Description,
			ClientName:    client.Name,
			CustomerName:  customerName,
			CustomerEmail: customerEmail,
			CustomerPhone: customerPhone,
			CallbackURL:   callbackURL,
			ReturnURL:     returnURL,
			ScanData:      req.ScanData,
		}

		var providerErr error
		provider, routeErr := s.router.Get(binding.Provider)
		if routeErr != nil {
			// No usable adapter for this binding — treat as retryable so we can
			// advance to the next provider in the mapping.
			providerErr = routeErr
		} else {
			start := time.Now()
			providerResp, providerErr = provider.CreatePayment(ctx, method, providerReq)
			elapsed := int(time.Since(start) / time.Millisecond)
			s.writeCreateLog(ctx, payment, binding.Provider, providerReq, providerResp, providerErr, elapsed)
		}

		if providerErr == nil {
			break
		}

		// Retryable failure: advance to the next healthy provider if available.
		if isRetryableProviderError(providerErr) {
			if next := s.selector.Next(group, binding); next != nil {
				binding = next
				providerResp = nil
				continue
			}
		}

		// Non-retryable, or no further provider: fail fast.
		s.markFailed(ctx, payment, paymentProviderErrorCode(providerErr), providerErr.Error())
		return nil, providerErr
	}

	// Simplify detail per spec (Fix #5): VA→{vaName,vaNumber}, QRIS-MPM→{qrString}, CPM→{}, EWALLET→{checkoutUrl} or {}, RETAIL→{paymentCode}
	simplified := simplifyPaymentDetail(method.Type, method.Code, providerResp.Normalized)
	detailJSON, _ := json.Marshal(simplified)
	payment.PaymentDetail = models.NullableRawMessage(detailJSON)
	if providerResp.ProviderRef != "" {
		v := providerResp.ProviderRef
		payment.ProviderRef = &v
	}
	payment.ProviderData = mergeProviderData(payment.ProviderData, "create", providerResp.RawResponse)
	payment.PaymentInstruction = renderInstruction(method, providerResp.Normalized, req.Amount)

	if err := s.paymentRepo.UpdatePayment(ctx, payment); err != nil {
		return nil, err
	}

	if s.notifier != nil {
		s.notifier.NotifyPaymentCreated(payment)
	}
	go s.EnqueueCallback(context.Background(), payment, "payment.pending")

	return s.buildResponse(payment), nil
}

// writeCreateLog persists a payment_logs row for a single provider create
// attempt, capturing the request, response, latency, and any error
// code/message (Req 16.7). Log failures are non-fatal.
func (s *PaymentService) writeCreateLog(
	ctx context.Context,
	payment *models.Payment,
	provider models.PaymentProvider,
	providerReq *PaymentCreateRequest,
	providerResp *PaymentCreateResponse,
	providerErr error,
	elapsedMs int,
) {
	logEntry := &models.PaymentLog{
		PaymentID: payment.ID,
		Action:    "create",
		Provider:  provider,
		IsSuccess: providerErr == nil,
	}
	reqBody, _ := json.Marshal(providerReq)
	logEntry.Request = models.NullableRawMessage(reqBody)
	if providerResp != nil {
		logEntry.Response = models.NullableRawMessage(providerResp.RawResponse)
	}
	if providerErr != nil {
		msg := providerErr.Error()
		logEntry.ErrorMessage = &msg
		code := paymentProviderErrorCode(providerErr)
		logEntry.ErrorCode = &code
	}
	now := time.Now()
	logEntry.ResponseAt = &now
	logEntry.ResponseTimeMs = &elapsedMs
	if err := s.paymentRepo.CreatePaymentLog(ctx, logEntry); err != nil {
		log.Warn().Err(err).Msg("payment: store log")
	}
}

// isRetryableProviderError reports whether a provider create failure should
// trigger fallback to the next provider in the mapping. Retryable failures are
// PROVIDER_UNAVAILABLE / 5xx (transient); request rejections and validation
// errors are not retryable. A missing/unconfigured adapter is also retryable.
func isRetryableProviderError(err error) bool {
	if err == nil {
		return false
	}
	var svcErr *PaymentServiceError
	if errors.As(err, &svcErr) {
		if svcErr.Code == "PROVIDER_UNAVAILABLE" || svcErr.Code == "PAYMENT_PROVIDER_UNAVAILABLE" {
			return true
		}
		return svcErr.HTTPStatus >= 500
	}
	// Non-typed errors (e.g. router.Get when adapter unregistered) are treated
	// as transient so the mapping can fall back to another provider.
	return true
}

// paymentProviderErrorCode extracts the stable error code from a provider
// failure, defaulting to PROVIDER_ERROR for untyped errors.
func paymentProviderErrorCode(err error) string {
	var svcErr *PaymentServiceError
	if errors.As(err, &svcErr) && svcErr.Code != "" {
		return svcErr.Code
	}
	return "PROVIDER_ERROR"
}

// ----------------------------------------------------------------------------
// Get / read path
// ----------------------------------------------------------------------------

func (s *PaymentService) GetPayment(ctx context.Context, paymentID string, clientID int) (*PaymentResponse, error) {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return nil, newPaymentError(400, "MISSING_FIELD", "paymentId is required", nil)
	}
	p, err := s.paymentRepo.GetByPaymentID(ctx, paymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYMENT_NOT_FOUND", "Payment not found", nil)
		}
		return nil, err
	}
	if p.ClientID != clientID {
		return nil, newPaymentError(404, "PAYMENT_NOT_FOUND", "Payment not found", nil)
	}
	if p.Status == models.PaymentStatusPending && time.Since(p.UpdatedAt) > 15*time.Second {
		if refreshed, err := s.refreshStatus(ctx, p); err == nil && refreshed != nil {
			p = refreshed
		}
	}
	return s.buildResponse(p), nil
}

// ----------------------------------------------------------------------------
// Cancel
// ----------------------------------------------------------------------------

func (s *PaymentService) CancelPayment(ctx context.Context, paymentID string, clientID int, reason string) (*PaymentResponse, error) {
	p, err := s.paymentRepo.GetByPaymentID(ctx, strings.TrimSpace(paymentID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYMENT_NOT_FOUND", "Payment not found", nil)
		}
		return nil, err
	}
	if p.ClientID != clientID {
		return nil, newPaymentError(404, "PAYMENT_NOT_FOUND", "Payment not found", nil)
	}
	if p.Status != models.PaymentStatusPending {
		return nil, newPaymentError(400, "INVALID_PAYMENT_STATE", "Only pending payments can be cancelled", nil)
	}
	if provider, err := s.router.Get(p.Provider); err == nil {
		if _, err := provider.CancelPayment(ctx, p, reason); err != nil {
			log.Warn().Err(err).Str("paymentId", p.PaymentID).Msg("payment: provider cancel failed")
			// Continue — we still mark locally cancelled so merchant is consistent.
		}
	}
	now := time.Now()
	p.Status = models.PaymentStatusCancelled
	p.CancelledAt = &now
	if err := s.paymentRepo.UpdatePayment(ctx, p); err != nil {
		return nil, err
	}
	if s.notifier != nil {
		s.notifier.NotifyPaymentStatusChanged(p)
	}
	go s.EnqueueCallback(context.Background(), p, "payment.cancelled")
	return s.buildResponse(p), nil
}

// ----------------------------------------------------------------------------
// Workers hooks
// ----------------------------------------------------------------------------

// ProcessPendingPayments reconciles stale Pending payments by inquiring provider.
func (s *PaymentService) ProcessPendingPayments(ctx context.Context, staleAfter time.Duration, limit int) error {
	rows, err := s.paymentRepo.GetPendingPaymentsPastStale(ctx, staleAfter, limit)
	if err != nil {
		return err
	}
	for i := range rows {
		p := &rows[i]
		if _, err := s.refreshStatus(ctx, p); err != nil {
			log.Warn().Err(err).Str("paymentId", p.PaymentID).Msg("payment: refresh status")
		}
	}
	return nil
}

// ExpirePendingPayments marks Pending payments whose expired_at is in the past.
func (s *PaymentService) ExpirePendingPayments(ctx context.Context, limit int) error {
	rows, err := s.paymentRepo.GetExpiredPendingPayments(ctx, limit)
	if err != nil {
		return err
	}
	for i := range rows {
		p := &rows[i]
		p.Status = models.PaymentStatusExpired
		if err := s.paymentRepo.UpdatePayment(ctx, p); err != nil {
			log.Warn().Err(err).Str("paymentId", p.PaymentID).Msg("payment: mark expired")
			continue
		}
		if s.notifier != nil {
			s.notifier.NotifyPaymentStatusChanged(p)
		}
		go s.EnqueueCallback(context.Background(), p, "payment.expired")
	}
	return nil
}

// ApplyWebhook processes an authenticated provider webhook. The provider
// layer is responsible for verifying signatures before calling this.
//
// Flow: receive webhook → call provider status API to validate → apply result.
// The webhook payload status is used as a hint; the inquiry result is authoritative.
// If inquiry fails (provider unreachable), we fall back to the webhook status so
// payments are not silently dropped.
func (s *PaymentService) ApplyWebhook(ctx context.Context, provider models.PaymentProvider, partnerRef string, event PaymentWebhookEvent) error {
	p, err := s.paymentRepo.GetByPaymentID(ctx, partnerRef)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Try lookup by provider_ref as some providers echo their own id.
			p, err = s.paymentRepo.GetByProviderRef(ctx, provider, partnerRef)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if p.Status.IsFinal() {
		// Idempotent: duplicate webhook.
		return nil
	}

	// Store the webhook payload before inquiry so it's always recorded.
	if len(event.RawPayload) > 0 {
		p.ProviderData = mergeProviderData(p.ProviderData, "webhook", event.RawPayload)
	}
	if event.ProviderRef != "" {
		v := event.ProviderRef
		p.ProviderRef = &v
	}

	// Validate via provider status API (authoritative source of truth).
	// On success use the inquiry result; on any error fall back to webhook payload status.
	confirmedStatus := event.Status
	if providerClient, routeErr := s.router.Get(p.Provider); routeErr == nil {
		if result, inquiryErr := providerClient.InquiryPayment(ctx, p); inquiryErr == nil && result != nil {
			if result.Status != "" {
				confirmedStatus = result.Status
			}
			if result.ProviderRef != "" {
				v := result.ProviderRef
				p.ProviderRef = &v
			}
			if len(result.RawResponse) > 0 {
				p.ProviderData = mergeProviderData(p.ProviderData, "inquiry", result.RawResponse)
			}
		} else if inquiryErr != nil {
			log.Warn().Err(inquiryErr).
				Str("provider", string(p.Provider)).
				Str("paymentId", p.PaymentID).
				Msg("payment webhook: inquiry validation failed, falling back to webhook status")
		}
	}

	prevStatus := p.Status
	p.Status = confirmedStatus
	now := time.Now()
	if confirmedStatus == models.PaymentStatusPaid && p.PaidAt == nil {
		p.PaidAt = &now
	}
	if confirmedStatus == models.PaymentStatusCancelled && p.CancelledAt == nil {
		p.CancelledAt = &now
	}

	if err := s.paymentRepo.UpdatePayment(ctx, p); err != nil {
		return err
	}
	if prevStatus == p.Status {
		return nil
	}
	if s.notifier != nil {
		s.notifier.NotifyPaymentStatusChanged(p)
	}
	eventName := paymentEventName(p.Status)
	go s.EnqueueCallback(context.Background(), p, eventName)
	return nil
}

// PaymentWebhookEvent carries the normalized webhook outcome for ApplyWebhook.
type PaymentWebhookEvent struct {
	Status      models.PaymentStatus
	ProviderRef string
	PaidAmount  int64
	RawPayload  json.RawMessage
}

// ----------------------------------------------------------------------------
// Internals
// ----------------------------------------------------------------------------

func (s *PaymentService) createPaymentWithGeneratedID(ctx context.Context, p *models.Payment) error {
	return createWithGeneratedID(p, func(pp *models.Payment) error {
		return s.paymentRepo.CreatePayment(ctx, pp)
	})
}

// createWithGeneratedID generates a fresh UUIDv4 public id for the payment and
// attempts the insert, regenerating the id and retrying on a payment_id unique
// violation (Req 5.4). A reference-id unique violation is surfaced as
// DUPLICATE_REFERENCE_ID; any other error fails immediately. The insert
// function is injected so the retry loop is testable without a database.
func createWithGeneratedID(p *models.Payment, insert func(*models.Payment) error) error {
	var lastErr error
	for i := 0; i < 5; i++ {
		p.PaymentID = newPaymentPublicID("")
		lastErr = insert(p)
		if lastErr == nil {
			return nil
		}
		if !isUniqueViolation(lastErr) {
			if isReferenceUniqueViolation(lastErr) {
				return newPaymentError(400, "DUPLICATE_REFERENCE_ID", "Reference ID already exists", lastErr)
			}
			return lastErr
		}
	}
	return lastErr
}

func (s *PaymentService) markFailed(ctx context.Context, p *models.Payment, code, message string) {
	p.Status = models.PaymentStatusFailed
	msg := code + ": " + message
	if p.Description == nil {
		p.Description = &msg
	}
	_ = s.paymentRepo.UpdatePayment(ctx, p)
	if s.notifier != nil {
		s.notifier.NotifyPaymentStatusChanged(p)
	}
	go s.EnqueueCallback(context.Background(), p, "payment.failed")
}

func (s *PaymentService) refreshStatus(ctx context.Context, p *models.Payment) (*models.Payment, error) {
	if p.Status.IsFinal() {
		return p, nil
	}
	provider, err := s.router.Get(p.Provider)
	if err != nil {
		return p, nil
	}
	result, err := provider.InquiryPayment(ctx, p)
	if err != nil {
		return p, err
	}
	if result == nil {
		return p, nil
	}
	if result.ProviderRef != "" {
		v := result.ProviderRef
		p.ProviderRef = &v
	}
	if len(result.RawResponse) > 0 {
		p.ProviderData = mergeProviderData(p.ProviderData, "inquiry", result.RawResponse)
	}
	if result.Status != "" && result.Status != p.Status {
		prev := p.Status
		p.Status = result.Status
		now := time.Now()
		if result.Status == models.PaymentStatusPaid && p.PaidAt == nil {
			p.PaidAt = &now
		}
		if result.Status == models.PaymentStatusCancelled && p.CancelledAt == nil {
			p.CancelledAt = &now
		}
		if err := s.paymentRepo.UpdatePayment(ctx, p); err != nil {
			return p, err
		}
		if s.notifier != nil {
			s.notifier.NotifyPaymentStatusChanged(p)
		}
		if prev != p.Status {
			eventName := paymentEventName(p.Status)
			go s.EnqueueCallback(context.Background(), p, eventName)
		}
	} else if err := s.paymentRepo.UpdatePayment(ctx, p); err != nil {
		return p, err
	}
	return p, nil
}

// EnqueueCallback forwards to the callback service if configured.
func (s *PaymentService) EnqueueCallback(ctx context.Context, p *models.Payment, event string) {
	if s == nil || s.callbackSvc == nil {
		return
	}
	s.callbackSvc.EnqueueEvent(ctx, p, event)
}

func (s *PaymentService) buildResponse(p *models.Payment) *PaymentResponse {
	resp := &PaymentResponse{
		ID:          p.PaymentID,
		ReferenceID: p.ReferenceID,
		PaymentMethod: PaymentMethodResponse{
			Type: string(p.PaymentType),
			Code: p.PaymentCode,
		},
		Amount: AmountResponse{
			Subtotal: p.Amount,
			Fee:      p.Fee,
			Total:    p.TotalAmount,
		},
		FeePaidBy:          string(p.FeePaidBy),
		Status:             string(p.Status),
		PaymentDetail:      json.RawMessage(p.PaymentDetail),
		PaymentInstruction: json.RawMessage(p.PaymentInstruction),
		ExpiredAt:          formatPaymentTime(p.ExpiredAt),
		CreatedAt:          formatPaymentTime(p.CreatedAt),
	}
	// Populate nested Customer object (Fix #1)
	if p.CustomerName != nil || p.CustomerEmail != nil || p.CustomerPhone != nil {
		resp.Customer = &CustomerResponse{}
		if p.CustomerName != nil {
			resp.Customer.Name = *p.CustomerName
		}
		if p.CustomerEmail != nil {
			resp.Customer.Email = *p.CustomerEmail
		}
		if p.CustomerPhone != nil {
			resp.Customer.Phone = *p.CustomerPhone
		}
	}
	if p.Description != nil {
		resp.Description = *p.Description
	}
	if p.PaidAt != nil {
		resp.PaidAt = formatPaymentTime(*p.PaidAt)
	}
	if p.CancelledAt != nil {
		resp.CancelledAt = formatPaymentTime(*p.CancelledAt)
	}
	return resp
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// simplifyPaymentDetail returns only the fields mandated by the contract (Fix #5).
func simplifyPaymentDetail(typ models.PaymentType, code string, n PaymentDetailNormalized) map[string]string {
	out := map[string]string{}
	switch typ {
	case models.PaymentTypeVA:
		if n.AccountName != "" {
			out["vaName"] = n.AccountName
		}
		if n.VANumber != "" {
			out["vaNumber"] = n.VANumber
		}
	case models.PaymentTypeQRIS:
		if strings.EqualFold(code, "CPM") {
			// CPM: provider stores internally via scanData; return empty object
			return map[string]string{}
		}
		if n.QRString != "" {
			out["qrString"] = n.QRString
		}
	case models.PaymentTypeEwallet:
		if strings.EqualFold(code, "OVO") {
			// OVO uses push notification, no URL to return
			return map[string]string{}
		}
		if n.CheckoutURL != "" {
			out["checkoutUrl"] = n.CheckoutURL
		} else if n.Deeplink != "" {
			out["checkoutUrl"] = n.Deeplink
		}
	case models.PaymentTypeRetail:
		if n.PaymentCode != "" {
			out["paymentCode"] = n.PaymentCode
		}
	}
	return out
}

func resolveExpiredAt(requested string, maxDurationSec int) time.Time {
	now := time.Now()
	var maxExp time.Time
	if maxDurationSec > 0 {
		maxExp = now.Add(time.Duration(maxDurationSec) * time.Second)
	}
	if maxDurationSec > 0 {
		maxExp = now.Add(time.Duration(maxDurationSec) * time.Second)
	}
	if strings.TrimSpace(requested) != "" {
		if t, err := time.Parse(time.RFC3339, requested); err == nil {
			if t.After(now) {
				if !maxExp.IsZero() && t.After(maxExp) {
					return maxExp
				}
				return t
			}
		}
	}
	if !maxExp.IsZero() {
		return maxExp
	}
	return now.Add(24 * time.Hour)
}

func mergeProviderData(current models.NullableRawMessage, key string, payload json.RawMessage) models.NullableRawMessage {
	merged := map[string]json.RawMessage{}
	if len(current) > 0 {
		_ = json.Unmarshal(current, &merged)
	}
	if strings.TrimSpace(key) != "" && len(payload) > 0 {
		merged[key] = payload
	}
	if len(merged) == 0 {
		return nil
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return current
	}
	return models.NullableRawMessage(raw)
}

// renderInstruction takes the method's template JSON and substitutes runtime
// placeholders like {VA_NUMBER}, {PAYMENT_CODE}, {AMOUNT}.
func renderInstruction(method *models.PaymentMethod, norm PaymentDetailNormalized, amount int64) models.NullableRawMessage {
	if len(method.PaymentInstruction) == 0 {
		return nil
	}
	tmpl := string(method.PaymentInstruction)
	replace := map[string]string{
		"{VA_NUMBER}":    norm.VANumber,
		"{PAYMENT_CODE}": norm.PaymentCode,
		"{QR_STRING}":    norm.QRString,
		"{AMOUNT}":       fmt.Sprintf("%d", amount),
	}
	for k, v := range replace {
		tmpl = strings.ReplaceAll(tmpl, k, v)
	}
	return models.NullableRawMessage(tmpl)
}

func formatPaymentTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	// Format in UTC but display with +07:00 offset label — the DB stores UTC
	// values and the server clock runs UTC, so the numeric hours already match
	// what WIB merchants see on their dashboard (no hour shift wanted).
	return t.UTC().Format("2006-01-02T15:04:05") + "+07:00"
}

func newPaymentPublicID(_ string) string {
	return uuid.New().String()
}

func computeTotal(subtotal, fee int64, feePaidBy models.FeePaidBy) int64 {
	if feePaidBy == models.FeePaidByCustomer {
		return subtotal + fee
	}
	return subtotal
}

// dedupMethodsByTypeCode keeps the first method seen for each (type, code)
// pair, preserving input order. It guarantees at most one entry per logical
// method on the public list endpoint even when several provider rows share the
// same (type, code) (Req 6.18).
func dedupMethodsByTypeCode(methods []models.PaymentMethod) []models.PaymentMethod {
	seen := make(map[string]struct{}, len(methods))
	out := make([]models.PaymentMethod, 0, len(methods))
	for i := range methods {
		key := string(methods[i].Type) + "\x00" + methods[i].Code
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, methods[i])
	}
	return out
}

// paymentEventName maps a payment status to its merchant webhook event name:
// "payment." + lowercase(status), with Partial_Refund mapped to
// "payment.partial_refund" (Req 8.4).
func paymentEventName(status models.PaymentStatus) string {
	if status == models.PaymentStatusPartialRefund {
		return "payment.partial_refund"
	}
	return "payment." + strings.ToLower(string(status))
}

// normalizeFeePaidBy validates and normalizes the request feePaidBy value.
// Empty defaults to merchant. Accepts "merchant"/"customer" case-insensitively
// and trimmed. Any other value is rejected with INVALID_FEE_PAID_BY (HTTP 400).
func normalizeFeePaidBy(raw string) (models.FeePaidBy, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return models.FeePaidByMerchant, nil
	case string(models.FeePaidByMerchant):
		return models.FeePaidByMerchant, nil
	case string(models.FeePaidByCustomer):
		return models.FeePaidByCustomer, nil
	default:
		return "", newPaymentError(400, "INVALID_FEE_PAID_BY",
			"feePaidBy must be 'merchant' or 'customer'", nil)
	}
}
