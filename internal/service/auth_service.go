package service

import (
    "database/sql"

    "github.com/GTDGit/gtd_api/internal/models"
    "github.com/GTDGit/gtd_api/internal/repository"
    "github.com/GTDGit/gtd_api/internal/utils"
)

// AuthService provides methods for authenticating and authorizing clients.
type AuthService struct {
    clientRepo *repository.ClientRepository
}

// NewAuthService constructs a new AuthService.
func NewAuthService(clientRepo *repository.ClientRepository) *AuthService {
    return &AuthService{clientRepo: clientRepo}
}

// ValidateAPIKey verifies the provided token against live and sandbox keys.
// Returns the client, a boolean indicating sandbox mode, or an error.
func (s *AuthService) ValidateAPIKey(token string) (*models.Client, bool, error) {
    if token == "" {
        return nil, false, utils.ErrInvalidToken
    }

    // Try live key first
    if c, err := s.clientRepo.GetByAPIKey(token); err == nil && c != nil {
        return c, false, nil
    } else if err != nil && err != sql.ErrNoRows {
        return nil, false, err
    }

    // Try sandbox key
    if c, err := s.clientRepo.GetBySandboxKey(token); err == nil && c != nil {
        return c, true, nil
    } else if err != nil && err != sql.ErrNoRows {
        return nil, false, err
    }

    return nil, false, utils.ErrInvalidToken
}

// ValidateClientID checks if the provided clientID matches the client's registered ID.
func (s *AuthService) ValidateClientID(client *models.Client, clientID string) bool {
    if client == nil {
        return false
    }
    return client.ClientID == clientID
}

// IsIPAllowed returns true if the provided IP is present in the client's whitelist.
func (s *AuthService) IsIPAllowed(client *models.Client, ip string) bool {
    if client == nil {
        return false
    }
    for _, allowed := range client.IPWhitelist {
        if allowed == ip {
            return true
        }
    }
    return false
}
