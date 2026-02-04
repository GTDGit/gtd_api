package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/cache"
	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/utils"
	"github.com/GTDGit/gtd_api/pkg/digiflazz"
)

// isDuplicateKeyError checks if the error is a PostgreSQL unique constraint violation.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// PostgreSQL unique violation error codes/messages
	return strings.Contains(errStr, "duplicate key") ||
		strings.Contains(errStr, "unique constraint") ||
		strings.Contains(errStr, "23505") // PostgreSQL error code for unique_violation
}

// TransactionService contains business logic for transactions.
type TransactionService struct {
	trxRepo       *repository.TransactionRepository
	productRepo   *repository.ProductRepository
	skuRepo       *repository.SKURepository
	callbackRepo  *repository.CallbackRepository
	digiflazzProd *digiflazz.Client
	digiflazzDev  *digiflazz.Client
	productSvc    *ProductService
	callbackSvc   *CallbackService
	sandboxMapper *SandboxMapper
	inquiryCache  *cache.InquiryCache
}

// NewTransactionService constructs a TransactionService.
func NewTransactionService(
	trxRepo *repository.TransactionRepository,
	productRepo *repository.ProductRepository,
	skuRepo *repository.SKURepository,
	callbackRepo *repository.CallbackRepository,
	digiProd *digiflazz.Client,
	digiDev *digiflazz.Client,
	productSvc *ProductService,
	callbackSvc *CallbackService,
	inquiryCache *cache.InquiryCache,
) *TransactionService {
	return &TransactionService{
		trxRepo:       trxRepo,
		productRepo:   productRepo,
		skuRepo:       skuRepo,
		callbackRepo:  callbackRepo,
		digiflazzProd: digiProd,
		digiflazzDev:  digiDev,
		productSvc:    productSvc,
		callbackSvc:   callbackSvc,
		sandboxMapper: NewSandboxMapper(),
		inquiryCache:  inquiryCache,
	}
}

// getDigiflazzClient returns the appropriate Digiflazz client based on sandbox mode.
func (s *TransactionService) getDigiflazzClient(isSandbox bool) *digiflazz.Client {
	if isSandbox {
		return s.digiflazzDev
	}
	return s.digiflazzProd
}

// CreateTransactionRequest input
type CreateTransactionRequest struct {
	ReferenceID   string `json:"referenceId" binding:"required"`
	SkuCode       string `json:"skuCode" binding:"required"`
	CustomerNo    string `json:"customerNo" binding:"required"`
	Type          string `json:"type" binding:"required,oneof=prepaid inquiry payment"`
	TransactionID string `json:"transactionId"` // Required for payment
}

// CreateTransaction routes processing based on req.Type.
func (s *TransactionService) CreateTransaction(ctx context.Context, req *CreateTransactionRequest, client *models.Client, isSandbox bool) (*models.Transaction, error) {
	switch req.Type {
	case "prepaid":
		return s.processPrepaid(ctx, req, client, isSandbox)
	case "inquiry":
		return s.processInquiry(ctx, req, client, isSandbox)
	case "payment":
		return s.processPayment(ctx, req, client, isSandbox)
	default:
		return nil, utils.ErrInvalidType
	}
}

// processPrepaid handles prepaid top-up workflow.
func (s *TransactionService) processPrepaid(ctx context.Context, req *CreateTransactionRequest, client *models.Client, isSandbox bool) (*models.Transaction, error) {
	// 1. Validate referenceId unique
	exists, err := s.trxRepo.ExistsReferenceID(client.ID, req.ReferenceID)
	if err == nil && exists {
		return nil, utils.ErrDuplicateReferenceID
	} else if err != nil {
		log.Error().Err(err).Msg("ExistsReferenceID failed")
	}

	// 2. Get product
	product, err := s.productRepo.GetBySKUCode(req.SkuCode)
	if err != nil || product == nil {
		return nil, utils.ErrInvalidSKU
	}

	// 3. Get available SKUs
	skus, err := s.productSvc.GetAvailableSKUs(product.ID)
	if err != nil || len(skus) == 0 {
		return nil, utils.ErrNoAvailableSKU
	}

	// 4. Generate transaction ID
	trxID, err := s.trxRepo.GenerateTransactionID()
	if err != nil {
		return nil, err
	}

	// 5. Create transaction record
	trx := &models.Transaction{
		TransactionID: trxID,
		ReferenceID:   req.ReferenceID,
		ClientID:      client.ID,
		ProductID:     product.ID,
		SkuCode:       product.SkuCode,
		CustomerNo:    req.CustomerNo,
		Type:          models.TrxTypePrepaid,
		Status:        models.StatusProcessing,
		IsSandbox:     isSandbox,
	}

	if err := s.trxRepo.Create(trx); err != nil {
		// Check for duplicate reference_id (unique constraint violation)
		if isDuplicateKeyError(err) {
			return nil, utils.ErrDuplicateReferenceID
		}
		return nil, err
	}

	// 6. Try each SKU
	return s.tryAllSKUs(ctx, trx, skus, isSandbox, 0)
}

