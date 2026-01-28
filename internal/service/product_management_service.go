package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

// ProductManagementService handles product CRUD operations.
type ProductManagementService struct {
	productRepo *repository.ProductRepository
	skuRepo     *repository.SKURepository
}

// NewProductManagementService constructs a ProductManagementService.
func NewProductManagementService(productRepo *repository.ProductRepository, skuRepo *repository.SKURepository) *ProductManagementService {
	return &ProductManagementService{
		productRepo: productRepo,
		skuRepo:     skuRepo,
	}
}

// CreateProductRequest represents the request to create a new product.
type CreateProductRequest struct {
    SKUCode     string `json:"skuCode" binding:"required"`
    Name        string `json:"name" binding:"required"`
    Category    string `json:"category" binding:"required"`
    Brand       string `json:"brand" binding:"required"`
    Type        string `json:"type" binding:"required"` // "prepaid" or "postpaid"
    Admin       int    `json:"admin"`
    Commission  int    `json:"commission"`
    Description string `json:"description"`
}

// UpdateProductRequest represents the request to update a product.
type UpdateProductRequest struct {
    SKUCode     string `json:"skuCode"`
    Name        string `json:"name"`
    Category    string `json:"category"`
    Brand       string `json:"brand"`
    Type        string `json:"type"`
    Admin       int    `json:"admin"`
    Commission  int    `json:"commission"`
    Description string `json:"description"`
    IsActive    *bool  `json:"isActive"`
}

// CreateSKURequest represents the request to create/update a SKU.
type CreateSKURequest struct {
    DigiSKUCode    string `json:"digiSkuCode" binding:"required"`
    SellerName     string `json:"sellerName"`
    Priority       int    `json:"priority" binding:"required"` // 1, 2, or 3
    Price          int    `json:"price" binding:"required"`
    IsActive       bool   `json:"isActive"`
    SupportMulti   bool   `json:"supportMulti"`
    UnlimitedStock bool   `json:"unlimitedStock"`
    Stock          int    `json:"stock"`
    CutOffStart    string `json:"cutOffStart"` // "HH:MM:SS"
    CutOffEnd      string `json:"cutOffEnd"`   // "HH:MM:SS"
}

// UpdateSKURequest represents the request to update a SKU.
type UpdateSKURequest struct {
    DigiSKUCode    string `json:"digiSkuCode"`
    SellerName     string `json:"sellerName"`
    Priority       *int   `json:"priority"`
    Price          *int   `json:"price"`
    IsActive       *bool  `json:"isActive"`
    SupportMulti   *bool  `json:"supportMulti"`
    UnlimitedStock *bool  `json:"unlimitedStock"`
    Stock          *int   `json:"stock"`
    CutOffStart    string `json:"cutOffStart"`
    CutOffEnd      string `json:"cutOffEnd"`
}

// CreateProduct creates a new product.
func (s *ProductManagementService) CreateProduct(ctx context.Context, req *CreateProductRequest) (*models.Product, error) {
	// Validate type
	if req.Type != "prepaid" && req.Type != "postpaid" {
		return nil, errors.New("type must be 'prepaid' or 'postpaid'")
	}

	// Check if SKU code already exists
	existing, _ := s.productRepo.GetBySKUCode(req.SKUCode)
	if existing != nil {
		return nil, errors.New("sku_code already exists")
	}

	product := &models.Product{
		SkuCode:     req.SKUCode,
		Name:        req.Name,
		Category:    req.Category,
		Brand:       req.Brand,
		Type:        models.ProductType(req.Type),
		Admin:       req.Admin,
		Commission:  req.Commission,
		Description: req.Description,
		IsActive:    false, // Will be updated by SKU trigger
	}

	if err := s.productRepo.Create(product); err != nil {
		return nil, err
	}

	return product, nil
}

// GetProduct retrieves a product by ID.
func (s *ProductManagementService) GetProduct(id int) (*models.Product, error) {
	product, err := s.productRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("product not found")
		}
		return nil, err
	}
	return product, nil
}

// UpdateProduct updates a product.
func (s *ProductManagementService) UpdateProduct(id int, req *UpdateProductRequest) (*models.Product, error) {
	product, err := s.productRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("product not found")
		}
		return nil, err
	}

	// Update fields if provided
	if req.SKUCode != "" {
		// Check if new SKU code already exists
		if req.SKUCode != product.SkuCode {
			existing, _ := s.productRepo.GetBySKUCode(req.SKUCode)
			if existing != nil {
				return nil, errors.New("sku_code already exists")
			}
		}
		product.SkuCode = req.SKUCode
	}
	if req.Name != "" {
		product.Name = req.Name
	}
	if req.Category != "" {
		product.Category = req.Category
	}
	if req.Brand != "" {
		product.Brand = req.Brand
	}
	if req.Type != "" {
		if req.Type != "prepaid" && req.Type != "postpaid" {
			return nil, errors.New("type must be 'prepaid' or 'postpaid'")
		}
		product.Type = models.ProductType(req.Type)
	}
	if req.Admin > 0 {
		product.Admin = req.Admin
	}
	if req.Commission > 0 {
		product.Commission = req.Commission
	}
	if req.Description != "" {
		product.Description = req.Description
	}
	if req.IsActive != nil {
		product.IsActive = *req.IsActive
	}

	if err := s.productRepo.Update(product); err != nil {
		return nil, err
	}

	return product, nil
}

