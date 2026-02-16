package service

import (
	"context"
	"errors"
	"strings"

	"github.com/GTDGit/gtd_api/internal/repository"
)

// ProductMasterService handles CRUD for product categories, brands, and types.
type ProductMasterService struct {
	repo *repository.ProductMasterRepository
}

// NewProductMasterService creates a new ProductMasterService.
func NewProductMasterService(repo *repository.ProductMasterRepository) *ProductMasterService {
	return &ProductMasterService{repo: repo}
}

// --- Categories ---

func (s *ProductMasterService) ListCategories() ([]repository.ProductCategory, error) {
	return s.repo.ListCategories()
}

func (s *ProductMasterService) CreateCategory(ctx context.Context, name string, displayOrder int) (*repository.ProductCategory, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("category name is required")
	}
	existing, _ := s.repo.GetCategoryByName(name)
	if existing != nil {
		return nil, errors.New("category already exists")
	}
	return s.repo.CreateCategory(name, displayOrder)
}

func (s *ProductMasterService) UpdateCategory(ctx context.Context, id int, name string, displayOrder int) error {
	c, _ := s.repo.GetCategoryByID(id)
	if c == nil {
		return errors.New("category not found")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("category name is required")
	}
	existing, _ := s.repo.GetCategoryByName(name)
	if existing != nil && existing.ID != id {
		return errors.New("category name already used by another")
	}
	return s.repo.UpdateCategory(id, name, displayOrder)
}

func (s *ProductMasterService) DeleteCategory(ctx context.Context, id int) error {
	c, _ := s.repo.GetCategoryByID(id)
	if c == nil {
		return errors.New("category not found")
	}
	return s.repo.DeleteCategory(id)
}

// --- Brands ---

func (s *ProductMasterService) ListBrands() ([]repository.ProductBrand, error) {
	return s.repo.ListBrands()
}

func (s *ProductMasterService) CreateBrand(ctx context.Context, name string, displayOrder int) (*repository.ProductBrand, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("brand name is required")
	}
	existing, _ := s.repo.GetBrandByName(name)
	if existing != nil {
		return nil, errors.New("brand already exists")
	}
	return s.repo.CreateBrand(name, displayOrder)
}

func (s *ProductMasterService) UpdateBrand(ctx context.Context, id int, name string, displayOrder int) error {
	c, err := s.repo.GetBrandByID(id)
	if err != nil || c == nil {
		return errors.New("brand not found")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("brand name is required")
	}
	existing, _ := s.repo.GetBrandByName(name)
	if existing != nil && existing.ID != id {
		return errors.New("brand name already used by another")
	}
	return s.repo.UpdateBrand(id, name, displayOrder)
}

func (s *ProductMasterService) DeleteBrand(ctx context.Context, id int) error {
	c, _ := s.repo.GetBrandByID(id)
	if c == nil {
		return errors.New("brand not found")
	}
	return s.repo.DeleteBrand(id)
}

// --- Types ---

func (s *ProductMasterService) ListTypes() ([]repository.ProductType, error) {
	return s.repo.ListTypes()
}

func (s *ProductMasterService) CreateType(ctx context.Context, name, code string, displayOrder int) (*repository.ProductType, error) {
	name = strings.TrimSpace(name)
	code = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(code, " ", "_")))
	if name == "" || code == "" {
		return nil, errors.New("type name and code are required")
	}
	existing, _ := s.repo.GetTypeByCode(code)
	if existing != nil {
		return nil, errors.New("type code already exists")
	}
	return s.repo.CreateType(name, code, displayOrder)
}

func (s *ProductMasterService) UpdateType(ctx context.Context, id int, name, code string, displayOrder int) error {
	t, _ := s.repo.GetTypeByID(id)
	if t == nil {
		return errors.New("type not found")
	}
	name = strings.TrimSpace(name)
	code = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(code, " ", "_")))
	if name == "" || code == "" {
		return errors.New("type name and code are required")
	}
	existing, _ := s.repo.GetTypeByCode(code)
	if existing != nil && existing.ID != id {
		return errors.New("type code already used by another")
	}
	return s.repo.UpdateType(id, name, code, displayOrder)
}

func (s *ProductMasterService) DeleteType(ctx context.Context, id int) error {
	t, _ := s.repo.GetTypeByID(id)
	if t == nil {
		return errors.New("type not found")
	}
	return s.repo.DeleteType(id)
}

// ValidateCategoryBrandType checks that category, brand, type exist in master tables.
func (s *ProductMasterService) ValidateCategoryBrandType(category, brand, typeCode string) error {
	if category == "" {
		return errors.New("category is required")
	}
	if brand == "" {
		return errors.New("brand is required")
	}
	if typeCode == "" {
		return errors.New("type is required")
	}
	cat, _ := s.repo.GetCategoryByName(category)
	if cat == nil {
		return errors.New("invalid category: not found in master list")
	}
	b, _ := s.repo.GetBrandByName(brand)
	if b == nil {
		return errors.New("invalid brand: not found in master list")
	}
	t, _ := s.repo.GetTypeByCode(typeCode)
	if t == nil {
		return errors.New("invalid type: not found in master list")
	}
	return nil
}
