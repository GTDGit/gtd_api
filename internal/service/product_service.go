package service

import (
    "time"

    "github.com/GTDGit/gtd_api/internal/models"
    "github.com/GTDGit/gtd_api/internal/repository"
)

// ProductService provides product-related business logic.
type ProductService struct {
    productRepo *repository.ProductRepository
    skuRepo     *repository.SKURepository
}

// NewProductService constructs a ProductService.
func NewProductService(productRepo *repository.ProductRepository, skuRepo *repository.SKURepository) *ProductService {
    return &ProductService{productRepo: productRepo, skuRepo: skuRepo}
}

// ProductResponse is the outward-facing payload for product listing.
type ProductResponse struct {
    SkuCode       string    `json:"skuCode"`
    Name          string    `json:"name"`
    Category      string    `json:"category"`
    Brand         string    `json:"brand"`
    Type          string    `json:"type"`
    Price         int       `json:"price,omitempty"`
    Admin         int       `json:"admin,omitempty"`
    Commission    int       `json:"commission,omitempty"`
    IsActive      bool      `json:"isActive"`
    Description   string    `json:"description"`
    UpdatedAt     time.Time `json:"updatedAt"`
}

// GetProducts returns products with filters and pagination and enriches
// with the main SKU price (priority=1) for prepaid products. It also returns total items.
func (s *ProductService) GetProducts(productType, category, brand, search string, page, limit int) ([]ProductResponse, int, error) {
    products, total, err := s.productRepo.GetAllPaged(productType, category, brand, search, page, limit)
    if err != nil {
        return nil, 0, err
    }

    result := make([]ProductResponse, 0, len(products))
    for _, p := range products {
        price, _ := s.skuRepo.GetMainSKUPrice(p.ID)
        result = append(result, ProductResponse{
            SkuCode:     p.SkuCode,
            Name:        p.Name,
            Category:    p.Category,
            Brand:       p.Brand,
            Type:        string(p.Type),
            Price:       price,
            Admin:       p.Admin,
            Commission:  p.Commission,
            IsActive:    p.IsActive,
            Description: p.Description,
            UpdatedAt:   p.UpdatedAt,
        })
    }
    return result, total, nil
}

// GetAvailableSKUs returns SKUs that are not in cutoff at the current WIB time.
func (s *ProductService) GetAvailableSKUs(productID int) ([]models.SKU, error) {
    wib := time.FixedZone("WIB", 7*3600) // UTC+7
    currentTime := time.Now().In(wib).Format("15:04:05")
    return s.skuRepo.GetAvailableSKUs(productID, currentTime)
}

// GetProductBySkuCode returns a product by sku code.
func (s *ProductService) GetProductBySkuCode(skuCode string) (*models.Product, error) {
    return s.productRepo.GetBySKUCode(skuCode)
}

// GetProductByID returns a product by ID.
func (s *ProductService) GetProductByID(productID int) (*models.Product, error) {
    return s.productRepo.GetByID(productID)
}
