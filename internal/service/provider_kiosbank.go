package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/pkg/kiosbank"
)

// KiosbankProviderClient implements PPOBProviderClient for Kiosbank
type KiosbankProviderClient struct {
	prodClient   *kiosbank.Client
	devClient    *kiosbank.Client
	trxRepo      *repository.TransactionRepository
	providerRepo *repository.PPOBProviderRepository
	healthy      bool
	healthMu     sync.RWMutex
}

// NewKiosbankProviderClient creates a new Kiosbank provider client
func NewKiosbankProviderClient(prodClient, devClient *kiosbank.Client, trxRepo *repository.TransactionRepository, providerRepo *repository.PPOBProviderRepository) *KiosbankProviderClient {
	return &KiosbankProviderClient{
		prodClient:   prodClient,
		devClient:    devClient,
		trxRepo:      trxRepo,
		providerRepo: providerRepo,
		healthy:      true,
	}
}

// Code returns the provider code
func (c *KiosbankProviderClient) Code() models.ProviderCode {
	return models.ProviderKiosbank
}

// getClient returns the appropriate client based on sandbox mode
func (c *KiosbankProviderClient) getClient(isSandbox bool) *kiosbank.Client {
	if isSandbox {
		return c.devClient
	}
	return c.prodClient
}

// Topup processes a prepaid transaction via SinglePayment
func (c *KiosbankProviderClient) Topup(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	// Get price info from request
	price := req.Amount
	admin := 0
	if v, ok := req.Extra["admin"].(int); ok {
		admin = v
	}
	total := price + admin

	resp, err := client.SinglePayment(ctx, req.SKUCode, req.CustomerNo, req.RefID, price, admin, total)
	responseTime := time.Since(startTime)

	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertSinglePaymentResponse(resp, req.RefID, responseTime), nil
}

// Inquiry checks a postpaid bill
func (c *KiosbankProviderClient) Inquiry(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	// Extract BPJS periode from Extra
	periode := ""
	if v, ok := req.Extra["periode"].(string); ok {
		periode = v
	}

	resp, err := client.Inquiry(ctx, req.SKUCode, req.CustomerNo, req.RefID, periode)
	responseTime := time.Since(startTime)

	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertInquiryResponse(resp, req.RefID, responseTime), nil
}

// Payment pays a postpaid bill
func (c *KiosbankProviderClient) Payment(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	// Get amounts from request
	tagihan := req.Amount
	admin := 0
	if v, ok := req.Extra["admin"].(int); ok {
		admin = v
	}
	total := tagihan + admin

	// Extract product-specific fields
	noHandphone := ""
	if v, ok := req.Extra["noHandphone"].(string); ok {
		noHandphone = v
	}
	nama := ""
	if v, ok := req.Extra["nama"].(string); ok {
		nama = v
	}
	kode := ""
	if v, ok := req.Extra["kode"].(string); ok {
		kode = v
	}

	resp, err := client.Payment(ctx, req.SKUCode, req.CustomerNo, req.RefID, tagihan, admin, total, noHandphone, nama, kode)
	responseTime := time.Since(startTime)

	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertPaymentResponse(resp, req.RefID, responseTime), nil
}

// CheckStatus checks transaction status by looking up original transaction data
func (c *KiosbankProviderClient) CheckStatus(ctx context.Context, refID string) (*ProviderResponse, error) {
	// Look up the transaction by provider_ref_id to get original params
	trx, err := c.trxRepo.GetByProviderRefID(refID)
	if err != nil {
		return nil, fmt.Errorf("cannot check status: transaction not found for ref %s: %w", refID, err)
	}

	// Get provider SKU code (Kiosbank productID)
	if trx.ProviderSKUID == nil {
		return nil, fmt.Errorf("cannot check status: no provider SKU ID for transaction %s", trx.TransactionID)
	}
	providerSKU, err := c.providerRepo.GetProviderSKUByID(*trx.ProviderSKUID)
	if err != nil {
		return nil, fmt.Errorf("cannot check status: failed to get provider SKU: %w", err)
	}

	// Reconstruct original amounts
	tagihan := 0
	if trx.Amount != nil {
		tagihan = *trx.Amount
	}
	admin := trx.Admin
	total := tagihan + admin

	// Transaction date for tglTransaksi
	tglTransaksi := trx.CreatedAt.Format("2006-01-02")

	client := c.getClient(trx.IsSandbox)
	startTime := time.Now()

	resp, err := client.CheckStatus(ctx, providerSKU.ProviderSKUCode, trx.CustomerNo, trx.TransactionID, tagihan, admin, total, tglTransaksi)
	responseTime := time.Since(startTime)

	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertPaymentResponse(resp.ToPaymentResponse(), refID, responseTime), nil
}