// DeleteProduct deletes a product.
func (s *ProductManagementService) DeleteProduct(id int) error {
	product, err := s.productRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return errors.New("product not found")
		}
		return err
	}

	return s.productRepo.Delete(product.ID)
}

// CreateSKU creates a new SKU for a product.
func (s *ProductManagementService) CreateSKU(productID int, req *CreateSKURequest) (*models.SKU, error) {
	// Validate priority
	if req.Priority < 1 || req.Priority > 3 {
		return nil, errors.New("priority must be 1, 2, or 3")
	}

	// Check if product exists
	_, err := s.productRepo.GetByID(productID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("product not found")
		}
		return nil, err
	}

	// Check if DigiSKUCode already exists
	existing, _ := s.skuRepo.GetByDigiSKUCode(req.DigiSKUCode)
	if existing != nil {
		return nil, errors.New("digi_sku_code already exists")
	}

	sku := &models.SKU{
		ProductID:      productID,
		DigiSkuCode:    req.DigiSKUCode,
		SellerName:     req.SellerName,
		Priority:       req.Priority,
		Price:          req.Price,
		IsActive:       req.IsActive,
		SupportMulti:   req.SupportMulti,
		UnlimitedStock: req.UnlimitedStock,
		Stock:          req.Stock,
		CutOffStart:    req.CutOffStart,
		CutOffEnd:      req.CutOffEnd,
	}

	if err := s.skuRepo.Create(sku); err != nil {
		return nil, err
	}

	return sku, nil
}

// GetSKU retrieves a SKU by ID.
func (s *ProductManagementService) GetSKU(id int) (*models.SKU, error) {
	sku, err := s.skuRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("sku not found")
		}
		return nil, err
	}
	return sku, nil
}

// UpdateSKU updates a SKU.
func (s *ProductManagementService) UpdateSKU(id int, req *UpdateSKURequest) (*models.SKU, error) {
	sku, err := s.skuRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("sku not found")
		}
		return nil, err
	}

	// Update fields if provided
	if req.DigiSKUCode != "" {
		if req.DigiSKUCode != sku.DigiSkuCode {
			existing, _ := s.skuRepo.GetByDigiSKUCode(req.DigiSKUCode)
			if existing != nil {
				return nil, errors.New("digi_sku_code already exists")
			}
		}
		sku.DigiSkuCode = req.DigiSKUCode
	}
	if req.SellerName != "" {
		sku.SellerName = req.SellerName
	}
	if req.Priority != nil {
		if *req.Priority < 1 || *req.Priority > 3 {
			return nil, errors.New("priority must be 1, 2, or 3")
		}
		sku.Priority = *req.Priority
	}
	if req.Price != nil {
		sku.Price = *req.Price
	}
	if req.IsActive != nil {
		sku.IsActive = *req.IsActive
	}
	if req.SupportMulti != nil {
		sku.SupportMulti = *req.SupportMulti
	}
	if req.UnlimitedStock != nil {
		sku.UnlimitedStock = *req.UnlimitedStock
	}
	if req.Stock != nil {
		sku.Stock = *req.Stock
	}
	if req.CutOffStart != "" {
		sku.CutOffStart = req.CutOffStart
	}
	if req.CutOffEnd != "" {
		sku.CutOffEnd = req.CutOffEnd
	}

	if err := s.skuRepo.Update(sku); err != nil {
		return nil, err
	}

	return sku, nil
}

// DeleteSKU deletes a SKU.
func (s *ProductManagementService) DeleteSKU(id int) error {
	sku, err := s.skuRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return errors.New("sku not found")
		}
		return err
	}

	return s.skuRepo.Delete(sku.ID)
}

// GetProductSKUs retrieves all SKUs for a product.
func (s *ProductManagementService) GetProductSKUs(productID int) ([]*models.SKU, error) {
	// Check if product exists
	_, err := s.productRepo.GetByID(productID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("product not found")
		}
		return nil, err
	}

	skus, err := s.skuRepo.GetByProductID(productID)
	if err != nil {
		return nil, err
	}

	// Convert []models.SKU to []*models.SKU
	result := make([]*models.SKU, len(skus))
	for i := range skus {
		result[i] = &skus[i]
	}
	return result, nil
}
