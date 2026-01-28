package repository

import (
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_api/internal/models"
)

// ProductRepository handles data access for products.
type ProductRepository struct {
	db *sqlx.DB
}

// NewProductRepository creates a new ProductRepository.
func NewProductRepository(db *sqlx.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

// GetAll returns all active products with optional filters for type and category.
// When productType or category is an empty string, the filter is ignored respectively.
func (r *ProductRepository) GetAll(productType, category string) ([]models.Product, error) {
	const q = `
        SELECT * FROM products 
        WHERE ($1 = '' OR type = $1) 
        AND ($2 = '' OR category = $2)
        AND is_active = true
        ORDER BY category, brand, name`

	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var products []models.Product
	if err := stmt.Select(&products, productType, category); err != nil {
		return nil, err
	}
	return products, nil
}

// GetAllPaged returns active products with filters and pagination and also returns total count.
// Filters: productType (prepaid/postpaid), category, brand (exact), search (ILIKE on name).
// If a filter is empty it will be ignored. Page begins at 1.
func (r *ProductRepository) GetAllPaged(productType, category, brand, search string, page, limit int) ([]models.Product, int, error) {
    if page <= 0 {
        page = 1
    }
    if limit <= 0 {
        limit = 50
    }
    offset := (page - 1) * limit

    // Base WHERE clause
    const baseWhere = `WHERE ($1 = '' OR type = $1)
        AND ($2 = '' OR category = $2)
        AND ($3 = '' OR brand = $3)
        AND ($4 = '' OR name ILIKE '%%' || $4 || '%%')
        AND is_active = true`

    // Count total
    countQuery := `SELECT COUNT(1) FROM products ` + baseWhere
    var total int
    if err := r.db.Get(&total, countQuery, productType, category, brand, search); err != nil {
        return nil, 0, err
    }

    // Fetch page
    listQuery := `SELECT * FROM products ` + baseWhere + `
        ORDER BY category, brand, name LIMIT $5 OFFSET $6`
    var products []models.Product
    if err := r.db.Select(&products, listQuery, productType, category, brand, search, limit, offset); err != nil {
        return nil, 0, err
    }
    return products, total, nil
}

// GetBySKUCode returns a single product by sku_code.
func (r *ProductRepository) GetBySKUCode(skuCode string) (*models.Product, error) {
	const q = `SELECT * FROM products WHERE sku_code = $1 LIMIT 1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var p models.Product
	if err := stmt.Get(&p, skuCode); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &p, nil
}

// GetByID returns a single product by id.
func (r *ProductRepository) GetByID(id int) (*models.Product, error) {
	const q = `SELECT * FROM products WHERE id = $1 LIMIT 1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var p models.Product
	if err := stmt.Get(&p, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &p, nil
}

// Upsert inserts or updates a product by sku_code.
func (r *ProductRepository) Upsert(product *models.Product) error {
	const q = `
        INSERT INTO products (sku_code, name, category, brand, type, admin, commission, description, is_active)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        ON CONFLICT (sku_code) DO UPDATE SET
            name = EXCLUDED.name,
            category = EXCLUDED.category,
            brand = EXCLUDED.brand,
            type = EXCLUDED.type,
            admin = EXCLUDED.admin,
            commission = EXCLUDED.commission,
            description = EXCLUDED.description,
            is_active = EXCLUDED.is_active,
            updated_at = NOW()`

	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		product.SkuCode,
		product.Name,
		product.Category,
		product.Brand,
		product.Type,
		product.Admin,
		product.Commission,
		product.Description,
		product.IsActive,
	)
	return err
}

// UpdateStatus sets the active flag of a product.
func (r *ProductRepository) UpdateStatus(id int, isActive bool) error {
	const q = `UPDATE products SET is_active = $2, updated_at = NOW() WHERE id = $1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(id, isActive)
	return err
}

// Create creates a new product.
func (r *ProductRepository) Create(product *models.Product) error {
	query := `INSERT INTO products (sku_code, name, category, brand, type, admin, commission, description, is_active)
              VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
              RETURNING id, created_at, updated_at`

	return r.db.QueryRowx(query,
		product.SkuCode,
		product.Name,
		product.Category,
		product.Brand,
		product.Type,
		product.Admin,
		product.Commission,
		product.Description,
		product.IsActive,
	).Scan(&product.ID, &product.CreatedAt, &product.UpdatedAt)
}

// Update updates an existing product.
func (r *ProductRepository) Update(product *models.Product) error {
	query := `UPDATE products
              SET sku_code = $1, name = $2, category = $3, brand = $4,
                  type = $5, admin = $6, commission = $7, description = $8, is_active = $9
              WHERE id = $10
              RETURNING updated_at`

	return r.db.QueryRowx(query,
		product.SkuCode,
		product.Name,
		product.Category,
		product.Brand,
		product.Type,
		product.Admin,
		product.Commission,
		product.Description,
		product.IsActive,
		product.ID,
	).Scan(&product.UpdatedAt)
}

// Delete deletes a product by ID.
func (r *ProductRepository) Delete(id int) error {
	query := `DELETE FROM products WHERE id = $1`
	_, err := r.db.Exec(query, id)
	return err
}
