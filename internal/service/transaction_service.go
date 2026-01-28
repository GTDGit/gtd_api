package service

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
    "github.com/rs/zerolog/log"

    "github.com/GTDGit/gtd_api/internal/cache"
    "github.com/GTDGit/gtd_api/internal/models"
    "github.com/GTDGit/gtd_api/internal/repository"
    "github.com/GTDGit/gtd_api/internal/utils"
    "github.com/GTDGit/gtd_api/pkg/digiflazz"
)

// TransactionService contains business logic for transactions.
type TransactionService struct {
    trxRepo         *repository.TransactionRepository
    productRepo     *repository.ProductRepository
    skuRepo         *repository.SKURepository
    callbackRepo    *repository.CallbackRepository
    digiflazzProd   *digiflazz.Client
    digiflazzDev    *digiflazz.Client
    productSvc      *ProductService
    callbackSvc     *CallbackService
    sandboxMapper   *SandboxMapper
    inquiryCache    *cache.InquiryCache
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
        trxRepo:         trxRepo,
        productRepo:     productRepo,
        skuRepo:         skuRepo,
        callbackRepo:    callbackRepo,
        digiflazzProd:   digiProd,
        digiflazzDev:    digiDev,
        productSvc:      productSvc,
        callbackSvc:     callbackSvc,
        sandboxMapper:   NewSandboxMapper(),
        inquiryCache:    inquiryCache,
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
        return nil, err
    }

    // 6. Try each SKU
    return s.tryAllSKUs(ctx, trx, skus, isSandbox)
}

// tryAllSKUs attempts transaction with each SKU until success/pending/fatal.
func (s *TransactionService) tryAllSKUs(ctx context.Context, trx *models.Transaction, skus []models.SKU, isSandbox bool) (*models.Transaction, error) {
    refIDSuffix := 0
    for _, sku := range skus {
        // Generate Digiflazz ref_id
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
            refIDSuffix++
            continue // Try next SKU with new ref id suffix
        }

        // Check RC
        switch {
        case digiflazz.IsSuccess(resp.RC):
            return s.handleSuccess(trx, &sku, resp)
        case digiflazz.IsPending(resp.RC):
            return s.handlePending(trx, &sku, resp)
        case digiflazz.IsFatal(resp.RC):
            return s.handleFatal(trx, resp)
        case digiflazz.NeedsNewRefID(resp.RC):
            refIDSuffix++
            continue
        case digiflazz.IsRetryable(resp.RC):
            refIDSuffix++
            continue
        default:
            // Unknown RC, treat as retryable switch
            refIDSuffix++
            continue
        }
    }
    // All SKUs failed - set Pending for retry worker
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
    trx.ProcessedAt = &now
    _ = s.trxRepo.Update(trx)

    go s.callbackSvc.SendCallback(trx, "transaction.failed")
    return trx, nil
}

// handleAllSKUsFailed sets trx as pending for retry.
func (s *TransactionService) handleAllSKUsFailed(trx *models.Transaction) (*models.Transaction, error) {
    nextRetry := time.Now().Add(15 * time.Minute)
    trx.Status = models.StatusPending
    trx.RetryCount++
    trx.NextRetryAt = &nextRetry
    _ = s.trxRepo.Update(trx)

    go s.callbackSvc.SendCallback(trx, "transaction.pending")
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

// RetryTransaction retries a pending transaction using available SKUs.
func (s *TransactionService) RetryTransaction(ctx context.Context, trx *models.Transaction) (*models.Transaction, error) {
    skus, err := s.productSvc.GetAvailableSKUs(trx.ProductID)
    if err != nil || len(skus) == 0 {
        return s.handleAllSKUsFailed(trx)
    }
    return s.tryAllSKUs(ctx, trx, skus, trx.IsSandbox)
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
        Description:   data.Description,
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
