package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

// AdminTransactionService provides admin-level transaction management.
type AdminTransactionService struct {
	trxRepo     *repository.TransactionRepository
	productSvc  *ProductService
	trxSvc      *TransactionService
	callbackSvc *CallbackService
}

// NewAdminTransactionService creates a new AdminTransactionService.
func NewAdminTransactionService(
	trxRepo *repository.TransactionRepository,
	productSvc *ProductService,
	trxSvc *TransactionService,
	callbackSvc *CallbackService,
) *AdminTransactionService {
	return &AdminTransactionService{
		trxRepo:     trxRepo,
		productSvc:  productSvc,
		trxSvc:      trxSvc,
		callbackSvc: callbackSvc,
	}
}

// ListTransactionsRequest holds request parameters for listing transactions.
type ListTransactionsRequest struct {
	ClientID      *int    `form:"clientId"`
	Status        *string `form:"status"`
	Type          *string `form:"type"`
	SkuCode       *string `form:"skuCode"`
	CustomerNo    *string `form:"customerNo"`
	ReferenceID   *string `form:"referenceId"`
	TransactionID *string `form:"transactionId"`
	StartDate     *string `form:"startDate"`
	EndDate       *string `form:"endDate"`
	IsSandbox     *bool   `form:"isSandbox"`
	Page          int     `form:"page"`
	Limit         int     `form:"limit"`
}

// ListTransactionsResponse holds response for listing transactions.
type ListTransactionsResponse struct {
	Transactions []TransactionAdminView `json:"transactions"`
	Pagination   PaginationMeta         `json:"pagination"`
}

// PaginationMeta contains pagination information.
type PaginationMeta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
}

// TransactionAdminView represents a transaction for admin view.
type TransactionAdminView struct {
	ID             int                      `json:"id"`
	TransactionID  string                   `json:"transactionId"`
	ReferenceID    string                   `json:"referenceId"`
	ClientID       int                      `json:"clientId"`
	SkuCode        string                   `json:"skuCode"`
	CustomerNo     string                   `json:"customerNo"`
	CustomerName   *string                  `json:"customerName,omitempty"`
	Type           models.TransactionType   `json:"type"`
	Status         models.TransactionStatus `json:"status"`
	SerialNumber   *string                  `json:"serialNumber,omitempty"`
	Price          *int                     `json:"price,omitempty"`
	Admin          int                      `json:"admin,omitempty"`
	Period         *string                  `json:"period,omitempty"`
	DigiSkuUsed    *string                  `json:"digiSkuUsed,omitempty"`
	ProviderRef    *string                  `json:"providerRef,omitempty"`
	RetryCount     int                      `json:"retryCount"`
	FailedReason   *string                  `json:"failedReason,omitempty"`
	FailedCode     *string                  `json:"failedCode,omitempty"`
	CallbackSent   bool                     `json:"callbackSent"`
	CallbackSentAt *string                  `json:"callbackSentAt,omitempty"`
	IsSandbox      bool                     `json:"isSandbox"`
	CreatedAt      string                   `json:"createdAt"`
	ProcessedAt    *string                  `json:"processedAt,omitempty"`
}

// ListTransactions returns paginated list of transactions for admin.
func (s *AdminTransactionService) ListTransactions(req *ListTransactionsRequest) (*ListTransactionsResponse, error) {
	filter := &repository.AdminTransactionFilter{
		ClientID:      req.ClientID,
		Status:        req.Status,
		Type:          req.Type,
		SkuCode:       req.SkuCode,
		CustomerNo:    req.CustomerNo,
		ReferenceID:   req.ReferenceID,
		TransactionID: req.TransactionID,
		StartDate:     req.StartDate,
		EndDate:       req.EndDate,
		IsSandbox:     req.IsSandbox,
		Page:          req.Page,
		Limit:         req.Limit,
	}

	result, err := s.trxRepo.GetAllAdmin(filter)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get transactions for admin")
		return nil, err
	}

	// Convert to admin view
	views := make([]TransactionAdminView, len(result.Transactions))
	for i, trx := range result.Transactions {
		views[i] = s.toAdminView(&trx)
	}

	return &ListTransactionsResponse{
		Transactions: views,
		Pagination: PaginationMeta{
			Page:       result.Page,
			Limit:      result.Limit,
			TotalItems: result.TotalItems,
			TotalPages: result.TotalPages,
		},
	}, nil
}

