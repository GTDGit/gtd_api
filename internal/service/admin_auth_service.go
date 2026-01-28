package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/utils"
)

type AdminAuthService struct {
	adminRepo *repository.AdminUserRepository
}

func NewAdminAuthService(adminRepo *repository.AdminUserRepository) *AdminAuthService {
	return &AdminAuthService{adminRepo: adminRepo}
}

func (s *AdminAuthService) Login(email, password string) (string, error) {
	user, err := s.adminRepo.GetByEmail(email)
	if err != nil {
		return "", errors.New("invalid credentials")
	}

	if !user.IsActive {
		return "", errors.New("account is inactive")
	}

	// Simple SHA256 hash check (for now, replace with bcrypt later)
	hashedPassword := hashPassword(password)
	if user.PasswordHash != hashedPassword {
		return "", errors.New("invalid credentials")
	}

	token, err := utils.GenerateJWT(user.ID, user.Email)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s *AdminAuthService) CreateAdmin(email, password, name string) error {
	user := &models.AdminUser{
		Email:        email,
		PasswordHash: hashPassword(password),
		Name:         name,
		IsActive:     true,
	}

	return s.adminRepo.Create(user)
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}