// tryAllSKUs attempts transaction with each SKU until success/pending/fatal.
// CRITICAL: ref_id handling for Digiflazz idempotency:
// - Same ref_id to Digiflazz = safe (returns previous response)
// - Different ref_id = NEW transaction (dangerous if previous actually succeeded)
// We only increment refIDSuffix when we get a DEFINITE response requiring new ref_id (RC 49).
// refIDSuffixStart is used when retrying from callback to continue suffix numbering.
func (s *TransactionService) tryAllSKUs(ctx context.Context, trx *models.Transaction, skus []models.SKU, isSandbox bool, refIDSuffixStart int) (*models.Transaction, error) {
	refIDSuffix := refIDSuffixStart
	networkRetryCount := 0
	const maxNetworkRetries = 2 // Max retries per SKU on network error

	for i := 0; i < len(skus); i++ {
		sku := skus[i]

		// Generate Digiflazz ref_id - UNIQUE per SKU attempt
		digiRefID := trx.TransactionID
		if refIDSuffix > 0 {
			digiRefID = fmt.Sprintf("%s-%d", trx.TransactionID, refIDSuffix)
		}

		// Determine SKU and customer number to send to Digiflazz
		digiSKU := sku.DigiSkuCode
		digiCustomerNo := trx.CustomerNo

		// In sandbox mode, use test SKU and customer number
		if isSandbox {
			testSKU, testCustomerNo := s.sandboxMapper.GetTestMapping(sku.DigiSkuCode, trx.Type)
			digiSKU = testSKU
			digiCustomerNo = testCustomerNo
		}

		// CRITICAL: Store digiRefID BEFORE making API call for recovery
		trx.DigiRefID = &digiRefID
		trx.SkuID = &sku.ID
		_ = s.trxRepo.Update(trx)

		// Call Digiflazz with test data in sandbox mode, real data otherwise
		digi := s.getDigiflazzClient(isSandbox)
		resp, err := digi.Topup(ctx, digiSKU, digiCustomerNo, digiRefID, isSandbox)

		// Log attempt
		s.logAttempt(trx.ID, sku.ID, digiRefID, map[string]any{
			"buyer_sku_code": digiSKU,
			"customer_no":    digiCustomerNo,
			"ref_id":         digiRefID,
			"testing":        isSandbox,
		}, resp, err)

		if err != nil {
			// CRITICAL: Network error - DON'T change ref_id!
			// Digiflazz might have processed it. Retry with SAME ref_id is safe.
			log.Warn().
				Err(err).
				Str("transaction_id", trx.TransactionID).
				Str("digi_ref_id", digiRefID).
				Int("network_retry", networkRetryCount).
				Msg("Network error calling Digiflazz, will retry with same ref_id")

			networkRetryCount++
			if networkRetryCount <= maxNetworkRetries {
				// Wait briefly then retry with SAME ref_id (safe - Digiflazz idempotent)
				select {
				case <-ctx.Done():
					return s.handleAllSKUsFailed(trx)
				case <-time.After(5 * time.Second):
					i-- // Retry same SKU
					continue
				}
			}

			// Max network retries reached for this SKU, move to next SKU with new ref_id
			// This is necessary because we can't know if Digiflazz processed it
			log.Warn().
				Str("transaction_id", trx.TransactionID).
				Str("sku", sku.DigiSkuCode).
				Msg("Max network retries reached, switching to next SKU")

			refIDSuffix++
			networkRetryCount = 0 // Reset for next SKU
			continue
		}

		// Got response - reset network retry counter
		networkRetryCount = 0

		// Check RC
		switch {
		case digiflazz.IsSuccess(resp.RC):
			return s.handleSuccess(trx, &sku, resp)
		case digiflazz.IsPending(resp.RC):
			return s.handlePending(trx, &sku, resp)
		case digiflazz.IsFatal(resp.RC):
			return s.handleFatal(trx, resp)
		case digiflazz.NeedsNewRefID(resp.RC):
			// RC 49: Ref ID sudah terpakai - HARUS ganti ref_id
			log.Info().
				Str("transaction_id", trx.TransactionID).
				Str("rc", resp.RC).
				Msg("RC 49: Ref ID not unique, generating new suffix")
			refIDSuffix++
			i-- // Retry SAME SKU with new ref_id
			continue
		case digiflazz.IsRetryableWait(resp.RC):
			// RC 85/86: Need to wait before retrying on SAME SKU
			log.Info().
				Str("transaction_id", trx.TransactionID).
				Str("rc", resp.RC).
				Str("sku", sku.DigiSkuCode).
				Msg("Rate limited, waiting 60s before retry on same SKU")

			select {
			case <-ctx.Done():
				return s.handleAllSKUsFailed(trx)
			case <-time.After(60 * time.Second):
				// Retry same SKU - but need new ref_id because this ref_id was "used"
				refIDSuffix++
				i-- // Don't advance to next SKU, retry current one
				continue
			}
		case digiflazz.IsRetryableSwitchSKU(resp.RC):
			// Switch to next SKU with new ref_id
			log.Info().
				Str("transaction_id", trx.TransactionID).
				Str("rc", resp.RC).
				Str("sku", sku.DigiSkuCode).
				Msg("SKU failed, switching to next SKU")
			refIDSuffix++
			continue
		default:
			// Unknown RC, treat as retryable switch
			log.Warn().
				Str("transaction_id", trx.TransactionID).
				Str("rc", resp.RC).
				Msg("Unknown RC code, switching to next SKU")
			refIDSuffix++
			continue
		}
	}
	// All SKUs failed
	return s.handleAllSKUsFailed(trx)
}

