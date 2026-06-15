package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

const (
	payoutInquiryExpiry        = 30 * time.Minute
	payoutMinAmount      int64 = 10000
	payoutStatusLookback       = 30 * 24 * time.Hour
)

var payoutPurposeDescriptions = map[string]string{
	"01": "Investasi",
	"02": "Pemindahan Dana",
	"03": "Pembelian",
	"99": "Lainnya",
}

// PayoutService orchestrates inquiry + disbursement across providers with
// per-method_type priority routing and automatic fallback. It mirrors the
// payment service: a selector resolves healthy+capable provider candidates and
// the service tries them in order until one accepts the payout.
type PayoutService struct {
	repo           *repository.PayoutRepository
	bankRepo       *repository.BankCodeRepository
	methodRepo     *repository.PayoutMethodRepository
	selector       *PayoutSelector
	router         *PayoutProviderRouter
	callbackSvc    *PayoutCallbackService
	defaultBankFee int64
}

func NewPayoutService(
	repo *repository.PayoutRepository,
	bankRepo *repository.BankCodeRepository,
	methodRepo *repository.PayoutMethodRepository,
	selector *PayoutSelector,
	router *PayoutProviderRouter,
	callbackSvc *PayoutCallbackService,
	defaultBankFee int64,
) *PayoutService {
	return &PayoutService{
		repo:           repo,
		bankRepo:       bankRepo,
		methodRepo:     methodRepo,
		selector:       selector,
		router:         router,
		callbackSvc:    callbackSvc,
		defaultBankFee: defaultBankFee,
	}
}

// Available reports whether at least one payout provider is registered.
func (s *PayoutService) Available() bool {
	return s != nil && s.router != nil && len(s.router.Providers()) > 0
}

// ---------------------------------------------------------------------------
// Request shapes (camelCase JSON), mirroring the payment create request.
// ---------------------------------------------------------------------------

// PayoutInquiryRequest validates a recipient before a payout.
type PayoutInquiryRequest struct {
	PayoutMethod models.PayoutMethodRef `json:"payoutMethod"`
	AccountNo    string                 `json:"accountNo"`
}

// CreatePayoutRequest is the unified payout create request, mirroring the
// payment create request (nested paymentMethod/customer/url, feePaidBy).
type CreatePayoutRequest struct {
	ReferenceID  string                 `json:"referenceId"`
	PayoutMethod models.PayoutMethodRef `json:"payoutMethod"`
	AccountNo    string                 `json:"accountNo"`
	Amount       int64                  `json:"amount"`
	FeePaidBy    string                 `json:"feePaidBy"`
	URL          *models.PayoutURL      `json:"url"`
	Customer     *models.PayoutCustomer `json:"customer"`
	Description  string                 `json:"description"`
	Purpose      string                 `json:"purpose"`
}

// ---------------------------------------------------------------------------
// Inquiry
// ---------------------------------------------------------------------------

func (s *PayoutService) Inquiry(ctx context.Context, req *PayoutInquiryRequest, client *models.Client, isSandbox bool) (*models.PayoutInquiryResponse, error) {
	if !s.Available() {
		return nil, newPayoutError(503, "PAYOUT_UNAVAILABLE", "Payout provider is not configured", nil)
	}
	if client == nil {
		return nil, newPayoutError(401, "INVALID_TOKEN", "Unauthorized", nil)
	}
	if req == nil {
		return nil, newPayoutError(400, "MISSING_FIELD", "Invalid request body", nil)
	}

	mt, channel, bank, err := s.validateMethod(ctx, req.PayoutMethod)
	if err != nil {
		return nil, err
	}
	accountNo := strings.TrimSpace(req.AccountNo)
	if err := validatePayoutAccount(mt, accountNo); err != nil {
		return nil, err
	}

	inquiry, _, err := s.runInquiry(ctx, mt, channel, accountNo, 0, client.ID, isSandbox, bank)
	if err != nil {
		return nil, err
	}

	return &models.PayoutInquiryResponse{
		ID: inquiry.InquiryID,
		PayoutMethod: models.PayoutMethodRef{
			Type: mt,
			Code: channel,
		},
		AccountNumber: accountNo,
		AccountName:   derefString(inquiry.AccountName),
		ExpiredAt:     formatPayoutTime(inquiry.ExpiredAt),
	}, nil
}

