package models

import "time"

type BankCode struct {
	ID        int       `json:"id" db:"id"`
	Code      string    `json:"code" db:"code"`
	Name      string    `json:"name" db:"name"`
	SwiftCode *string   `json:"swift_code,omitempty" db:"swift_code"`
	SupportVA bool      `json:"support_va" db:"support_va"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