// GetPriceList fetches current prices
func (c *KiosbankProviderClient) GetPriceList(ctx context.Context, category string) ([]ProviderProduct, error) {
	client := c.getClient(false)

	var products []ProviderProduct

	// Get pulsa/data price list
	pulsaResp, err := client.GetPriceListPulsa(ctx)
	if err != nil {
		return nil, err
	}

	for _, p := range pulsaResp.Record {
		price := parseAmount(p.Price)
		products = append(products, ProviderProduct{
			SKUCode:     p.Code,
			ProductName: p.Name,
			Price:       price,
			IsActive:    true, // pulsa pricelist doesn't have status field
		})
	}

	// Get general price list
	generalResp, err := client.GetPriceList(ctx)
	if err != nil {
		return nil, err
	}

	for _, p := range generalResp.Record {
		if category != "" && !strings.EqualFold(p.Category, category) {
			continue
		}
		price := parseAmount(p.Price)
		isActive := strings.EqualFold(p.Status, "AKTIF")
		products = append(products, ProviderProduct{
			SKUCode:     p.Code,
			ProductName: p.Name,
			Category:    p.Category,
			Price:       price,
			IsActive:    isActive,
		})
	}

	return products, nil
}

// IsHealthy returns whether the provider is healthy
func (c *KiosbankProviderClient) IsHealthy() bool {
	c.healthMu.RLock()
	defer c.healthMu.RUnlock()
	return c.healthy
}

// markHealthy marks the provider as healthy
func (c *KiosbankProviderClient) markHealthy() {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	c.healthy = true
}

// markUnhealthy marks the provider as unhealthy
func (c *KiosbankProviderClient) markUnhealthy() {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	c.healthy = false
}

// convertInquiryResponse converts Kiosbank inquiry response to unified format
func (c *KiosbankProviderClient) convertInquiryResponse(resp *kiosbank.InquiryResponse, refID string, responseTime time.Duration) *ProviderResponse {
	rawResp, _ := json.Marshal(resp)

	// Parse amounts
	amount := parseAmount(resp.Data.TotalTagihan)
	admin := parseAmount(resp.Data.Admin)

	// Create description
	desc := map[string]any{
		"idPelanggan":   resp.Data.IDPelanggan,
		"nama":          resp.Data.Nama,
		"periode":       resp.Data.Period,
		"jumlahTagihan": resp.Data.JumlahTagihan,
		"info":          resp.Data.Info,
	}
	if len(resp.Data.RincianTagihan) > 0 {
		desc["rincianTagihan"] = resp.Data.RincianTagihan
	}
	descJSON, _ := json.Marshal(desc)

	return &ProviderResponse{
		Success:       kiosbank.IsSuccess(resp.RC),
		Pending:       kiosbank.IsPending(resp.RC),
		RefID:         refID,
		ProviderRefID: resp.Data.NoReferensi,
		Status:        getStatusFromRC(resp.RC),
		RC:            resp.RC,
		Message:       resp.Description,
		CustomerName:  resp.Data.Nama,
		Amount:        amount,
		Admin:         admin,
		Description:   descJSON,
		RawResponse:   rawResp,
		NeedsRetry:    kiosbank.NeedsNewRefID(resp.RC),
		ResponseTime:  responseTime,
	}
}