// handleSuccess updates trx to success and dispatches callback.
func (s *TransactionService) handleSuccess(trx *models.Transaction, sku *models.SKU, resp *digiflazz.TransactionResponse) (*models.Transaction, error) {
	now := time.Now()
	trx.SkuID = &sku.ID
	trx.Status = models.StatusSuccess
	if resp.SN != "" {
		trx.SerialNumber = &resp.SN
	}
	trx.Amount = &resp.Price
	trx.ProcessedAt = &now
	if resp.RefID != "" {
		trx.DigiRefID = &resp.RefID
	}
	_ = s.trxRepo.Update(trx)

	// Send callback to client asynchronously
	go s.callbackSvc.SendCallback(trx, "transaction.success")
	return trx, nil
}

// handlePending updates trx to processing and stores digi ref id.
func (s *TransactionService) handlePending(trx *models.Transaction, sku *models.SKU, resp *digiflazz.TransactionResponse) (*models.Transaction, error) {
	trx.SkuID = &sku.ID
	trx.Status = models.StatusProcessing
	trx.Amount = &resp.Price
	if resp.RefID != "" {
		trx.DigiRefID = &resp.RefID
	}
	_ = s.trxRepo.Update(trx)
	return trx, nil
}

// handleFatal updates trx to failed and dispatches callback.
func (s *TransactionService) handleFatal(trx *models.Transaction, resp *digiflazz.TransactionResponse) (*models.Transaction, error) {
	now := time.Now()
	trx.Status = models.StatusFailed
	if resp.Message != "" {
		msg := resp.Message
		trx.FailedReason = &msg
	}
	if resp.RC != "" {
		rc := resp.RC
		trx.FailedCode = &rc
	}
	trx.ProcessedAt = &now
	_ = s.trxRepo.Update(trx)

	go s.callbackSvc.SendCallback(trx, "transaction.failed")
	return trx, nil
}

