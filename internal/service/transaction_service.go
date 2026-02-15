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
	trxRepo        *repository.TransactionRepository
	productRepo    *repository.ProductRepository
	skuRepo        *repository.SKURepository
	callbackRepo   *repository.CallbackRepository
	digiflazzProd  *digiflazz.Client
	digiflazzDev   *digiflazz.Client
	productSvc     *ProductService
	callbackSvc    *CallbackService
	sandboxMapper  *SandboxMapper
	inquiryCache   *cache.InquiryCache
	providerRouter *ProviderRouter // Multi-provider router (optional)
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

// SetProviderRouter sets the multi-provider router for the transaction service
func (s *TransactionService) SetProviderRouter(router *ProviderRouter) {
	s.providerRouter = router
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
	Provider      string `json:"provider"`      // Optional: force specific provider (kiosbank, alterra, digiflazz)
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

	// 3. Generate transaction ID
	trxID, err := s.trxRepo.GenerateTransactionID()
	if err != nil {
		return nil, err
	}

	// 4. Determine sell_price (cheapest provider price = what client sees)
	var sellPrice *int
	if s.providerRouter != nil && !isSandbox {
		if bestPrice, _, err := s.providerRouter.GetBestPrice(product.ID); err == nil && bestPrice != nil {
			sellPrice = bestPrice
		}
	}
	if sellPrice == nil && product.MinPrice != nil && *product.MinPrice > 0 {
		sellPrice = product.MinPrice
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
		SellPrice:     sellPrice,
	}

	if err := s.trxRepo.Create(trx); err != nil {
		// Check for duplicate reference_id (unique constraint violation)
		if isDuplicateKeyError(err) {
			return nil, utils.ErrDuplicateReferenceID
		}
		return nil, err
	}

	// 6. Try multi-provider routing if available
	if s.providerRouter != nil && !isSandbox {
		// Check if we have providers for this product
		providers, err := s.providerRouter.GetProviderOptions(product.ID)
		if err == nil && len(providers) > 0 {
			return s.executeWithProviderRouter(ctx, trx, ProviderTrxPrepaid, req.Provider)
		}
		// No providers configured, fallback to legacy Digiflazz flow
		log.Debug().Int("product_id", product.ID).Msg("No multi-provider SKUs, using legacy Digiflazz flow")
	}

	// 7. Legacy flow: Get available SKUs and try each
	skus, err := s.productSvc.GetAvailableSKUs(product.ID)
	if err != nil || len(skus) == 0 {
		return s.handleAllSKUsFailed(trx)
	}

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
		if err := s.trxRepo.Update(trx); err != nil {
		log.Error().Err(err).Str("transaction_id", trx.TransactionID).Str("status", string(trx.Status)).Msg("CRITICAL: failed to update transaction in DB")
	}

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
	trx.BuyPrice = &resp.Price
	trx.ProcessedAt = &now
	if resp.RefID != "" {
		trx.DigiRefID = &resp.RefID
	}
	if err := s.trxRepo.Update(trx); err != nil {
		log.Error().Err(err).Str("transaction_id", trx.TransactionID).Str("status", string(trx.Status)).Msg("CRITICAL: failed to update transaction in DB")
	}

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
	if err := s.trxRepo.Update(trx); err != nil {
		log.Error().Err(err).Str("transaction_id", trx.TransactionID).Str("status", string(trx.Status)).Msg("CRITICAL: failed to update transaction in DB")
	}
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
	if err := s.trxRepo.Update(trx); err != nil {
		log.Error().Err(err).Str("transaction_id", trx.TransactionID).Str("status", string(trx.Status)).Msg("CRITICAL: failed to update transaction in DB")
	}

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
	if err := s.trxRepo.Update(trx); err != nil {
		log.Error().Err(err).Str("transaction_id", trx.TransactionID).Str("status", string(trx.Status)).Msg("CRITICAL: failed to update transaction in DB")
	}

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

	// Expiration end of day WIB
	wib := time.FixedZone("WIB", 7*3600) // UTC+7
	nowWIB := time.Now().In(wib)
	eod := time.Date(nowWIB.Year(), nowWIB.Month(), nowWIB.Day(), 23, 59, 59, 0, wib)

	// Try multi-provider inquiry if available and not sandbox
	if s.providerRouter != nil && !isSandbox {
		providers, provErr := s.providerRouter.GetProviderOptionsPostpaid(product.ID)
		if provErr == nil && len(providers) > 0 {
			return s.executeInquiryWithProviders(ctx, req, client, product, trxID, providers, eod)
		}
		log.Debug().Int("product_id", product.ID).Msg("No multi-provider SKUs for inquiry, using legacy Digiflazz flow")
	}

	// Legacy Digiflazz inquiry flow
	return s.executeInquiryWithDigiflazz(ctx, req, client, product, trxID, eod, isSandbox)
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
	// sell_price for payment = the inquiry amount (what client was quoted)
	var sellPrice *int
	if inquiryData.Amount > 0 {
		sp := inquiryData.Amount
		sellPrice = &sp
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
		SellPrice:     sellPrice,
	}
	if err := s.trxRepo.Create(payment); err != nil {
		return nil, err
	}

	// 4. Route payment to the correct provider
	// If inquiry was handled by a multi-provider (ProviderCode is set), use that same provider.
	// Otherwise, fall back to legacy Digiflazz flow.
	if inquiryData.ProviderCode != "" && s.providerRouter != nil && !isSandbox {
		log.Info().
			Str("provider", inquiryData.ProviderCode).
			Str("inquiry_trx_id", inquiryData.TransactionID).
			Str("payment_trx_id", payTrxID).
			Msg("Routing payment to same provider as inquiry")
		return s.executePaymentWithProvider(ctx, payment, inquiryData)
	}

	// Legacy Digiflazz payment flow
	digiSKU := req.SkuCode
	digiCustomerNo := inquiryData.CustomerNo

	if isSandbox {
		testSKU, testCustomerNo := s.sandboxMapper.GetTestMapping(req.SkuCode, payment.Type)
		digiSKU = testSKU
		digiCustomerNo = testCustomerNo
	}

	refID := inquiryData.TransactionID
	digi := s.getDigiflazzClient(isSandbox)
	resp, err := digi.Payment(ctx, digiSKU, digiCustomerNo, refID, isSandbox)

	s.logAttempt(payment.ID, 0, refID, map[string]any{
		"buyer_sku_code": digiSKU,
		"customer_no":    digiCustomerNo,
		"ref_id":         refID,
		"testing":        isSandbox,
	}, resp, err)

	if err != nil {
		return s.handleAllSKUsFailed(payment)
	}

	if digiflazz.IsSuccess(resp.RC) {
		now := time.Now()
		payment.Status = models.StatusSuccess
		if resp.SN != "" {
			payment.SerialNumber = &resp.SN
		}
		payment.Amount = &resp.Price
		payment.BuyPrice = &resp.Price
		payment.ProcessedAt = &now
		payment.DigiRefID = &refID
		if err := s.trxRepo.Update(payment); err != nil {
		log.Error().Err(err).Str("transaction_id", payment.TransactionID).Str("status", string(payment.Status)).Msg("CRITICAL: failed to update payment in DB")
	}

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
		if err := s.trxRepo.Update(payment); err != nil {
		log.Error().Err(err).Str("transaction_id", payment.TransactionID).Str("status", string(payment.Status)).Msg("CRITICAL: failed to update payment in DB")
	}
		return payment, nil
	}

	// Fatal/other
	now := time.Now()
	payment.Status = models.StatusFailed
	msg := resp.Message
	payment.FailedReason = &msg
	payment.ProcessedAt = &now
	payment.DigiRefID = &refID
	if err := s.trxRepo.Update(payment); err != nil {
		log.Error().Err(err).Str("transaction_id", payment.TransactionID).Str("status", string(payment.Status)).Msg("CRITICAL: failed to update payment in DB")
	}
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
// CRITICAL: Must check if there's a pending transaction at any provider first to avoid duplicates.
func (s *TransactionService) RetryTransaction(ctx context.Context, trx *models.Transaction) (*models.Transaction, error) {
	// If transaction is Processing with an active provider ref, don't retry - wait for callback.
	if trx.Status == models.StatusProcessing {
		if (trx.DigiRefID != nil && *trx.DigiRefID != "") ||
			(trx.ProviderRefID != nil && *trx.ProviderRefID != "") {
			providerName := "provider"
			if trx.ProviderCode != nil && *trx.ProviderCode != "" {
				providerName = *trx.ProviderCode
			} else if trx.DigiRefID != nil && *trx.DigiRefID != "" {
				providerName = "digiflazz"
			}
			log.Info().
				Str("transaction_id", trx.TransactionID).
				Str("provider", providerName).
				Msg("Transaction is processing at provider, cannot retry - wait for callback")
			return trx, fmt.Errorf("transaction is processing at %s, please wait for callback", providerName)
		}
	}

	// Use provider router for multi-provider transactions (non-sandbox, prepaid)
	if s.providerRouter != nil && !trx.IsSandbox && trx.Type == "prepaid" {
		return s.executeWithProviderRouter(ctx, trx, ProviderTrxPrepaid, "")
	}

	// Legacy Digiflazz-only path (sandbox or no provider router)
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
		if err := s.trxRepo.Update(trx); err != nil {
		log.Error().Err(err).Str("transaction_id", trx.TransactionID).Str("status", string(trx.Status)).Msg("CRITICAL: failed to update transaction in DB")
	}

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

// logProviderAttempt logs a provider transaction attempt to the transaction_logs table.
func (s *TransactionService) logProviderAttempt(trxID int, refID string, request any, resp *ProviderResponse, err error) {
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
		DigiRefID:     refID,
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

	// Get available SKUs for this product (use WIB timezone for availability window)
	wib := time.FixedZone("WIB", 7*3600)
	currentTime := time.Now().In(wib).Format("15:04:05")
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

// executeWithProviderRouter executes a transaction using the multi-provider router
func (s *TransactionService) executeWithProviderRouter(ctx context.Context, trx *models.Transaction, trxType ProviderTransactionType, forceProvider string) (*models.Transaction, error) {
	if s.providerRouter == nil {
		return nil, fmt.Errorf("provider router not configured")
	}

	// Build provider request
	req := &ProviderRequest{
		RefID:         trx.TransactionID,
		CustomerNo:    trx.CustomerNo,
		Type:          trxType,
		IsSandbox:     trx.IsSandbox,
		ForceProvider: models.ProviderCode(forceProvider),
	}

	// Execute with provider router
	result, err := s.providerRouter.Execute(ctx, trx.ProductID, req)
	if err != nil {
		log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Provider router execution failed")
		return s.handleAllSKUsFailed(trx)
	}

	// Store provider info
	if result.ProviderUsed != nil {
		providerID := result.ProviderUsed.ProviderID
		trx.ProviderID = &providerID
		providerSKUID := result.ProviderUsed.ProviderSKUID
		trx.ProviderSKUID = &providerSKUID
		providerCode := string(result.ProviderUsed.ProviderCode)
		trx.ProviderCode = &providerCode
	}

	if result.Response == nil {
		log.Error().Str("transaction_id", trx.TransactionID).Msg("Provider returned nil response")
		return s.handleAllSKUsFailed(trx)
	}

	// Store provider reference ID
	if result.Response.ProviderRefID != "" {
		trx.ProviderRefID = &result.Response.ProviderRefID
	}

	// Store raw response
	if len(result.Response.RawResponse) > 0 {
		trx.ProviderResponse = models.NullableRawMessage(result.Response.RawResponse)
	}

	// Handle response based on status
	if result.Response.Success {
		return s.handleProviderSuccess(trx, result.Response)
	}

	if result.Response.Pending {
		return s.handleProviderPending(trx, result.Response)
	}

	// Failed
	return s.handleProviderFailed(trx, result.Response)
}

// handleProviderSuccess handles a successful provider response
func (s *TransactionService) handleProviderSuccess(trx *models.Transaction, resp *ProviderResponse) (*models.Transaction, error) {
	now := time.Now()
	trx.Status = models.StatusSuccess
	if resp.SerialNumber != "" {
		trx.SerialNumber = &resp.SerialNumber
	}
	if resp.Amount > 0 {
		trx.Amount = &resp.Amount
		trx.BuyPrice = &resp.Amount
	}
	if resp.CustomerName != "" {
		trx.CustomerName = &resp.CustomerName
	}
	if len(resp.Description) > 0 {
		trx.Description = models.NullableRawMessage(resp.Description)
	}
	trx.ProcessedAt = &now
	if err := s.trxRepo.Update(trx); err != nil {
		log.Error().Err(err).Str("transaction_id", trx.TransactionID).Str("status", string(trx.Status)).Msg("CRITICAL: failed to update transaction in DB")
	}

	go s.callbackSvc.SendCallback(trx, "transaction.success")
	return trx, nil
}

// handleProviderPending handles a pending provider response
func (s *TransactionService) handleProviderPending(trx *models.Transaction, resp *ProviderResponse) (*models.Transaction, error) {
	trx.Status = models.StatusProcessing
	if resp.Amount > 0 {
		trx.Amount = &resp.Amount
	}
	if resp.CustomerName != "" {
		trx.CustomerName = &resp.CustomerName
	}
	if len(resp.Description) > 0 {
		trx.Description = models.NullableRawMessage(resp.Description)
	}
	if err := s.trxRepo.Update(trx); err != nil {
		log.Error().Err(err).Str("transaction_id", trx.TransactionID).Str("status", string(trx.Status)).Msg("CRITICAL: failed to update transaction in DB")
	}
	return trx, nil
}

// handleProviderFailed handles a failed provider response
func (s *TransactionService) handleProviderFailed(trx *models.Transaction, resp *ProviderResponse) (*models.Transaction, error) {
	now := time.Now()
	trx.Status = models.StatusFailed
	if resp.Message != "" {
		trx.FailedReason = &resp.Message
	}
	if resp.RC != "" {
		trx.FailedCode = &resp.RC
	}
	trx.ProcessedAt = &now
	if err := s.trxRepo.Update(trx); err != nil {
		log.Error().Err(err).Str("transaction_id", trx.TransactionID).Str("status", string(trx.Status)).Msg("CRITICAL: failed to update transaction in DB")
	}

	go s.callbackSvc.SendCallback(trx, "transaction.failed")
	return trx, nil
}

// executeInquiryWithProviders tries inquiry across multiple providers in price order.
// On success, caches the inquiry data WITH provider info so payment routes to the same provider.
// If req.Provider is set, only that provider is used (user preference).
func (s *TransactionService) executeInquiryWithProviders(
	ctx context.Context,
	req *CreateTransactionRequest,
	client *models.Client,
	product *models.Product,
	trxID string,
	providers []models.ProviderOption,
	eod time.Time,
) (*models.Transaction, error) {
	// Filter providers if user specifies a preferred provider
	if req.Provider != "" {
		filtered := make([]models.ProviderOption, 0)
		for _, opt := range providers {
			if string(opt.ProviderCode) == req.Provider {
				filtered = append(filtered, opt)
				break
			}
		}
		if len(filtered) == 0 {
			log.Warn().Str("provider", req.Provider).Int("product_id", product.ID).Msg("Requested provider not available for this product")
			// Fall through to all providers
		} else {
			providers = filtered
		}
	}

	for _, opt := range providers {
		adapter := s.providerRouter.GetAdapter(string(opt.ProviderCode))
		if adapter == nil {
			log.Warn().Str("provider", string(opt.ProviderCode)).Msg("Provider adapter not found for inquiry")
			continue
		}
		if !adapter.IsHealthy() {
			log.Warn().Str("provider", string(opt.ProviderCode)).Msg("Provider not healthy, skipping inquiry")
			continue
		}

		provReq := &ProviderRequest{
			RefID:      trxID,
			SKUCode:    opt.ProviderSKUCode,
			CustomerNo: req.CustomerNo,
			Type:       ProviderTrxInquiry,
			IsSandbox:  false,
		}

		log.Info().
			Str("provider", string(opt.ProviderCode)).
			Str("sku_code", opt.ProviderSKUCode).
			Str("ref_id", trxID).
			Msg("Trying inquiry with provider")

		resp, err := adapter.Inquiry(ctx, provReq)
		if err != nil {
			log.Warn().Err(err).Str("provider", string(opt.ProviderCode)).Msg("Inquiry network error, trying next provider")
			continue
		}

		if resp.Success {
			// Cache inquiry with provider info
			inquiryData := &cache.InquiryData{
				TransactionID:   trxID,
				ReferenceID:     req.ReferenceID,
				ClientID:        client.ID,
				ProductID:       product.ID,
				CustomerNo:      req.CustomerNo,
				SKUCode:         req.SkuCode,
				Amount:          resp.Amount,
				Admin:           resp.Admin,
				CustomerName:    resp.CustomerName,
				Description:     resp.Description,
				ExpiredAt:       eod,
				ProviderCode:    string(opt.ProviderCode),
				ProviderSKUCode: opt.ProviderSKUCode,
				ProviderID:      opt.ProviderID,
				ProviderSKUID:   opt.ProviderSKUID,
			}

			if err := s.inquiryCache.Set(ctx, inquiryData); err != nil {
				log.Error().Err(err).Msg("failed to cache inquiry")
			}

			log.Info().
				Str("provider", string(opt.ProviderCode)).
				Str("transaction_id", trxID).
				Int("amount", resp.Amount).
				Msg("Inquiry successful with provider")

			return s.cachedInquiryToTransaction(inquiryData, client.ID, product.ID), nil
		}

		// Not successful - log and try next provider
		log.Warn().
			Str("provider", string(opt.ProviderCode)).
			Str("rc", resp.RC).
			Str("message", resp.Message).
			Msg("Inquiry failed with provider, trying next")
	}

	// All providers failed, fall back to legacy Digiflazz
	log.Info().Msg("All multi-providers failed for inquiry, falling back to Digiflazz")
	return s.executeInquiryWithDigiflazz(ctx, req, client, product, trxID, eod, false)
}

// executeInquiryWithDigiflazz runs the legacy Digiflazz inquiry flow.
func (s *TransactionService) executeInquiryWithDigiflazz(
	ctx context.Context,
	req *CreateTransactionRequest,
	client *models.Client,
	product *models.Product,
	trxID string,
	eod time.Time,
	isSandbox bool,
) (*models.Transaction, error) {
	digiSKU := req.SkuCode
	digiCustomerNo := req.CustomerNo

	if isSandbox {
		testSKU, testCustomerNo := s.sandboxMapper.GetTestMapping(req.SkuCode, models.TrxTypeInquiry)
		digiSKU = testSKU
		digiCustomerNo = testCustomerNo
	}

	digi := s.getDigiflazzClient(isSandbox)
	resp, err := digi.Inquiry(ctx, digiSKU, digiCustomerNo, trxID, isSandbox)

	log.Info().
		Str("transactionId", trxID).
		Str("buyer_sku_code", digiSKU).
		Str("customer_no", digiCustomerNo).
		Bool("sandbox", isSandbox).
		Msg("inquiry request to digiflazz (fallback)")

	if err != nil {
		log.Error().Err(err).Str("transactionId", trxID).Msg("inquiry failed")
		return nil, fmt.Errorf("inquiry failed: %w", err)
	}

	if !digiflazz.IsSuccess(resp.RC) {
		log.Warn().Str("rc", resp.RC).Str("message", resp.Message).Msg("inquiry not successful")
		return nil, fmt.Errorf("inquiry failed: %s", resp.Message)
	}

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
		// ProviderCode left empty = legacy Digiflazz
	}

	if err := s.inquiryCache.Set(ctx, inquiryData); err != nil {
		log.Error().Err(err).Msg("failed to cache inquiry")
	}

	return s.cachedInquiryToTransaction(inquiryData, client.ID, product.ID), nil
}

// executePaymentWithProvider executes payment using a specific provider (from inquiry cache).
func (s *TransactionService) executePaymentWithProvider(
	ctx context.Context,
	payment *models.Transaction,
	inquiryData *cache.InquiryData,
) (*models.Transaction, error) {
	adapter := s.providerRouter.GetAdapter(inquiryData.ProviderCode)
	if adapter == nil {
		log.Warn().Str("provider", inquiryData.ProviderCode).Msg("Provider adapter not found for payment, falling back to Digiflazz")
		return nil, fmt.Errorf("provider adapter not found: %s", inquiryData.ProviderCode)
	}

	provReq := &ProviderRequest{
		RefID:      inquiryData.TransactionID, // Use inquiry transaction ID as ref for payment
		SKUCode:    inquiryData.ProviderSKUCode,
		CustomerNo: inquiryData.CustomerNo,
		Amount:     inquiryData.Amount,
		Type:       ProviderTrxPayment,
		IsSandbox:  payment.IsSandbox,
		Extra: map[string]any{
			"admin": inquiryData.Admin,
		},
	}

	// Store provider info on the payment transaction
	providerID := inquiryData.ProviderID
	payment.ProviderID = &providerID
	providerSKUID := inquiryData.ProviderSKUID
	payment.ProviderSKUID = &providerSKUID
	providerCode := inquiryData.ProviderCode
	payment.ProviderCode = &providerCode

	log.Info().
		Str("provider", inquiryData.ProviderCode).
		Str("sku_code", inquiryData.ProviderSKUCode).
		Str("ref_id", provReq.RefID).
		Str("payment_trx_id", payment.TransactionID).
		Msg("Executing payment with provider")

	resp, err := adapter.Payment(ctx, provReq)

	// Log attempt
	s.logProviderAttempt(payment.ID, provReq.RefID, map[string]any{
		"provider":    inquiryData.ProviderCode,
		"sku_code":    inquiryData.ProviderSKUCode,
		"customer_no": inquiryData.CustomerNo,
		"ref_id":      provReq.RefID,
		"amount":      inquiryData.Amount,
		"admin":       inquiryData.Admin,
	}, resp, err)

	if err != nil {
		log.Error().Err(err).Str("provider", inquiryData.ProviderCode).Msg("Payment network error")
		return s.handleAllSKUsFailed(payment)
	}

	// Store provider reference ID
	if resp.ProviderRefID != "" {
		payment.ProviderRefID = &resp.ProviderRefID
	}
	if len(resp.RawResponse) > 0 {
		payment.ProviderResponse = models.NullableRawMessage(resp.RawResponse)
	}

	if resp.Success {
		now := time.Now()
		payment.Status = models.StatusSuccess
		if resp.SerialNumber != "" {
			payment.SerialNumber = &resp.SerialNumber
		}
		if resp.Amount > 0 {
			payment.Amount = &resp.Amount
			payment.BuyPrice = &resp.Amount
		}
		payment.ProcessedAt = &now
		if err := s.trxRepo.Update(payment); err != nil {
		log.Error().Err(err).Str("transaction_id", payment.TransactionID).Str("status", string(payment.Status)).Msg("CRITICAL: failed to update payment in DB")
	}

		// Delete inquiry from cache (already paid)
		if err := s.inquiryCache.Delete(ctx, inquiryData); err != nil {
			log.Warn().Err(err).Msg("failed to delete inquiry cache after payment")
		}

		go s.callbackSvc.SendCallback(payment, "transaction.success")
		return payment, nil
	}

	if resp.Pending {
		payment.Status = models.StatusProcessing
		if resp.Amount > 0 {
			payment.Amount = &resp.Amount
		}
		if err := s.trxRepo.Update(payment); err != nil {
		log.Error().Err(err).Str("transaction_id", payment.TransactionID).Str("status", string(payment.Status)).Msg("CRITICAL: failed to update payment in DB")
	}
		return payment, nil
	}

	// Fatal/failed
	return s.handleProviderFailed(payment, resp)
}