// convertPaymentResponse converts Kiosbank payment response to unified format
func (c *KiosbankProviderClient) convertPaymentResponse(resp *kiosbank.PaymentResponse, refID string, responseTime time.Duration) *ProviderResponse {
	rawResp, _ := json.Marshal(resp)

	// Parse amounts
	amount := parseAmount(resp.Data.Tagihan)
	admin := parseAmount(resp.Data.Admin)

	// Extract serial number from known fields
	sn := resp.Data.SerialNumber
	if sn == "" {
		sn = resp.Data.Token
	}

	// Create description
	desc := map[string]any{
		"idPelanggan": resp.Data.IDPelanggan,
		"nama":        resp.Data.Nama,
		"noReferensi": resp.Data.NoReferensi,
		"status":      resp.Data.Status,
		"token":       resp.Data.Token,
		"kwh":         resp.Data.KWH,
	}
	if len(resp.Data.Info) > 0 {
		desc["info"] = resp.Data.Info
	}
	descJSON, _ := json.Marshal(desc)

	return &ProviderResponse{
		Success:       kiosbank.IsSuccess(resp.RC),
		Pending:       kiosbank.IsPending(resp.RC),
		RefID:         refID,
		ProviderRefID: resp.Data.NoReferensi,
		Status:        getStatusFromRC(resp.RC),
		RC:            resp.RC,
		Message:       resp.Description,
		SerialNumber:  sn,
		CustomerName:  resp.Data.Nama,
		Amount:        amount,
		Admin:         admin,
		Description:   descJSON,
		RawResponse:   rawResp,
		NeedsRetry:    kiosbank.NeedsNewRefID(resp.RC),
		ResponseTime:  responseTime,
	}
}

// convertSinglePaymentResponse converts Kiosbank single payment response to unified format
func (c *KiosbankProviderClient) convertSinglePaymentResponse(resp *kiosbank.SinglePaymentResponse, refID string, responseTime time.Duration) *ProviderResponse {
	rawResp, _ := json.Marshal(resp)

	// Parse amount
	amount := parseAmount(resp.Data.Harga)

	// Create description
	desc := map[string]any{
		"idPelanggan": resp.Data.IDPelanggan,
		"nama":        resp.Data.Nama,
		"noReferensi": resp.Data.NoReferensi,
		"status":      resp.Data.Status,
	}
	descJSON, _ := json.Marshal(desc)

	return &ProviderResponse{
		Success:       kiosbank.IsSuccess(resp.RC),
		Pending:       kiosbank.IsPending(resp.RC),
		RefID:         refID,
		ProviderRefID: resp.Data.NoReferensi,
		Status:        getStatusFromRC(resp.RC),
		RC:            resp.RC,
		Message:       resp.Description,
		SerialNumber:  resp.Data.NoReferensi, // Use noReferensi as SN for prepaid
		CustomerName:  resp.Data.Nama,
		Amount:        amount,
		Description:   descJSON,
		RawResponse:   rawResp,
		NeedsRetry:    kiosbank.NeedsNewRefID(resp.RC),
		ResponseTime:  responseTime,
	}
}

// parseAmount converts string amount to int, returns 0 and logs error if invalid
func parseAmount(s string) int {
	if s == "" {
		return 0
	}
	// Remove common formatting characters
	clean := strings.ReplaceAll(s, ".", "")
	clean = strings.ReplaceAll(clean, ",", "")
	clean = strings.ReplaceAll(clean, " ", "")
	clean = strings.TrimPrefix(clean, "Rp")
	clean = strings.TrimSpace(clean)

	v, err := strconv.Atoi(clean)
	if err != nil {
		log.Warn().Str("input", s).Err(err).Msg("parseAmount: failed to parse amount string")
		return 0
	}
	return v
}

// getStatusFromRC converts RC to status string
func getStatusFromRC(rc string) string {
	if kiosbank.IsSuccess(rc) {
		return "Success"
	}
	if kiosbank.IsPending(rc) {
		return "Pending"
	}
	return "Failed"
}