// runInquiry tries each candidate provider for the method/channel until one
// returns a recipient name, persists the inquiry, and returns it together with
// the provider that succeeded.
func (s *PayoutService) runInquiry(
	ctx context.Context,
	mt models.MethodType,
	channel, accountNo string,
	amount int64,
	clientID int,
	isSandbox bool,
	bank *models.BankCode,
) (*models.PayoutInquiry, PayoutProviderClient, error) {
	candidates, err := s.selector.Candidates(ctx, mt, channel)
	if err != nil {
		return nil, nil, err
	}

	partnerRef := newPayoutPublicID("PLI")
	var lastErr error
	for _, adapter := range candidates {
		out, ierr := adapter.Inquiry(ctx, &PayoutInquiryInput{
			PartnerRef:    partnerRef,
			MethodType:    mt,
			ChannelCode:   channel,
			AccountNumber: accountNo,
			Amount:        amount,
		})
		if ierr != nil {
			lastErr = ierr
			var pErr *PayoutServiceError
			if errors.As(ierr, &pErr) && pErr.Retryable {
				continue // try next provider
			}
			return nil, nil, ierr // definitive (e.g. account not found)
		}

		bankName := out.BankName
		if bankName == "" && bank != nil {
			bankName = bank.Name
		}
		var tt *models.TransferType
		if out.TransferType != "" {
			t := out.TransferType
			tt = &t
		}
		inquiry := &models.PayoutInquiry{
			ClientID:      clientID,
			IsSandbox:     isSandbox,
			MethodType:    mt,
			ChannelCode:   channel,
			BankCode:      bankCodeFor(mt, channel),
			BankName:      stringPtr(bankName),
			AccountNumber: accountNo,
			AccountName:   stringPtr(out.AccountName),
			TransferType:  tt,
			Provider:      adapter.Code(),
			ProviderRef:   stringPtr(out.ProviderRef),
			ProviderData:  mergePayoutProviderData(nil, "inquiry", out.RawResponse),
			ExpiredAt:     time.Now().Add(payoutInquiryExpiry),
		}
		if err := s.createInquiryWithGeneratedID(ctx, inquiry); err != nil {
			return nil, nil, err
		}
		return inquiry, adapter, nil
	}

	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, newPayoutError(503, "PAYOUT_METHOD_UNAVAILABLE", "No payout provider could validate the account", nil)
}

// ---------------------------------------------------------------------------
// Create / Execute
// ---------------------------------------------------------------------------

