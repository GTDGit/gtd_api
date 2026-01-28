package handler

import (
    "strconv"

    "github.com/gin-gonic/gin"

    "github.com/GTDGit/gtd_api/internal/service"
    "github.com/GTDGit/gtd_api/internal/utils"
)

// ProductHandler handles product-related HTTP endpoints.
type ProductHandler struct {
    productService *service.ProductService
}

// NewProductHandler constructs a ProductHandler.
func NewProductHandler(productService *service.ProductService) *ProductHandler {
    return &ProductHandler{productService: productService}
}

// GetProducts returns the product list with optional filters and pagination.
func (h *ProductHandler) GetProducts(c *gin.Context) {
    productType := c.Query("type")   // prepaid, postpaid
    category := c.Query("category")  // Pulsa, Data, PLN, etc
    brand := c.Query("brand")
    search := c.Query("search")

    // pagination
    page := 1
    limit := 50
    if v := c.Query("page"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n > 0 {
            page = n
        }
    }
    if v := c.Query("limit"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n > 0 {
            limit = n
        }
    }

    products, total, err := h.productService.GetProducts(productType, category, brand, search, page, limit)
    if err != nil {
        utils.Error(c, 500, "INTERNAL_ERROR", "Failed to get products")
        return
    }

    utils.SuccessWithPagination(c, 200, "Products retrieved successfully", gin.H{
        "products": products,
    }, page, limit, total)
}
