package handler

import (
    "strings"

    "github.com/gin-gonic/gin"

    "github.com/GTDGit/gtd_api/internal/middleware"
    "github.com/GTDGit/gtd_api/internal/models"
    "github.com/GTDGit/gtd_api/internal/service"
    "github.com/GTDGit/gtd_api/internal/utils"
)

// TransactionHandler handles transaction HTTP endpoints.
type TransactionHandler struct {
    trxService     *service.TransactionService
    productService *service.ProductService
}

// NewTransactionHandler constructs a TransactionHandler.
func NewTransactionHandler(trxService *service.TransactionService, productService *service.ProductService) *TransactionHandler {
    return &TransactionHandler{
        trxService:     trxService,
        productService: productService,
    }
}

// CreateTransaction handles POST /v1/transaction
func (h *TransactionHandler) CreateTransaction(c *gin.Context) {
    var req service.CreateTransactionRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        utils.Error(c, 400, "MISSING_FIELD", "Invalid request body")
        return
    }

    // Validate type
    if req.Type != "prepaid" && req.Type != "inquiry" && req.Type != "payment" {
        utils.Error(c, 400, "INVALID_TYPE", "Type must be 'prepaid', 'inquiry', or 'payment'")
        return
    }

    // Payment requires transactionId
    if req.Type == "payment" && req.TransactionID == "" {
        utils.Error(c, 400, "MISSING_FIELD", "transactionId is required for payment")
        return
    }

    client := middleware.GetClient(c)
    if client == nil {
        utils.Error(c, 401, "INVALID_TOKEN", "Unauthorized")
        return
    }
    isSandbox := middleware.IsSandbox(c)

    trx, err := h.trxService.CreateTransaction(c.Request.Context(), &req, client, isSandbox)
    if err != nil {
        h.handleError(c, err)
        return
    }

    message := "Transaction created"
    if req.Type == "inquiry" {
        message = "Inquiry success"
    } else if req.Type == "payment" {
        message = "Payment " + strings.ToLower(string(trx.Status))
    }

    utils.Success(c, 201, message, h.formatTransaction(trx))
}

// GetTransaction handles GET /v1/transaction/:transactionId
func (h *TransactionHandler) GetTransaction(c *gin.Context) {
    transactionID := c.Param("transactionId")
    clientID := c.GetInt("client_id")

    trx, err := h.trxService.GetTransaction(transactionID, clientID)
    if err != nil {
        utils.Error(c, 404, "TRANSACTION_NOT_FOUND", "Transaction not found")
        return
    }

    utils.Success(c, 200, "Transaction retrieved", h.formatTransaction(trx))
}

func (h *TransactionHandler) handleError(c *gin.Context, err error) {
    switch err {
    case utils.ErrDuplicateReferenceID:
        utils.Error(c, 400, "DUPLICATE_REFERENCE_ID", "Reference ID already exists")
    case utils.ErrInvalidSKU:
        utils.Error(c, 400, "INVALID_SKU", "SKU code not found")
    case utils.ErrNoAvailableSKU:
        utils.Error(c, 400, "NO_AVAILABLE_SKU", "No available SKU for this product")
    case utils.ErrTransactionNotFound:
        utils.Error(c, 404, "TRANSACTION_NOT_FOUND", "Transaction not found")
    case utils.ErrInvalidTransactionType:
        utils.Error(c, 400, "INVALID_TRANSACTION_TYPE", "Transaction is not an inquiry")
    case utils.ErrReferenceMismatch:
        utils.Error(c, 400, "REFERENCE_MISMATCH", "Reference ID does not match")
    case utils.ErrSkuMismatch:
        utils.Error(c, 400, "SKU_MISMATCH", "SKU code does not match")
    case utils.ErrCustomerMismatch:
        utils.Error(c, 400, "CUSTOMER_MISMATCH", "Customer number does not match")
    case utils.ErrInquiryExpired:
        utils.Error(c, 400, "INQUIRY_EXPIRED", "Inquiry has expired")
    case utils.ErrInquiryAlreadyPaid:
        utils.Error(c, 400, "INQUIRY_ALREADY_PAID", "Inquiry has already been paid")
    default:
        utils.Error(c, 500, "INTERNAL_ERROR", "Internal server error")
    }
}

func (h *TransactionHandler) formatTransaction(trx *models.Transaction) interface{} {
    // Populate skuCode from product
    if trx.SkuCode == "" && trx.ProductID > 0 {
        if product, err := h.productService.GetProductByID(trx.ProductID); err == nil && product != nil {
            trx.SkuCode = product.SkuCode
        }
    }
    return trx
}
