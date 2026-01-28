package repository

import (
	"context"
	"database/sql"

	"github.com/GTDGit/gtd_api/internal/models"
)

type BankCodeRepository struct {
	db *sql.DB
}

func NewBankCodeRepository(db *sql.DB) *BankCodeRepository {
	return &BankCodeRepository{db: db}
}

func (r *BankCodeRepository) GetByCode(ctx context.Context, code string) (*models.BankCode, error) {
	query := `SELECT id, code, name, swift_code, support_va, is_active, created_at, updated_at 
	          FROM bank_codes WHERE code = $1 AND is_active = true`
	
	var bank models.BankCode
	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&bank.ID, &bank.Code, &bank.Name, &bank.SwiftCode, 
		&bank.SupportVA, &bank.IsActive, &bank.CreatedAt, &bank.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &bank, nil
}

func (r *BankCodeRepository) GetAll(ctx context.Context) ([]models.BankCode, error) {
	query := `SELECT id, code, name, swift_code, support_va, is_active, created_at, updated_at 
	          FROM bank_codes WHERE is_active = true ORDER BY code`
	
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var banks []models.BankCode
	for rows.Next() {
		var bank models.BankCode
		if err := rows.Scan(&bank.ID, &bank.Code, &bank.Name, &bank.SwiftCode, 
			&bank.SupportVA, &bank.IsActive, &bank.CreatedAt, &bank.UpdatedAt); err != nil {
			return nil, err
		}
		banks = append(banks, bank)
	}
	return banks, rows.Err()
}

func (r *BankCodeRepository) GetVABanks(ctx context.Context) ([]models.BankCode, error) {
	query := `SELECT id, code, name, swift_code, support_va, is_active, created_at, updated_at 
	          FROM bank_codes WHERE is_active = true AND support_va = true ORDER BY code`
	
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var banks []models.BankCode
	for rows.Next() {
		var bank models.BankCode
		if err := rows.Scan(&bank.ID, &bank.Code, &bank.Name, &bank.SwiftCode, 
			&bank.SupportVA, &bank.IsActive, &bank.CreatedAt, &bank.UpdatedAt); err != nil {
			return nil, err
		}
		banks = append(banks, bank)
	}
	return banks, rows.Err()
}
