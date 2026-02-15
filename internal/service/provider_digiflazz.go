package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/digiflazz"
)

// DigiflazzProviderClient wraps the existing Digiflazz client to implement PPOBProviderClient
type DigiflazzProviderClient struct {
	prodClient *digiflazz.Client
	devClient  *digiflazz.Client
	healthy    bool
	healthMu   sync.RWMutex
}

// NewDigiflazzProviderClient creates a new Digiflazz provider client
func NewDigiflazzProviderClient(prodClient, devClient *digiflazz.Client) *DigiflazzProviderClient {
	return &DigiflazzProviderClient{
		prodClient: prodClient,
		devClient:  devClient,
		healthy:    true,
	}
}

// Code returns the provider code
func (c *DigiflazzProviderClient) Code() models.ProviderCode {
	return models.ProviderDigiflazz
}

// getClient returns the appropriate client based on sandbox mode
func (c *DigiflazzProviderClient) getClient(isSandbox bool) *digiflazz.Client {
	if isSandbox {
		return c.devClient
	}
	return c.prodClient
}

// Topup processes a prepaid transaction
func (c *DigiflazzProviderClient) Topup(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	resp, err := client.Topup(ctx, req.SKUCode, req.CustomerNo, req.RefID, req.IsSandbox)
	responseTime := time.Since(startTime)

	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertResponse(resp, responseTime), nil
}

// Inquiry checks a postpaid bill
func (c *DigiflazzProviderClient) Inquiry(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	resp, err := client.Inquiry(ctx, req.SKUCode, req.CustomerNo, req.RefID, req.IsSandbox)
	responseTime := time.Since(startTime)

	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertResponse(resp, responseTime), nil
}

// Payment pays a postpaid bill
func (c *DigiflazzProviderClient) Payment(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	resp, err := client.Payment(ctx, req.SKUCode, req.CustomerNo, req.RefID, req.IsSandbox)
	responseTime := time.Since(startTime)

	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertResponse(resp, responseTime), nil
}

// CheckStatus checks transaction status
func (c *DigiflazzProviderClient) CheckStatus(ctx context.Context, refID string) (*ProviderResponse, error) {
	// Digiflazz doesn't have a direct check status API
	// Status is determined from callback or inquiry response
	return nil, fmt.Errorf("CheckStatus not implemented for %s: use callback/inquiry", c.Code())
}

// GetPriceList fetches current prices
func (c *DigiflazzProviderClient) GetPriceList(ctx context.Context, category string) ([]ProviderProduct, error) {
	client := c.getClient(false)

	var products []ProviderProduct

	// Fetch prepaid
	prepaidResp, err := client.GetPricelist(ctx, "prepaid")
	if err != nil {
		return nil, err
	}
	for _, p := range prepaidResp.Data {
		if category != "" && p.Category != category {
			continue
		}
		products = append(products, ProviderProduct{
			SKUCode:     p.BuyerSkuCode,
			ProductName: p.ProductName,
			Category:    p.Category,
			Brand:       p.Brand,
			Price:       p.Price,
			IsActive:    p.SellerProductStatus && p.BuyerProductStatus,
		})
	}

	// Fetch postpaid
	postpaidResp, err := client.GetPricelist(ctx, "pasca")
	if err != nil {
		return nil, err
	}
	for _, p := range postpaidResp.Data {
		if category != "" && p.Category != category {
			continue
		}
		products = append(products, ProviderProduct{
			SKUCode:     p.BuyerSkuCode,
			ProductName: p.ProductName,
			Category:    p.Category,
			Brand:       p.Brand,
			Price:       p.Price,
			Admin:       p.Admin,
			IsActive:    p.SellerProductStatus && p.BuyerProductStatus,
		})
	}

	return products, nil
}

// IsHealthy returns whether the provider is healthy
func (c *DigiflazzProviderClient) IsHealthy() bool {
	c.healthMu.RLock()
	defer c.healthMu.RUnlock()
	return c.healthy
}

// markHealthy marks the provider as healthy
func (c *DigiflazzProviderClient) markHealthy() {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	c.healthy = true
}

// markUnhealthy marks the provider as unhealthy
func (c *DigiflazzProviderClient) markUnhealthy() {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	c.healthy = false
}

// convertResponse converts Digiflazz response to unified ProviderResponse
func (c *DigiflazzProviderClient) convertResponse(resp *digiflazz.TransactionResponse, responseTime time.Duration) *ProviderResponse {
	rawResp, _ := json.Marshal(resp)

	description, _ := json.Marshal(resp.Desc)

	return &ProviderResponse{
		Success:       digiflazz.IsSuccess(resp.RC),
		Pending:       digiflazz.IsPending(resp.RC),
		RefID:         resp.RefID,
		ProviderRefID: resp.RefID,
		Status:        resp.Status,
		RC:            resp.RC,
		Message:       resp.Message,
		SerialNumber:  resp.SN,
		CustomerName:  resp.CustomerName,
		Amount:        resp.Price,
		Admin:         resp.Admin,
		Description:   description,
		RawResponse:   rawResp,
		NeedsRetry:    digiflazz.NeedsNewRefID(resp.RC),
		ResponseTime:  responseTime,
	}
}