// handleAllSKUsFailed marks transaction as failed when all SKUs have been exhausted.
// This happens when all available SKUs return retryable errors - since we've already
// tried all sellers, there's no point in waiting. Mark as failed immediately.
func (s *TransactionService) handleAllSKUsFailed(trx *models.Transaction) (*models.Transaction, error) {
	now := time.Now()
	reason := "All available SKUs failed"
	trx.Status = models.StatusFailed
	trx.FailedReason = &reason
	trx.ProcessedAt = &now
	trx.NextRetryAt = nil
	_ = s.trxRepo.Update(trx)

	go s.callbackSvc.SendCallback(trx, "transaction.failed")
	return trx, nil
}

// processInquiry handles postpaid inquiry using Redis cache.
func (s *TransactionService) processInquiry(ctx context.Context, req *CreateTransactionRequest, client *models.Client, isSandbox bool) (*models.Transaction, error) {
	// Product must exist
	product, err := s.productRepo.GetBySKUCode(req.SkuCode)
	if err != nil || product == nil {
		return nil, utils.ErrInvalidSKU
	}

	// Check if inquiry already cached (same client, customer, sku, refId)
	cached, err := s.inquiryCache.GetByCacheKey(ctx, client.ID, req.CustomerNo, req.SkuCode, req.ReferenceID)
	if err == nil && cached != nil {
		log.Debug().Str("transactionId", cached.TransactionID).Msg("inquiry cache hit")
		// Return cached inquiry as transaction model
		return s.cachedInquiryToTransaction(cached, client.ID, product.ID), nil
	} else if err != nil && err != redis.Nil {
		log.Warn().Err(err).Msg("failed to get inquiry cache")
	}

	// Cache miss - generate new transaction ID
	trxID, err := s.trxRepo.GenerateTransactionID()
	if err != nil {
		return nil, err
	}

	// Determine SKU and customer number to send to Digiflazz
	digiSKU := req.SkuCode
	digiCustomerNo := req.CustomerNo

	// In sandbox mode, use test SKU and customer number
	if isSandbox {
		testSKU, testCustomerNo := s.sandboxMapper.GetTestMapping(req.SkuCode, models.TrxTypeInquiry)
		digiSKU = testSKU
		digiCustomerNo = testCustomerNo
	}

	// Make inquiry call with test data in sandbox mode, real data otherwise
	digi := s.getDigiflazzClient(isSandbox)
	resp, err := digi.Inquiry(ctx, digiSKU, digiCustomerNo, trxID, isSandbox)

	// Log attempt (no database transaction yet, log to transaction_logs later if needed)
	log.Info().
		Str("transactionId", trxID).
		Str("buyer_sku_code", digiSKU).
		Str("customer_no", digiCustomerNo).
		Bool("sandbox", isSandbox).
		Msg("inquiry request to digiflazz")

	if err != nil {
		log.Error().Err(err).Str("transactionId", trxID).Msg("inquiry failed")
		return nil, fmt.Errorf("inquiry failed: %w", err)
	}

	// Expiration end of day WIB
	wib := time.FixedZone("WIB", 7*3600) // UTC+7
	nowWIB := time.Now().In(wib)
	eod := time.Date(nowWIB.Year(), nowWIB.Month(), nowWIB.Day(), 23, 59, 59, 0, wib)

	// Process RC
	if !digiflazz.IsSuccess(resp.RC) {
		log.Warn().Str("rc", resp.RC).Str("message", resp.Message).Msg("inquiry not successful")
		return nil, fmt.Errorf("inquiry failed: %s", resp.Message)
	}

	// Store in Redis cache
	inquiryData := &cache.InquiryData{
		TransactionID: trxID,
		ReferenceID:   req.ReferenceID,
		ClientID:      client.ID,
		ProductID:     product.ID,
		CustomerNo:    req.CustomerNo,
		SKUCode:       req.SkuCode,
		Amount:        resp.Price,
		Admin:         resp.Admin,
		CustomerName:  resp.CustomerName,
		Description:   resp.Desc,
		ExpiredAt:     eod,
	}

	if err := s.inquiryCache.Set(ctx, inquiryData); err != nil {
		log.Error().Err(err).Msg("failed to cache inquiry")
		// Continue anyway, inquiry succeeded
	}

	// Return transaction model for response
	return s.cachedInquiryToTransaction(inquiryData, client.ID, product.ID), nil
}

