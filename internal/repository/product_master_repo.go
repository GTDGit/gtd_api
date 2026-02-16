package repository

import (
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

// ProductCategory model
type ProductCategory struct {
	ID           int    `db:"id"`
	Name         string `db:"name"`
	DisplayOrder int    `db:"display_order"`
}

// ProductBrand model
type ProductBrand struct {
	ID           int    `db:"id"`
	Name         string `db:"name"`
	DisplayOrder int    `db:"display_order"`
}

// ProductType model
type ProductType struct {
	ID           int    `db:"id"`
	Name         string `db:"name"`
	Code         string `db:"code"`
	DisplayOrder int    `db:"display_order"`
}

// ProductMasterRepository handles product master data (categories, brands, types).
type ProductMasterRepository struct {
	db *sqlx.DB
}

// NewProductMasterRepository creates a new ProductMasterRepository.
func NewProductMasterRepository(db *sqlx.DB) *ProductMasterRepository {
	return &ProductMasterRepository{db: db}
}

// --- Categories ---

func (r *ProductMasterRepository) ListCategories() ([]ProductCategory, error) {
	const q = `SELECT id, name, display_order FROM product_categories ORDER BY display_order, name`
	var out []ProductCategory
	if err := r.db.Select(&out, q); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ProductMasterRepository) GetCategoryByID(id int) (*ProductCategory, error) {
	const q = `SELECT id, name, display_order FROM product_categories WHERE id = $1`
	var c ProductCategory
	if err := r.db.Get(&c, q, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *ProductMasterRepository) GetCategoryByName(name string) (*ProductCategory, error) {
	const q = `SELECT id, name, display_order FROM product_categories WHERE name = $1`
	var c ProductCategory
	if err := r.db.Get(&c, q, name); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *ProductMasterRepository) CreateCategory(name string, displayOrder int) (*ProductCategory, error) {
	const q = `INSERT INTO product_categories (name, display_order) VALUES ($1, $2) RETURNING id, name, display_order`
	var c ProductCategory
	if err := r.db.Get(&c, q, name, displayOrder); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *ProductMasterRepository) UpdateCategory(id int, name string, displayOrder int) error {
	const q = `UPDATE product_categories SET name = $2, display_order = $3, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(q, id, name, displayOrder)
	return err
}

func (r *ProductMasterRepository) CountProductsByCategoryName(name string) (int, error) {
	const q = `SELECT COUNT(*) FROM products WHERE category = $1`
	var n int
	if err := r.db.Get(&n, q, name); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *ProductMasterRepository) DeleteCategory(id int) error {
	c, _ := r.GetCategoryByID(id)
	if c == nil {
		return nil
	}
	n, _ := r.CountProductsByCategoryName(c.Name)
	if n > 0 {
		return errors.New("cannot delete: category is used by products")
	}
	const q = `DELETE FROM product_categories WHERE id = $1`
	_, err := r.db.Exec(q, id)
	return err
}

// --- Brands ---

func (r *ProductMasterRepository) ListBrands() ([]ProductBrand, error) {
	const q = `SELECT id, name, display_order FROM product_brands ORDER BY display_order, name`
	var out []ProductBrand
	if err := r.db.Select(&out, q); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ProductMasterRepository) GetBrandByID(id int) (*ProductBrand, error) {
	const q = `SELECT id, name, display_order FROM product_brands WHERE id = $1`
	var b ProductBrand
	if err := r.db.Get(&b, q, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

func (r *ProductMasterRepository) GetBrandByName(name string) (*ProductBrand, error) {
	const q = `SELECT id, name, display_order FROM product_brands WHERE name = $1`
	var b ProductBrand
	if err := r.db.Get(&b, q, name); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

func (r *ProductMasterRepository) CreateBrand(name string, displayOrder int) (*ProductBrand, error) {
	const q = `INSERT INTO product_brands (name, display_order) VALUES ($1, $2) RETURNING id, name, display_order`
	var b ProductBrand
	if err := r.db.Get(&b, q, name, displayOrder); err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *ProductMasterRepository) UpdateBrand(id int, name string, displayOrder int) error {
	const q = `UPDATE product_brands SET name = $2, display_order = $3, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(q, id, name, displayOrder)
	return err
}

func (r *ProductMasterRepository) CountProductsByBrandName(name string) (int, error) {
	const q = `SELECT COUNT(*) FROM products WHERE brand = $1`
	var n int
	if err := r.db.Get(&n, q, name); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *ProductMasterRepository) DeleteBrand(id int) error {
	b, _ := r.GetBrandByID(id)
	if b == nil {
		return nil
	}
	n, _ := r.CountProductsByBrandName(b.Name)
	if n > 0 {
		return errors.New("cannot delete: brand is used by products")
	}
	const q = `DELETE FROM product_brands WHERE id = $1`
	_, err := r.db.Exec(q, id)
	return err
}

// --- Types ---

func (r *ProductMasterRepository) ListTypes() ([]ProductType, error) {
	const q = `SELECT id, name, code, display_order FROM product_types ORDER BY display_order, name`
	var out []ProductType
	if err := r.db.Select(&out, q); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ProductMasterRepository) GetTypeByID(id int) (*ProductType, error) {
	const q = `SELECT id, name, code, display_order FROM product_types WHERE id = $1`
	var t ProductType
	if err := r.db.Get(&t, q, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (r *ProductMasterRepository) GetTypeByCode(code string) (*ProductType, error) {
	const q = `SELECT id, name, code, display_order FROM product_types WHERE code = $1`
	var t ProductType
	if err := r.db.Get(&t, q, code); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (r *ProductMasterRepository) CreateType(name, code string, displayOrder int) (*ProductType, error) {
	const q = `INSERT INTO product_types (name, code, display_order) VALUES ($1, $2, $3) RETURNING id, name, code, display_order`
	var t ProductType
	if err := r.db.Get(&t, q, name, code, displayOrder); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *ProductMasterRepository) UpdateType(id int, name, code string, displayOrder int) error {
	const q = `UPDATE product_types SET name = $2, code = $3, display_order = $4, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(q, id, name, code, displayOrder)
	return err
}

func (r *ProductMasterRepository) CountProductsByTypeCode(code string) (int, error) {
	const q = `SELECT COUNT(*) FROM products WHERE type = $1`
	var n int
	if err := r.db.Get(&n, q, code); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *ProductMasterRepository) DeleteType(id int) error {
	t, _ := r.GetTypeByID(id)
	if t == nil {
		return nil
	}
	n, _ := r.CountProductsByTypeCode(t.Code)
	if n > 0 {
		return errors.New("cannot delete: type is used by products")
	}
	const q = `DELETE FROM product_types WHERE id = $1`
	_, err := r.db.Exec(q, id)
	return err
}
