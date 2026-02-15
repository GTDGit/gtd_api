package service

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/alterra"
)

// AlterraProviderClient implements PPOBProviderClient for Alterra
type AlterraProviderClient struct {
	prodClient *alterra.Client
	devClient  *alterra.Client
	healthy    bool
	healthMu   sync.RWMutex
}

// NewAlterraProviderClient creates a new Alterra provider client
func NewAlterraProviderClient(prodClient, devClient *alterra.Client) *AlterraProviderClient {
	return &AlterraProviderClient{
		prodClient: prodClient,
		devClient:  devClient,
		healthy:    true,
	}
}

// Code returns the provider code
func (c *AlterraProviderClient) Code() models.ProviderCode {
	return models.ProviderAlterra
}

// getClient returns the appropriate client based on sandbox mode
func (c *AlterraProviderClient) getClient(isSandbox bool) *alterra.Client {
	if isSandbox && c.devClient != nil {
		return c.devClient
	}
	return c.prodClient
}

// Topup processes a prepaid transaction via Purchase
func (c *AlterraProviderClient) Topup(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	// Parse product ID from SKU code
	productID, err := strconv.Atoi(req.SKUCode)
	if err != nil {
		return nil, err
	}

	// Get extra data if provided
	var data json.RawMessage
	if req.Extra != nil {
		data, _ = json.Marshal(req.Extra)
	}

	resp, err := client.Purchase(ctx, req.CustomerNo, productID, req.RefID, data)
	responseTime := time.Since(startTime)

	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertResponse(resp, req.RefID, responseTime), nil
}

// Inquiry checks a postpaid bill
func (c *AlterraProviderClient) Inquiry(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	// Parse product ID from SKU code
	productID, err := strconv.Atoi(req.SKUCode)
	if err != nil {
		return nil, err
	}

	// Get extra data if provided
	var data json.RawMessage
	if req.Extra != nil {
		data, _ = json.Marshal(req.Extra)
	}

	resp, err := client.Inquiry(ctx, req.CustomerNo, productID, req.RefID, data)
	responseTime := time.Since(startTime)

	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertResponse(resp, req.RefID, responseTime), nil
}

// Payment pays a postpaid bill
func (c *AlterraProviderClient) Payment(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	// Parse product ID from SKU code
	productID, err := strconv.Atoi(req.SKUCode)
	if err != nil {
		return nil, err
	}

	// Get extra data if provided
	var data json.RawMessage
	if req.Extra != nil {
		data, _ = json.Marshal(req.Extra)
	}

	resp, err := client.Payment(ctx, req.CustomerNo, productID, req.RefID, data)
	responseTime := time.Since(startTime)

	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertResponse(resp, req.RefID, responseTime), nil
}

// CheckStatus checks transaction status
func (c *AlterraProviderClient) CheckStatus(ctx context.Context, refID string) (*ProviderResponse, error) {
	client := c.getClient(false)
	startTime := time.Now()

	resp, err := client.GetTransactionByOrderID(ctx, refID)
	responseTime := time.Since(startTime)

	if err != nil {
		return nil, err
	}

	// Convert detail response to transaction response format
	trxResp := &alterra.TransactionResponse{
		TransactionID: resp.TransactionID,
		Type:          resp.Type,
		CreatedAt:     resp.CreatedAt,
		UpdatedAt:     resp.UpdatedAt,
		CustomerID:    resp.CustomerID,
		CustomerName:  resp.CustomerName,
		OrderID:       resp.OrderID,
		Price:         resp.Price,
		Status:        resp.Status,
		ResponseCode:  resp.ResponseCode,
		Amount:        resp.Amount,
		Admin:         resp.Admin,
		Product:       resp.Product,
		Data:          resp.Data,
	}

	return c.convertResponse(trxResp, refID, responseTime), nil
}

// GetPriceList fetches current prices
func (c *AlterraProviderClient) GetPriceList(ctx context.Context, category string) ([]ProviderProduct, error) {
	client := c.getClient(false)

	products, err := client.GetAllProducts(ctx)
	if err != nil {
		return nil, err
	}

	var result []ProviderProduct
	for _, p := range products {
		if category != "" && p.ProductType != category {
			continue
		}
		result = append(result, ProviderProduct{
			SKUCode:     strconv.Itoa(p.ProductID),
			ProductName: p.Label,
			Category:    p.ProductType,
			Brand:       p.Operator,
			Price:       p.Price,
			IsActive:    p.Enable,
		})
	}

	return result, nil
}

// IsHealthy returns whether the provider is healthy
func (c *AlterraProviderClient) IsHealthy() bool {
	c.healthMu.RLock()
	defer c.healthMu.RUnlock()
	return c.healthy
}

// markHealthy marks the provider as healthy
func (c *AlterraProviderClient) markHealthy() {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	c.healthy = true
}

// markUnhealthy marks the provider as unhealthy
func (c *AlterraProviderClient) markUnhealthy() {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	c.healthy = false
}

// convertResponse converts Alterra response to unified format
func (c *AlterraProviderClient) convertResponse(resp *alterra.TransactionResponse, refID string, responseTime time.Duration) *ProviderResponse {
	rawResp, _ := json.Marshal(resp)

	// Create description from data
	var description json.RawMessage
	if resp.Data != nil {
		desc := map[string]any{
			"nominal":   resp.Data.Nominal,
			"admin":     resp.Data.Admin,
			"token":     resp.Data.Token,
			"kwh":       resp.Data.KWH,
			"period":    resp.Data.Period,
			"refNumber": resp.Data.RefNumber,
		}
		if resp.Data.Identifier != nil {
			desc["identifier"] = resp.Data.Identifier
		}
		if resp.Data.BillInfo != nil {
			desc["billInfo"] = resp.Data.BillInfo
		}
		description, _ = json.Marshal(desc)
	}

	// Get serial number from data
	serialNumber := ""
	if resp.Data != nil {
		if resp.Data.Token != "" {
			serialNumber = resp.Data.Token
		} else if resp.Data.VendorRefNo != "" {
			serialNumber = resp.Data.VendorRefNo
		} else if resp.Data.RefNumber != "" {
			serialNumber = resp.Data.RefNumber
		}
	}

	// Get error message
	message := ""
	if resp.Error != nil {
		message = resp.Error.Message
	}

	return &ProviderResponse{
		Success:       alterra.IsSuccess(resp.ResponseCode),
		Pending:       alterra.IsPending(resp.ResponseCode),
		RefID:         refID,
		ProviderRefID: strconv.Itoa(resp.TransactionID),
		Status:        resp.Status,
		RC:            resp.ResponseCode,
		Message:       message,
		SerialNumber:  serialNumber,
		CustomerName:  resp.CustomerName,
		Amount:        resp.Amount,
		Admin:         resp.Admin,
		Description:   description,
		RawResponse:   rawResp,
		NeedsRetry:    alterra.NeedsNewRefID(resp.ResponseCode),
		ResponseTime:  responseTime,
	}
}