// processPayment handles postpaid payment after a successful inquiry.
func (s *TransactionService) processPayment(ctx context.Context, req *CreateTransactionRequest, client *models.Client, isSandbox bool) (*models.Transaction, error) {
	// 1. Get inquiry from Redis
	inquiryData, err := s.inquiryCache.GetByTransactionID(ctx, req.TransactionID)
	if err == redis.Nil {
		return nil, utils.ErrTransactionNotFound
	} else if err != nil {
		log.Error().Err(err).Str("transactionId", req.TransactionID).Msg("failed to get inquiry from cache")
		return nil, fmt.Errorf("failed to get inquiry: %w", err)
	}

	// 2. Validate
	if inquiryData.ReferenceID != req.ReferenceID {
		return nil, utils.ErrReferenceMismatch
	}
	if inquiryData.CustomerNo != req.CustomerNo {
		return nil, utils.ErrCustomerMismatch
	}
	if inquiryData.ClientID != client.ID {
		return nil, utils.ErrTransactionNotFound
	}
	// Validate SKU code belongs to same product
	product, err := s.productRepo.GetBySKUCode(req.SkuCode)
	if err != nil || product == nil || product.ID != inquiryData.ProductID {
		return nil, utils.ErrSkuMismatch
	}
	if inquiryData.ExpiredAt.Before(time.Now()) {
		return nil, utils.ErrInquiryExpired
	}

	// 3. Create payment transaction in database (this one we store!)
	payTrxID, err := s.trxRepo.GenerateTransactionID()
	if err != nil {
		return nil, err
	}
	payment := &models.Transaction{
		TransactionID: payTrxID,
		ReferenceID:   req.ReferenceID,
		ClientID:      client.ID,
		ProductID:     inquiryData.ProductID,
		SkuCode:       inquiryData.SKUCode,
		CustomerNo:    inquiryData.CustomerNo,
		Type:          models.TrxTypePayment,
		Status:        models.StatusProcessing,
		IsSandbox:     isSandbox,
		// Note: InquiryID is nil since inquiry is not in database
	}
	if err := s.trxRepo.Create(payment); err != nil {
		return nil, err
	}

	// 4. Determine SKU and customer number to send to Digiflazz
	digiSKU := req.SkuCode
	digiCustomerNo := inquiryData.CustomerNo

	// In sandbox mode, use test SKU and customer number
	if isSandbox {
		testSKU, testCustomerNo := s.sandboxMapper.GetTestMapping(req.SkuCode, payment.Type)
		digiSKU = testSKU
		digiCustomerNo = testCustomerNo
	}

	// Call Digiflazz.Payment() with SAME ref_id as inquiry
	refID := inquiryData.TransactionID
	digi := s.getDigiflazzClient(isSandbox)
	resp, err := digi.Payment(ctx, digiSKU, digiCustomerNo, refID, isSandbox)

	// Log attempt (no SKU context for pasca)
	s.logAttempt(payment.ID, 0, refID, map[string]any{
		"buyer_sku_code": digiSKU,
		"customer_no":    digiCustomerNo,
		"ref_id":         refID,
		"testing":        isSandbox,
	}, resp, err)

	if err != nil {
		// Schedule retry
		return s.handleAllSKUsFailed(payment)
	}

	if digiflazz.IsSuccess(resp.RC) {
		now := time.Now()
		payment.Status = models.StatusSuccess
		if resp.SN != "" {
			payment.SerialNumber = &resp.SN
		}
		payment.Amount = &resp.Price
		payment.ProcessedAt = &now
		payment.DigiRefID = &refID
		_ = s.trxRepo.Update(payment)

		// Delete inquiry from Redis cache (already paid!)
		if err := s.inquiryCache.Delete(ctx, inquiryData); err != nil {
			log.Warn().Err(err).Str("transactionId", inquiryData.TransactionID).Msg("failed to delete inquiry cache")
		}

		go s.callbackSvc.SendCallback(payment, "transaction.success")
		return payment, nil
	}

	if digiflazz.IsPending(resp.RC) {
		payment.Status = models.StatusProcessing
		payment.Amount = &resp.Price
		payment.DigiRefID = &refID
		_ = s.trxRepo.Update(payment)
		return payment, nil
	}

	// Fatal/other
	now := time.Now()
	payment.Status = models.StatusFailed
	msg := resp.Message
	payment.FailedReason = &msg
	payment.ProcessedAt = &now
	payment.DigiRefID = &refID
	_ = s.trxRepo.Update(payment)
	go s.callbackSvc.SendCallback(payment, "transaction.failed")
	return payment, nil
}