func (s *PayoutService) Create(ctx context.Context, req *CreatePayoutRequest, client *models.Client, isSandbox bool) (*models.PayoutResponse, error) {
	if !s.Available() {
		return nil, newPayoutError(503, "PAYOUT_UNAVAILABLE", "Payout provider is not configured", nil)
	}
	if client == nil {
		return nil, newPayoutError(401, "INVALID_TOKEN", "Unauthorized", nil)
	}
	if req == nil {
		return nil, newPayoutError(400, "MISSING_FIELD", "Invalid request body", nil)
	}

	mt, channel, bank, err := s.validateMethod(ctx, req.PayoutMethod)
	if err != nil {
		return nil, err
	}
	minAmount := s.resolveMinAmount(ctx, mt, channel)
	if err := s.validateCreateRequest(req, mt, minAmount); err != nil {
		return nil, err
	}

	// Idempotency: reject duplicate referenceId for this client.
	if existing, err := s.repo.GetPayoutByReferenceID(ctx, client.ID, req.ReferenceID); err == nil && existing != nil {
		return nil, newPayoutError(400, "DUPLICATE_REFERENCE_ID", "Reference ID already exists", nil)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Always run inquiry first so the response carries the verified recipient
	// name and we confirm the account exists at the destination.
	inquiry, _, err := s.runInquiry(ctx, mt, channel, req.AccountNo, req.Amount, client.ID, isSandbox, bank)
	if err != nil {
		return nil, err
	}

	// Fee + amount math driven by feePaidBy.
	feePaidBy := models.FeePaidByMerchant
	if strings.EqualFold(strings.TrimSpace(req.FeePaidBy), string(models.FeePaidByCustomer)) {
		feePaidBy = models.FeePaidByCustomer
	}
	fee := s.resolveFee(ctx, mt, channel, inquiry)
	amount := req.Amount
	var sendAmount, totalAmount int64
	if feePaidBy == models.FeePaidByCustomer {
		sendAmount = amount - fee
		totalAmount = amount
		if sendAmount < minAmount {
			return nil, newPayoutError(400, "AMOUNT_TOO_LOW", "Amount net of fee is below the minimum", nil)
		}
	} else {
		sendAmount = amount
		totalAmount = amount + fee
	}

	var customerName, customerEmail, customerPhone *string
	if req.Customer != nil {
		customerName = stringPtr(req.Customer.Name)
		customerEmail = stringPtr(req.Customer.Email)
		customerPhone = stringPtr(req.Customer.Phone)
	}
	callbackURL := stringPtr(req.URL.Callback)

	payout := &models.Payout{
		ReferenceID:   req.ReferenceID,
		ClientID:      client.ID,
		IsSandbox:     isSandbox,
		MethodType:    mt,
		ChannelCode:   channel,
		TransferType:  inquiry.TransferType,
		Provider:      inquiry.Provider,
		BankCode:      bankCodeFor(mt, channel),
		BankName:      inquiry.BankName,
		AccountNumber: req.AccountNo,
		AccountName:   inquiry.AccountName,
		Amount:        amount,
		Fee:           fee,
		SendAmount:    sendAmount,
		TotalAmount:   totalAmount,
		FeePaidBy:     feePaidBy,
		Status:        models.PayoutStatusProcessing,
		PurposeCode:   stringPtr(req.Purpose),
		Description:   stringPtr(req.Description),
		CustomerName:  customerName,
		CustomerEmail: customerEmail,
		CustomerPhone: customerPhone,
		InquiryRowID:  intPtr(inquiry.ID),
		CallbackURL:   callbackURL,
		CallbackSent:  false,
	}
	if err := s.createPayoutWithGeneratedID(ctx, payout); err != nil {
		return nil, err
	}

	// Emit payout.processing immediately so clients learn the payout was
	// accepted, mirroring the payment lifecycle's pending notification.
	s.trySendProcessingCallback(ctx, payout)

	if err := s.submitWithFallback(ctx, payout); err != nil {
		return nil, err
	}

	return s.buildResponse(ctx, payout, bank), nil
}

// submitWithFallback tries the priority-ordered candidates until one accepts
// the payout. Retryable failures advance to the next provider; a definitive
// rejection fails the payout; an uncertain (5xx/timeout) error keeps it pending.
func (s *PayoutService) submitWithFallback(ctx context.Context, payout *models.Payout) error {
	candidates, err := s.selector.Candidates(ctx, payout.MethodType, payout.ChannelCode)
	if err != nil {
		return err
	}

	callbackURL := derefString(payout.CallbackURL)
	tt := models.TransferType("")
	if payout.TransferType != nil {
		tt = *payout.TransferType
	}

	var lastErr error
	for _, adapter := range candidates {
		out, perr := adapter.Pay(ctx, &PayoutExecInput{
			PartnerRef:    payout.PayoutID,
			MethodType:    payout.MethodType,
			ChannelCode:   payout.ChannelCode,
			AccountNumber: payout.AccountNumber,
			AccountName:   derefString(payout.AccountName),
			Amount:        payout.SendAmount,
			TransferType:  tt,
			Purpose:       derefString(payout.PurposeCode),
			Remark:        derefString(payout.Remark),
			CallbackURL:   callbackURL,
		})
		if perr == nil {
			sourceBank, sourceAcc := adapter.SourceAccount(payout.MethodType)
			payout.Provider = adapter.Code()
			payout.SourceBankCode = stringPtr(sourceBank)
			payout.SourceAccountNumber = stringPtr(sourceAcc)
			payout.ProviderRef = stringPtr(out.ProviderRef)
			if out.Fee > 0 {
				payout.Fee = out.Fee
			}
			payout.ProviderData = mergePayoutProviderData(payout.ProviderData, "submit", out.RawResponse)
			payout.Status = out.Status
			payout.FailedReason = nil
			payout.FailedCode = nil
			payout.FailedAt = nil
			return s.repo.UpdatePayout(ctx, payout)
		}

		lastErr = perr
		if isUncertainPayoutError(perr) {
			// Indeterminate: keep Processing, reconcile via status worker.
			log.Warn().Err(perr).Str("payout_id", payout.PayoutID).Str("provider", string(adapter.Code())).
				Msg("payout submission uncertain, keeping processing")
			payout.Provider = adapter.Code()
			payout.Status = models.PayoutStatusProcessing
			payout.ProviderData = mergePayoutProviderData(payout.ProviderData, "submit_error", payoutErrorPayload(perr))
			if err := s.repo.UpdatePayout(ctx, payout); err != nil {
				return err
			}
			return nil
		}

		appErr := mapPayoutSubmitError(perr)
		if appErr.Retryable {
			// Provider unavailable: record and try the next candidate.
			payout.ProviderData = mergePayoutProviderData(payout.ProviderData, "submit_error", payoutErrorPayload(perr))
			continue
		}

		// Definitive rejection: fail the payout.
		now := time.Now()
		payout.Provider = adapter.Code()
		payout.Status = models.PayoutStatusFailed
		payout.FailedReason = stringPtr(appErr.Message)
		payout.FailedCode = stringPtr(payoutFailedCode(perr))
		payout.FailedAt = &now
		payout.CallbackSent = false
		payout.ProviderData = mergePayoutProviderData(payout.ProviderData, "submit_error", payoutErrorPayload(perr))
		if err := s.repo.UpdatePayout(ctx, payout); err != nil {
			return err
		}
		s.trySendFinalCallback(ctx, payout)
		return appErr
	}

	// All candidates exhausted with retryable errors → keep Processing so the
	// status worker can retry rather than losing the payout.
	payout.Status = models.PayoutStatusProcessing
	if err := s.repo.UpdatePayout(ctx, payout); err != nil {
		return err
	}
	if lastErr != nil {
		log.Warn().Err(lastErr).Str("payout_id", payout.PayoutID).Msg("all payout providers unavailable, kept processing")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Read / status reconciliation
// ---------------------------------------------------------------------------

func (s *PayoutService) GetPayout(ctx context.Context, payoutID string, clientID int) (*models.PayoutResponse, error) {
	payout, err := s.repo.GetPayoutByPayoutID(ctx, strings.TrimSpace(payoutID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPayoutError(404, "PAYOUT_NOT_FOUND", "Payout not found", nil)
		}
		return nil, err
	}
	if payout.ClientID != clientID {
		return nil, newPayoutError(404, "PAYOUT_NOT_FOUND", "Payout not found", nil)
	}

	if s.Available() &&
		payout.Status == models.PayoutStatusProcessing &&
		time.Since(payout.UpdatedAt) >= 15*time.Second {
		if _, err := s.refreshStatus(ctx, payout, 0); err != nil {
			log.Warn().Err(err).Str("payout_id", payout.PayoutID).Msg("failed to refresh payout status on read")
		}
	}

	var bank *models.BankCode
	if payout.MethodType == models.MethodTypeBank {
		bank, _ = s.bankRepo.GetByCode(ctx, payout.ChannelCode)
	}
	return s.buildResponse(ctx, payout, bank), nil
}

func (s *PayoutService) ProcessPendingPayouts(ctx context.Context, staleAfter, maxAge time.Duration, limit int) error {
	if !s.Available() {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	updatedBefore := time.Now().Add(-staleAfter)
	createdAfter := time.Now().Add(-payoutStatusLookback)
	payouts, err := s.repo.ListPayoutsForStatusCheck(ctx, updatedBefore, createdAfter, limit)
	if err != nil {
		return err
	}
	for i := range payouts {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if _, err := s.refreshStatus(ctx, &payouts[i], maxAge); err != nil {
			log.Warn().Err(err).Str("payout_id", payouts[i].PayoutID).Msg("failed to reconcile payout status")
		}
	}
	return nil
}

func (s *PayoutService) refreshStatus(ctx context.Context, payout *models.Payout, maxAge time.Duration) (bool, error) {
	if payout == nil || !s.Available() {
		return false, nil
	}
	if payout.Status == models.PayoutStatusSuccess || payout.Status == models.PayoutStatusFailed {
		return false, nil
	}

	adapter, err := s.router.Get(payout.Provider)
	if err != nil {
		return false, nil
	}
	tt := models.TransferType("")
	if payout.TransferType != nil {
		tt = *payout.TransferType
	}

	out, err := adapter.Status(ctx, &PayoutStatusInput{
		PartnerRef:   payout.PayoutID,
		ProviderRef:  derefString(payout.ProviderRef),
		MethodType:   payout.MethodType,
		TransferType: tt,
	})
	if err != nil {
		if maxAge > 0 && time.Since(payout.CreatedAt) > maxAge {
			now := time.Now()
			payout.Status = models.PayoutStatusFailed
			payout.FailedReason = stringPtr("Payout timeout - unable to confirm final status with provider")
			payout.FailedCode = stringPtr(payoutFailedCode(err))
			payout.FailedAt = &now
			payout.ProviderData = mergePayoutProviderData(payout.ProviderData, "status_error", payoutErrorPayload(err))
			if updateErr := s.repo.UpdatePayout(ctx, payout); updateErr != nil {
				return false, updateErr
			}
			s.trySendFinalCallback(ctx, payout)
			return true, nil
		}
		payout.ProviderData = mergePayoutProviderData(payout.ProviderData, "status_error", payoutErrorPayload(err))
		_ = s.repo.UpdatePayout(ctx, payout)
		return false, nil
	}

	prevStatus := payout.Status
	payout.ProviderData = mergePayoutProviderData(payout.ProviderData, "status", out.RawResponse)
	if out.ProviderRef != "" {
		payout.ProviderRef = stringPtr(out.ProviderRef)
	}
	s.applyStatus(payout, out.Status, out.FailedReason, out.FailedCode)

	if err := s.repo.UpdatePayout(ctx, payout); err != nil {
		return false, err
	}
	if payout.Status == models.PayoutStatusSuccess || payout.Status == models.PayoutStatusFailed {
		s.trySendFinalCallback(ctx, payout)
	}
	return payout.Status != prevStatus, nil
}

// applyStatus transitions the payout to a terminal/processing state.
func (s *PayoutService) applyStatus(payout *models.Payout, status models.PayoutStatus, failedReason, failedCode string) {
	switch status {
	case models.PayoutStatusSuccess:
		if payout.Status != models.PayoutStatusSuccess {
			now := time.Now()
			payout.Status = models.PayoutStatusSuccess
			payout.CompletedAt = &now
			payout.FailedAt = nil
			payout.FailedReason = nil
			payout.FailedCode = nil
		}
	case models.PayoutStatusFailed:
		if payout.Status != models.PayoutStatusFailed {
			now := time.Now()
			payout.Status = models.PayoutStatusFailed
			payout.FailedReason = stringPtr(nonEmptyOrDefault(failedReason, "Payout failed"))
			payout.FailedCode = stringPtr(nonEmptyOrDefault(failedCode, ""))
			payout.FailedAt = &now
			payout.CompletedAt = nil
		}
	default:
		if payout.Status != models.PayoutStatusProcessing {
			payout.Status = models.PayoutStatusProcessing
		}
	}
}

// ---------------------------------------------------------------------------
// Callbacks (client-facing webhook delivery)
// ---------------------------------------------------------------------------

func (s *PayoutService) RetryPendingCallbacks(ctx context.Context, limit int) error {
	if s.callbackSvc == nil {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	payouts, err := s.repo.ListFinalCallbackPending(ctx, limit)
	if err != nil {
		return err
	}
	for i := range payouts {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		s.trySendFinalCallback(ctx, &payouts[i])
	}
	return nil
}

func (s *PayoutService) trySendFinalCallback(ctx context.Context, payout *models.Payout) {
	if s.callbackSvc == nil || payout == nil || payout.CallbackSent {
		return
	}
	handled, err := s.callbackSvc.Send(ctx, payout)
	if err != nil {
		_ = s.repo.IncrementCallbackAttempts(ctx, payout.ID)
		log.Warn().Err(err).Str("payout_id", payout.PayoutID).Msg("failed to deliver payout callback")
		return
	}
	if !handled {
		return
	}
	if err := s.repo.MarkCallbackSent(ctx, payout.ID); err != nil {
		log.Warn().Err(err).Str("payout_id", payout.PayoutID).Msg("failed to mark payout callback as sent")
		return
	}
	now := time.Now()
	payout.CallbackSent = true
	payout.CallbackSentAt = &now
}

// trySendProcessingCallback emits the payout.processing webhook right after the
// payout is accepted. It is best-effort and does not touch callback_sent (which
// tracks the authoritative final-state delivery), so a processing-callback
// failure never blocks the success/failed callback.
func (s *PayoutService) trySendProcessingCallback(ctx context.Context, payout *models.Payout) {
	if s.callbackSvc == nil || payout == nil {
		return
	}
	if err := s.callbackSvc.SendProcessing(ctx, payout); err != nil {
		log.Warn().Err(err).Str("payout_id", payout.PayoutID).Msg("failed to deliver payout.processing callback")
	}
}

// ---------------------------------------------------------------------------
// Validation + helpers
// ---------------------------------------------------------------------------

// validateMethod resolves the payout method into a normalized (type, channel)
// and, for BANK, the bank record. It rejects unknown methods/banks.
func (s *PayoutService) validateMethod(ctx context.Context, m models.PayoutMethodRef) (models.MethodType, string, *models.BankCode, error) {
	mt := models.MethodType(strings.ToUpper(strings.TrimSpace(string(m.Type))))
	code := strings.ToUpper(strings.TrimSpace(m.Code))
	if code == "" {
		return "", "", nil, newPayoutError(400, "MISSING_FIELD", "payoutMethod.code is required", nil)
	}
	switch mt {
	case models.MethodTypeBank:
		bank, err := s.bankRepo.GetByCode(ctx, code)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "", "", nil, newPayoutError(400, "INVALID_BANK_CODE", "Invalid bank code", nil)
			}
			return "", "", nil, err
		}
		if !bank.SupportDisbursement {
			return "", "", nil, newPayoutError(400, "INVALID_BANK_CODE", "Bank is not available for payout", nil)
		}
		return mt, code, bank, nil
	case models.MethodTypeEwallet:
		return mt, code, nil, nil
	default:
		return "", "", nil, newPayoutError(400, "INVALID_METHOD_TYPE", "payoutMethod.type must be BANK or EWALLET", nil)
	}
}

func (s *PayoutService) validateCreateRequest(req *CreatePayoutRequest, mt models.MethodType, minAmount int64) error {
	req.ReferenceID = strings.TrimSpace(req.ReferenceID)
	req.AccountNo = strings.TrimSpace(req.AccountNo)
	req.Description = strings.TrimSpace(req.Description)
	req.Purpose = strings.TrimSpace(req.Purpose)

	switch {
	case req.ReferenceID == "":
		return newPayoutError(400, "MISSING_FIELD", "referenceId is required", nil)
	case req.URL == nil || strings.TrimSpace(req.URL.Callback) == "":
		return newPayoutError(400, "MISSING_FIELD", "url.callback is required", nil)
	case req.Amount < minAmount:
		return newPayoutError(400, "AMOUNT_TOO_LOW", "Amount is below minimum payout amount", nil)
	case len(req.Description) > 200:
		return newPayoutError(400, "INVALID_DESCRIPTION", "Description must be 200 characters or fewer", nil)
	}
	if req.FeePaidBy != "" &&
		!strings.EqualFold(req.FeePaidBy, string(models.FeePaidByMerchant)) &&
		!strings.EqualFold(req.FeePaidBy, string(models.FeePaidByCustomer)) {
		return newPayoutError(400, "INVALID_FEE_PAID_BY", "feePaidBy must be merchant or customer", nil)
	}
	if req.Purpose != "" {
		if _, ok := payoutPurposeDescriptions[req.Purpose]; !ok {
			return newPayoutError(400, "INVALID_PURPOSE", "Invalid purpose code", nil)
		}
	}
	return validatePayoutAccount(mt, req.AccountNo)
}

// resolveMinAmount returns the per-channel minimum payout amount from the
// payout_methods catalog. BANK uses the shared 'DEFAULT' row; each e-wallet has
// its own row (e.g. DANA accepts payouts from a lower floor than other wallets).
// Falls back to payoutMinAmount when no row matches.
func (s *PayoutService) resolveMinAmount(ctx context.Context, mt models.MethodType, channel string) int64 {
	if s.methodRepo == nil {
		return payoutMinAmount
	}
	code := channel
	if mt == models.MethodTypeBank {
		code = "DEFAULT"
	}
	method, err := s.methodRepo.GetMethod(ctx, mt, code)
	if err != nil || method == nil {
		return payoutMinAmount
	}
	if method.MinAmount <= 0 {
		return payoutMinAmount
	}
	return int64(method.MinAmount)
}

// resolveFee picks the fee for a payout: the provider-reported inquiry fee when
// available, otherwise the configured default bank fee for interbank transfers.
func (s *PayoutService) resolveFee(_ context.Context, mt models.MethodType, _ string, inquiry *models.PayoutInquiry) int64 {
	if mt == models.MethodTypeBank && inquiry != nil && inquiry.TransferType != nil &&
		*inquiry.TransferType == models.TransferTypeInterbank {
		return s.defaultBankFee
	}
	return 0
}

func (s *PayoutService) createInquiryWithGeneratedID(ctx context.Context, inquiry *models.PayoutInquiry) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		inquiry.InquiryID = uuid.New().String()
		lastErr = s.repo.CreateInquiry(ctx, inquiry)
		if lastErr == nil {
			return nil
		}
		if !isUniqueViolation(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

func (s *PayoutService) createPayoutWithGeneratedID(ctx context.Context, payout *models.Payout) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		payout.PayoutID = uuid.New().String()
		lastErr = s.repo.CreatePayout(ctx, payout)
		if lastErr == nil {
			return nil
		}
		if !isUniqueViolation(lastErr) {
			if isReferenceUniqueViolation(lastErr) {
				return newPayoutError(400, "DUPLICATE_REFERENCE_ID", "Reference ID already exists", lastErr)
			}
			return lastErr
		}
	}
	return lastErr
}

func (s *PayoutService) buildResponse(ctx context.Context, payout *models.Payout, bank *models.BankCode) *models.PayoutResponse {
	_ = ctx
	_ = bank
	resp := &models.PayoutResponse{
		ID:          payout.PayoutID,
		ReferenceID: payout.ReferenceID,
		Status:      string(payout.Status),
		PayoutMethod: models.PayoutMethodRef{
			Type: payout.MethodType,
			Code: payout.ChannelCode,
		},
		AccountNumber: payout.AccountNumber,
		AccountName:   derefString(payout.AccountName),
		Amount: models.PayoutAmount{
			Subtotal: payout.Amount,
			Fee:      payout.Fee,
			Total:    payout.TotalAmount,
		},
		FeePaidBy:   string(payout.FeePaidBy),
		Description: derefString(payout.Description),
		CreatedAt:   formatPayoutTime(payout.CreatedAt),
	}
	if name := derefString(payout.CustomerName); name != "" || derefString(payout.CustomerEmail) != "" || derefString(payout.CustomerPhone) != "" {
		resp.Customer = &models.PayoutCustomer{
			Name:  derefString(payout.CustomerName),
			Email: derefString(payout.CustomerEmail),
			Phone: derefString(payout.CustomerPhone),
		}
	}
	if payout.CompletedAt != nil {
		resp.CompletedAt = formatPayoutTime(*payout.CompletedAt)
	}
	if payout.FailedAt != nil {
		resp.FailedAt = formatPayoutTime(*payout.FailedAt)
	}
	if payout.Status == models.PayoutStatusFailed {
		resp.FailedReason = derefString(payout.FailedReason)
		resp.FailedCode = derefString(payout.FailedCode)
	}
	return resp
}

// ListMethods returns the available payout channels grouped by method type.
// BANK entries are the disbursement-enabled banks (they share the BANK/DEFAULT
// catalog row for fee + amount limits); EWALLET entries come straight from the
// payout_methods catalog. Maintenance/inactive channels are surfaced/hidden the
// same way the payment method list does.
func (s *PayoutService) ListMethods(ctx context.Context) (*models.PayoutMethodsResponse, error) {
	resp := &models.PayoutMethodsResponse{
		Bank:    []models.PayoutMethodEntry{},
		Ewallet: []models.PayoutMethodEntry{},
	}

	// BANK: each disbursement-enabled bank, with limits/fee from BANK/DEFAULT.
	bankDefault, _ := s.methodRepo.GetMethod(ctx, models.MethodTypeBank, "DEFAULT")
	banks, err := s.bankRepo.GetDisbursementBanks(ctx)
	if err != nil {
		return nil, err
	}
	for i := range banks {
		entry := models.PayoutMethodEntry{
			Code:      banks[i].Code,
			Name:      banks[i].Name,
			MinAmount: int(payoutMinAmount),
		}
		if bankDefault != nil {
			applyCatalogToEntry(&entry, bankDefault)
		}
		resp.Bank = append(resp.Bank, entry)
	}

	// EWALLET: active catalog rows for the e-wallet method type.
	methods, err := s.methodRepo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	for i := range methods {
		if methods[i].MethodType != models.MethodTypeEwallet {
			continue
		}
		entry := models.PayoutMethodEntry{
			Code: methods[i].Code,
			Name: methods[i].Name,
		}
		applyCatalogToEntry(&entry, &methods[i])
		resp.Ewallet = append(resp.Ewallet, entry)
	}
	return resp, nil
}

// applyCatalogToEntry copies fee + amount-limit config from a catalog row onto a
// public method entry.
func applyCatalogToEntry(entry *models.PayoutMethodEntry, m *models.PayoutMethodCatalog) {
	entry.FeeType = m.FeeType
	entry.FeeFlat = m.FeeFlat
	entry.FeePercent = m.FeePercent
	entry.FeeMin = m.FeeMin
	entry.FeeMax = m.FeeMax
	if m.MinAmount > 0 {
		entry.MinAmount = m.MinAmount
	}
	entry.MaxAmount = m.MaxAmount
	entry.LogoURL = derefString(m.LogoURL)
	entry.IsMaintenance = m.IsMaintenance
}

func validatePayoutAccount(mt models.MethodType, accountNo string) error {
	accountNo = strings.TrimSpace(accountNo)
	if accountNo == "" {
		return newPayoutError(400, "MISSING_FIELD", "accountNo is required", nil)
	}
	if !isNumericString(accountNo) || len(accountNo) < 5 || len(accountNo) > 34 {
		return newPayoutError(400, "INVALID_ACCOUNT_NUMBER", "Invalid account number", nil)
	}
	_ = mt
	return nil
}

// bankCodeFor returns the bank_code persisted on a payout: the channel for
// BANK, empty for e-wallet.
func bankCodeFor(mt models.MethodType, channel string) string {
	if mt == models.MethodTypeBank {
		return channel
	}
	return ""
}

func payoutFailedCode(err error) string {
	if info, ok := extractSNAPError(err); ok && info.ResponseCode != "" {
		return info.ResponseCode
	}
	return "UNKNOWN"
}
