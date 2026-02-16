package service

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/pkg/digiflazz"
)

// SyncService periodically syncs products/SKUs with Digiflazz pricelist.
type SyncService struct {
	digiflazz   *digiflazz.Client
	productRepo *repository.ProductRepository
	skuRepo     *repository.SKURepository
}

// NewSyncService constructs a SyncService.
func NewSyncService(digi *digiflazz.Client, productRepo *repository.ProductRepository, skuRepo *repository.SKURepository) *SyncService {
	return &SyncService{digiflazz: digi, productRepo: productRepo, skuRepo: skuRepo}
}

// SyncPricelist fetches pricelists for prepaid and postpaid and processes them with smart SKU grouping.
// Naming convention:
//   - Main SKU: TSEL5 (priority 1)
//   - Backup 1: TSEL5B1 (priority 2)
//   - Backup 2: TSEL5B2 (priority 3)
func (s *SyncService) SyncPricelist(ctx context.Context) error {
	// Sync prepaid
	prepaidList, err := s.digiflazz.GetPricelist(ctx, "prepaid")
	if err != nil {
		return err
	}
	s.processPricelist(prepaidList.Data, "prepaid")

	// Sync postpaid
	pascaList, err := s.digiflazz.GetPricelist(ctx, "pasca")
	if err != nil {
		return err
	}
	s.processPricelist(pascaList.Data, "postpaid")

	return nil
}

// processPricelist uses smart SKU grouping logic.
// It groups SKUs by base code (TSEL5, TSEL5B1, TSEL5B2 all belong to TSEL5).
// For each group:
//  1. Find main SKU (without B1/B2 suffix)
//  2. Create/update product with main SKU data
//  3. Create/update SKUs with priority 1 (main), 2 (B1), 3 (B2)
//  4. If backup SKUs don't exist, skip them
func (s *SyncService) processPricelist(items []digiflazz.PricelistItem, productType string) {
	// Group items by base SKU code
	groups := make(map[string][]*digiflazz.PricelistItem)

	for i := range items {
		item := &items[i]
		baseSKU := getBaseSKUCode(item.BuyerSkuCode)
		groups[baseSKU] = append(groups[baseSKU], item)
	}

	// Process each group
	for baseSKU, skuItems := range groups {
		s.processSkuGroup(baseSKU, skuItems, productType)
	}
}

// processSkuGroup processes a group of SKUs (main + backups).
func (s *SyncService) processSkuGroup(baseSKU string, items []*digiflazz.PricelistItem, productType string) {
	// Find main, B1, and B2 SKUs
	var mainItem, b1Item, b2Item *digiflazz.PricelistItem

	for _, item := range items {
		sku := item.BuyerSkuCode
		if sku == baseSKU {
			mainItem = item
		} else if sku == baseSKU+"B1" {
			b1Item = item
		} else if sku == baseSKU+"B2" {
			b2Item = item
		}
	}

	// Main SKU is required
	if mainItem == nil {
		return
	}

	// Find or create product using main SKU data
	product, err := s.productRepo.GetBySKUCode(baseSKU)
	if err != nil || product == nil {
		// Create new product
		product = &models.Product{
			SkuCode:     baseSKU,
			Name:        mainItem.ProductName,
			Category:    mainItem.Category,
			Brand:       mainItem.Brand,
			Type:        models.ProductType(productType),
			Admin:       mainItem.Admin,
			Commission:  mainItem.Commission,
			Description: mainItem.Desc,
			IsActive:    false, // Will be updated by trigger
		}

		if err := s.productRepo.Create(product); err != nil {
			log.Error().Err(err).Str("sku", baseSKU).Msg("failed to create product")
			return
		}
	} else {
		// Update existing product with latest data from main SKU
		product.Name = mainItem.ProductName
		product.Category = mainItem.Category
		product.Brand = mainItem.Brand
		product.Type = models.ProductType(productType)
		product.Admin = mainItem.Admin
		product.Commission = mainItem.Commission
		product.Description = mainItem.Desc

		if err := s.productRepo.Update(product); err != nil {
			log.Error().Err(err).Str("sku", baseSKU).Msg("failed to update product")
			return
		}
	}

	// Upsert main SKU (priority 1)
	s.upsertSKU(product.ID, mainItem, 1)

	// Upsert B1 SKU if exists (priority 2)
	if b1Item != nil {
		s.upsertSKU(product.ID, b1Item, 2)
	}

	// Upsert B2 SKU if exists (priority 3)
	if b2Item != nil {
		s.upsertSKU(product.ID, b2Item, 3)
	}
}

// upsertSKU creates or updates a SKU.
func (s *SyncService) upsertSKU(productID int, item *digiflazz.PricelistItem, priority int) {
	sku := &models.SKU{
		ProductID:      productID,
		DigiSkuCode:    item.BuyerSkuCode,
		SellerName:     item.SellerName,
		Priority:       priority,
		Price:          item.Price,
		IsActive:       item.BuyerProductStatus && item.SellerProductStatus,
		SupportMulti:   item.Multi,
		UnlimitedStock: item.UnlimitedStock,
		Stock:          item.Stock,
		CutOffStart:    normalizeCutoff(item.StartCutOff),
		CutOffEnd:      normalizeCutoff(item.EndCutOff),
	}

	if err := s.skuRepo.Upsert(sku); err != nil {
		log.Error().Err(err).Str("digi_sku", item.BuyerSkuCode).Int("priority", priority).Msg("failed to upsert SKU")
	} else {
		log.Debug().Str("digi_sku", item.BuyerSkuCode).Int("priority", priority).Msg("SKU synced")
	}
}

// getBaseSKUCode extracts the base SKU code by removing B1/B2 suffix.
// Example: TSEL5B1 → TSEL5, TSEL5B2 → TSEL5, TSEL5 → TSEL5
func getBaseSKUCode(skuCode string) string {
	if strings.HasSuffix(skuCode, "B2") {
		return strings.TrimSuffix(skuCode, "B2")
	}
	if strings.HasSuffix(skuCode, "B1") {
		return strings.TrimSuffix(skuCode, "B1")
	}
	return skuCode
}

// normalizeCutoff ensures cut-off time strings are in HH:MM:SS format; if empty, returns 00:00:00
func normalizeCutoff(v string) string {
	if v == "" {
		return "00:00:00"
	}
	// If already HH:MM:SS, keep; if HH:MM, append :00
	if len(v) == 5 {
		return v + ":00"
	}
	// Best effort: try to parse and reformat
	if t, err := time.Parse("15:04:05", v); err == nil {
		return t.Format("15:04:05")
	}
	if t, err := time.Parse("15:04", v); err == nil {
		return t.Format("15:04:05")
	}
	return "00:00:00"
}