// GetTransaction retrieves a transaction visible to the given client.
func (s *TransactionService) GetTransaction(transactionID string, clientID int) (*models.Transaction, error) {
	trx, err := s.trxRepo.GetByTransactionID(transactionID)
	if err != nil || trx == nil {
		return nil, utils.ErrTransactionNotFound
	}
	if trx.ClientID != clientID {
		return nil, utils.ErrTransactionNotFound
	}
	return trx, nil
}

// RetryTransaction retries a pending/processing transaction.
// CRITICAL: Must check if there's a pending transaction at Digiflazz first to avoid duplicates.
func (s *TransactionService) RetryTransaction(ctx context.Context, trx *models.Transaction) (*models.Transaction, error) {
	// If transaction has a digi_ref_id and status is Processing,
	// it means Digiflazz is still processing. Don't retry - wait for callback.
	if trx.Status == models.StatusProcessing && trx.DigiRefID != nil && *trx.DigiRefID != "" {
		log.Info().
			Str("transaction_id", trx.TransactionID).
			Str("digi_ref_id", *trx.DigiRefID).
			Msg("Transaction is processing at Digiflazz, cannot retry - wait for callback")
		return trx, fmt.Errorf("transaction is processing at Digiflazz, please wait for callback")
	}

	skus, err := s.productSvc.GetAvailableSKUs(trx.ProductID)
	if err != nil || len(skus) == 0 {
		return s.handleAllSKUsFailed(trx)
	}

	// Start retry with suffix based on existing digi_ref_id to avoid collision
	return s.tryAllSKUsWithOffset(ctx, trx, skus, trx.IsSandbox, s.extractRefIDSuffix(trx.DigiRefID)+1)
}

// extractRefIDSuffix extracts the numeric suffix from a digi_ref_id.
// "GRB-20250203-000001" returns 0, "GRB-20250203-000001-3" returns 3.
func (s *TransactionService) extractRefIDSuffix(digiRefID *string) int {
	if digiRefID == nil || *digiRefID == "" {
		return 0
	}
	ref := *digiRefID
	// Count dashes - format is GRB-YYYYMMDD-NNNNNN[-suffix]
	dashCount := 0
	lastDashPos := -1
	for i, c := range ref {
		if c == '-' {
			dashCount++
			lastDashPos = i
		}
	}
	if dashCount < 3 || lastDashPos <= 0 {
		return 0 // No suffix
	}
	suffixStr := ref[lastDashPos+1:]
	var suffix int
	fmt.Sscanf(suffixStr, "%d", &suffix)
	return suffix
}

