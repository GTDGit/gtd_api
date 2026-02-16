package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/internal/utils"
)

// ProductMasterHandler handles CRUD for categories, brands, types.
type ProductMasterHandler struct {
	svc *service.ProductMasterService
}

// NewProductMasterHandler creates a new ProductMasterHandler.
func NewProductMasterHandler(svc *service.ProductMasterService) *ProductMasterHandler {
	return &ProductMasterHandler{svc: svc}
}

// --- Categories ---

func (h *ProductMasterHandler) ListCategories(c *gin.Context) {
	list, err := h.svc.ListCategories()
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to list categories")
		return
	}
	utils.Success(c, 200, "Categories retrieved", list)
}

func (h *ProductMasterHandler) CreateCategory(c *gin.Context) {
	var req struct {
		Name         string `json:"name" binding:"required"`
		DisplayOrder int    `json:"displayOrder"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}
	cat, err := h.svc.CreateCategory(c.Request.Context(), req.Name, req.DisplayOrder)
	if err != nil {
		if err.Error() == "category name is required" || err.Error() == "category already exists" {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to create category")
		return
	}
	utils.Success(c, 201, "Category created", cat)
}

func (h *ProductMasterHandler) UpdateCategory(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid category ID")
		return
	}
	var req struct {
		Name         string `json:"name" binding:"required"`
		DisplayOrder int    `json:"displayOrder"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}
	if err := h.svc.UpdateCategory(c.Request.Context(), id, req.Name, req.DisplayOrder); err != nil {
		if err.Error() == "category not found" {
			utils.Error(c, 404, "NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "category name is required" || err.Error() == "category name already used by another" {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to update category")
		return
	}
	utils.Success(c, 200, "Category updated", nil)
}

func (h *ProductMasterHandler) DeleteCategory(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid category ID")
		return
	}
	if err := h.svc.DeleteCategory(c.Request.Context(), id); err != nil {
		if err.Error() == "category not found" {
			utils.Error(c, 404, "NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "cannot delete: category is used by products" {
			utils.Error(c, 400, "IN_USE", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to delete category")
		return
	}
	utils.Success(c, 200, "Category deleted", nil)
}

// --- Brands ---

func (h *ProductMasterHandler) ListBrands(c *gin.Context) {
	list, err := h.svc.ListBrands()
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to list brands")
		return
	}
	utils.Success(c, 200, "Brands retrieved", list)
}

func (h *ProductMasterHandler) CreateBrand(c *gin.Context) {
	var req struct {
		Name         string `json:"name" binding:"required"`
		DisplayOrder int    `json:"displayOrder"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}
	brand, err := h.svc.CreateBrand(c.Request.Context(), req.Name, req.DisplayOrder)
	if err != nil {
		if err.Error() == "brand name is required" || err.Error() == "brand already exists" {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to create brand")
		return
	}
	utils.Success(c, 201, "Brand created", brand)
}

func (h *ProductMasterHandler) UpdateBrand(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid brand ID")
		return
	}
	var req struct {
		Name         string `json:"name" binding:"required"`
		DisplayOrder int    `json:"displayOrder"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}
	if err := h.svc.UpdateBrand(c.Request.Context(), id, req.Name, req.DisplayOrder); err != nil {
		if err.Error() == "brand not found" {
			utils.Error(c, 404, "NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "brand name is required" || err.Error() == "brand name already used by another" {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to update brand")
		return
	}
	utils.Success(c, 200, "Brand updated", nil)
}

func (h *ProductMasterHandler) DeleteBrand(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid brand ID")
		return
	}
	if err := h.svc.DeleteBrand(c.Request.Context(), id); err != nil {
		if err.Error() == "brand not found" {
			utils.Error(c, 404, "NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "cannot delete: brand is used by products" {
			utils.Error(c, 400, "IN_USE", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to delete brand")
		return
	}
	utils.Success(c, 200, "Brand deleted", nil)
}

// --- Types ---

func (h *ProductMasterHandler) ListTypes(c *gin.Context) {
	list, err := h.svc.ListTypes()
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to list types")
		return
	}
	utils.Success(c, 200, "Types retrieved", list)
}

func (h *ProductMasterHandler) CreateType(c *gin.Context) {
	var req struct {
		Name         string `json:"name" binding:"required"`
		Code         string `json:"code" binding:"required"`
		DisplayOrder int    `json:"displayOrder"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}
	typ, err := h.svc.CreateType(c.Request.Context(), req.Name, req.Code, req.DisplayOrder)
	if err != nil {
		if err.Error() == "type name and code are required" || err.Error() == "type code already exists" {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to create type")
		return
	}
	utils.Success(c, 201, "Type created", typ)
}

func (h *ProductMasterHandler) UpdateType(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid type ID")
		return
	}
	var req struct {
		Name         string `json:"name" binding:"required"`
		Code         string `json:"code" binding:"required"`
		DisplayOrder int    `json:"displayOrder"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}
	if err := h.svc.UpdateType(c.Request.Context(), id, req.Name, req.Code, req.DisplayOrder); err != nil {
		if err.Error() == "type not found" {
			utils.Error(c, 404, "NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "type name and code are required" || err.Error() == "type code already used by another" {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to update type")
		return
	}
	utils.Success(c, 200, "Type updated", nil)
}

func (h *ProductMasterHandler) DeleteType(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid type ID")
		return
	}
	if err := h.svc.DeleteType(c.Request.Context(), id); err != nil {
		if err.Error() == "type not found" {
			utils.Error(c, 404, "NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "cannot delete: type is used by products" {
			utils.Error(c, 400, "IN_USE", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to delete type")
		return
	}
	utils.Success(c, 200, "Type deleted", nil)
}