// GetTransaction returns a transaction by ID or transaction_id for admin.
func (s *AdminTransactionService) GetTransaction(idOrTrxID string) (*TransactionAdminView, error) {
	// Try to get by transaction_id first
	trx, err := s.trxRepo.GetByTransactionIDAdmin(idOrTrxID)
	if err != nil && err != sql.ErrNoRows {
		log.Error().Err(err).Str("id", idOrTrxID).Msg("Failed to get transaction")
		return nil, err
	}
	if trx != nil {
		view := s.toAdminView(trx)
		return &view, nil
	}

	return nil, errors.New("transaction not found")
}

// AdminStatsResponse contains full statistics response.
type AdminStatsResponse struct {
	Stats      *repository.AdminTransactionStats
	DailyTrend []repository.DailyTrend
}

// GetStats returns transaction statistics for admin.
func (s *AdminTransactionService) GetStats(clientID *int, startDate, endDate *string) (*AdminStatsResponse, error) {
	stats, err := s.trxRepo.GetAdminStats(clientID, startDate, endDate)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get transaction stats")
		return nil, err
	}

	dailyTrend, err := s.trxRepo.GetDailyTrend(clientID, startDate, endDate)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get daily trend")
		// Don't fail, just return empty trend
		dailyTrend = []repository.DailyTrend{}
	}

	return &AdminStatsResponse{
		Stats:      stats,
		DailyTrend: dailyTrend,
	}, nil
}

// ManualRetry manually retries a pending transaction.
func (s *AdminTransactionService) ManualRetry(ctx context.Context, transactionID string) (*TransactionAdminView, error) {
	// Get transaction
	trx, err := s.trxRepo.GetByTransactionIDAdmin(transactionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("transaction not found")
		}
		return nil, err
	}

	// Validate status
	if trx.Status != models.StatusPending && trx.Status != models.StatusProcessing {
		return nil, errors.New("transaction cannot be retried - status must be Pending or Processing")
	}

	// For prepaid, retry with available SKUs
	if trx.Type == models.TrxTypePrepaid {
		retried, err := s.trxSvc.RetryTransaction(ctx, trx)
		if err != nil {
			return nil, err
		}
		view := s.toAdminView(retried)
		return &view, nil
	}

	// For payment type, we can't automatically retry as it requires inquiry state
	return nil, errors.New("manual retry not supported for this transaction type")
}

// toAdminView converts a transaction model to admin view.
func (s *AdminTransactionService) toAdminView(trx *models.Transaction) TransactionAdminView {
	view := TransactionAdminView{
		ID:            trx.ID,
		TransactionID: trx.TransactionID,
		ReferenceID:   trx.ReferenceID,
		ClientID:      trx.ClientID,
		SkuCode:       trx.SkuCode,
		CustomerNo:    trx.CustomerNo,
		CustomerName:  trx.CustomerName,
		Type:          trx.Type,
		Status:        trx.Status,
		SerialNumber:  trx.SerialNumber,
		Price:         trx.Amount,
		Admin:         trx.Admin,
		Period:        trx.Period,
		DigiSkuUsed:   trx.DigiSkuCode,
		ProviderRef:   trx.DigiRefID,
		RetryCount:    trx.RetryCount,
		FailedReason:  trx.FailedReason,
		FailedCode:    trx.FailedCode,
		CallbackSent:  trx.CallbackSent,
		IsSandbox:     trx.IsSandbox,
		CreatedAt:     trx.CreatedAt.Format("2006-01-02T15:04:05+07:00"),
	}

	if trx.ProcessedAt != nil {
		t := trx.ProcessedAt.Format("2006-01-02T15:04:05+07:00")
		view.ProcessedAt = &t
	}
	if trx.CallbackAt != nil {
		t := trx.CallbackAt.Format("2006-01-02T15:04:05+07:00")
		view.CallbackSentAt = &t
	}

	return view
}