// tryAllSKUsWithOffset is like tryAllSKUs but starts with a specific refIDSuffix offset.
func (s *TransactionService) tryAllSKUsWithOffset(ctx context.Context, trx *models.Transaction, skus []models.SKU, isSandbox bool, startSuffix int) (*models.Transaction, error) {
	refIDSuffix := startSuffix
	networkRetryCount := 0
	const maxNetworkRetries = 2

	log.Info().
		Str("transaction_id", trx.TransactionID).
		Int("start_suffix", startSuffix).
		Msg("Starting retry with offset")

	for i := 0; i < len(skus); i++ {
		sku := skus[i]

		digiRefID := trx.TransactionID
		if refIDSuffix > 0 {
			digiRefID = fmt.Sprintf("%s-%d", trx.TransactionID, refIDSuffix)
		}

		digiSKU := sku.DigiSkuCode
		digiCustomerNo := trx.CustomerNo

		if isSandbox {
			testSKU, testCustomerNo := s.sandboxMapper.GetTestMapping(sku.DigiSkuCode, trx.Type)
			digiSKU = testSKU
			digiCustomerNo = testCustomerNo
		}

		trx.DigiRefID = &digiRefID
		trx.SkuID = &sku.ID
		_ = s.trxRepo.Update(trx)

		digi := s.getDigiflazzClient(isSandbox)
		resp, err := digi.Topup(ctx, digiSKU, digiCustomerNo, digiRefID, isSandbox)

		s.logAttempt(trx.ID, sku.ID, digiRefID, map[string]any{
			"buyer_sku_code": digiSKU,
			"customer_no":    digiCustomerNo,
			"ref_id":         digiRefID,
			"testing":        isSandbox,
			"is_retry":       true,
		}, resp, err)

		if err != nil {
			log.Warn().Err(err).Str("transaction_id", trx.TransactionID).Str("digi_ref_id", digiRefID).Msg("Network error on retry")
			networkRetryCount++
			if networkRetryCount <= maxNetworkRetries {
				select {
				case <-ctx.Done():
					return s.handleAllSKUsFailed(trx)
				case <-time.After(5 * time.Second):
					i--
					continue
				}
			}
			refIDSuffix++
			networkRetryCount = 0
			continue
		}

		networkRetryCount = 0

		switch {
		case digiflazz.IsSuccess(resp.RC):
			return s.handleSuccess(trx, &sku, resp)
		case digiflazz.IsPending(resp.RC):
			return s.handlePending(trx, &sku, resp)
		case digiflazz.IsFatal(resp.RC):
			return s.handleFatal(trx, resp)
		case digiflazz.NeedsNewRefID(resp.RC):
			refIDSuffix++
			i--
			continue
		case digiflazz.IsRetryableWait(resp.RC):
			select {
			case <-ctx.Done():
				return s.handleAllSKUsFailed(trx)
			case <-time.After(60 * time.Second):
				refIDSuffix++
				i--
				continue
			}
		case digiflazz.IsRetryableSwitchSKU(resp.RC):
			refIDSuffix++
			continue
		default:
			refIDSuffix++
			continue
		}
	}
	return s.handleAllSKUsFailed(trx)
}

// logAttempt writes a transaction log entry.
func (s *TransactionService) logAttempt(trxID int, skuID int, digiRefID string, request any, resp *digiflazz.TransactionResponse, err error) {
	reqJSON, _ := json.Marshal(request)
	var respJSON []byte
	var rcPtr, statusPtr *string
	var responseAt *time.Time
	if resp != nil {
		respJSON, _ = json.Marshal(resp)
		if resp.RC != "" {
			rc := resp.RC
			rcPtr = &rc
		}
		if resp.Status != "" {
			st := resp.Status
			statusPtr = &st
		}
		now := time.Now()
		responseAt = &now
	}
	logEntry := &models.TransactionLog{
		TransactionID: trxID,
		SkuID:         skuID,
		DigiRefID:     digiRefID,
		Request:       json.RawMessage(reqJSON),
		Response:      json.RawMessage(respJSON),
		RC:            rcPtr,
		Status:        statusPtr,
		ResponseAt:    responseAt,
	}
	if err := s.callbackRepo.CreateTransactionLog(logEntry); err != nil {
		log.Error().Err(err).Msg("failed to create transaction log")
	}
}

// cachedInquiryToTransaction converts cached inquiry data to transaction model.
func (s *TransactionService) cachedInquiryToTransaction(data *cache.InquiryData, clientID, productID int) *models.Transaction {
	status := models.StatusSuccess
	amount := data.Amount
	customerName := data.CustomerName
	expiredAt := data.ExpiredAt

	return &models.Transaction{
		TransactionID: data.TransactionID,
		ReferenceID:   data.ReferenceID,
		ClientID:      clientID,
		ProductID:     productID,
		SkuCode:       data.SKUCode,
		CustomerNo:    data.CustomerNo,
		Type:          models.TrxTypeInquiry,
		Status:        status,
		Amount:        &amount,
		Admin:         data.Admin,
		CustomerName:  &customerName,
		Description:   models.NullableRawMessage(data.Description),
		ExpiredAt:     &expiredAt,
	}
}

// safeMarshalRaw returns json.RawMessage from a RawMessage or nil if empty.
func safeMarshalRaw(v json.RawMessage) json.RawMessage {
	if len(v) == 0 || bytes.Equal(v, []byte("null")) {
		return nil
	}
	// Ensure it's a copy
	cp := make([]byte, len(v))
	copy(cp, v)
	return json.RawMessage(cp)
}

// RetryWithNextSKU retries a transaction with the next available SKU.
// Called by callback worker when Digiflazz returns a retryable RC code.
// Returns (transaction, shouldMarkFailed, error)
// - shouldMarkFailed=true means all SKUs exhausted, mark as failed
// - shouldMarkFailed=false means either success/pending or error occurred
func (s *TransactionService) RetryWithNextSKU(ctx context.Context, trx *models.Transaction, failedRC string, failedMessage string) (*models.Transaction, bool, error) {
	log.Info().
		Str("transaction_id", trx.TransactionID).
		Str("failed_rc", failedRC).
		Str("failed_message", failedMessage).
		Msg("Retrying transaction with next SKU from callback")

	// Get available SKUs for this product
	currentTime := time.Now().Format("15:04:05")
	skus, err := s.skuRepo.GetAvailableSKUs(trx.ProductID, currentTime)
	if err != nil || len(skus) == 0 {
		log.Error().Err(err).Int("product_id", trx.ProductID).Msg("No available SKUs for retry")
		// Call handleAllSKUsFailed to update transaction and send callback
		result, _ := s.handleAllSKUsFailed(trx)
		return result, true, nil // Mark as failed
	}

	// Get previous attempts from transaction_logs to find which SKUs were already tried
	logs, err := s.callbackRepo.GetLogsByTransactionID(trx.ID)
	if err != nil {
		log.Error().Err(err).Int("transaction_id", trx.ID).Msg("Failed to get transaction logs for retry")
		return trx, false, err
	}

	// Build set of already-tried SKU IDs
	triedSKUs := make(map[int]bool)
	for _, l := range logs {
		if l.SkuID > 0 {
			triedSKUs[l.SkuID] = true
		}
	}

	log.Debug().
		Int("total_skus", len(skus)).
		Int("tried_skus", len(triedSKUs)).
		Interface("tried_sku_ids", triedSKUs).
		Msg("SKU retry status")

	// Find next untried SKUs
	var nextSKUs []models.SKU
	for _, sku := range skus {
		if !triedSKUs[sku.ID] {
			nextSKUs = append(nextSKUs, sku)
		}
	}

	if len(nextSKUs) == 0 {
		log.Info().Str("transaction_id", trx.TransactionID).Msg("All SKUs exhausted, marking as failed")
		// Call handleAllSKUsFailed to update transaction and send callback
		result, _ := s.handleAllSKUsFailed(trx)
		return result, true, nil // All SKUs tried
	}

	log.Info().
		Str("transaction_id", trx.TransactionID).
		Int("remaining_skus", len(nextSKUs)).
		Str("next_sku", nextSKUs[0].DigiSkuCode).
		Msg("Attempting retry with remaining SKUs")

	// Calculate new ref_id suffix based on number of previous attempts
	refIDSuffixStart := len(logs)
	if refIDSuffixStart == 0 {
		refIDSuffixStart = 1 // Start from 1 if no logs (shouldn't happen but safety)
	}

	// Use the same tryAllSKUs logic with remaining SKUs
	result, err := s.tryAllSKUs(ctx, trx, nextSKUs, trx.IsSandbox, refIDSuffixStart)
	if err != nil {
		return result, false, err
	}

	// Check if result is in final state
	if result.Status == models.StatusFailed {
		return result, true, nil // All SKUs exhausted (handleAllSKUsFailed was called)
	}

	return result, false, nil // Success or Pending
}
