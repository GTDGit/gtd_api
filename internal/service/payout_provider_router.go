package service

import (
	"sync"

	"github.com/GTDGit/gtd_api/internal/models"
)

// PayoutProviderRouter dispatches to provider-specific payout adapters.
type PayoutProviderRouter struct {
	mu      sync.RWMutex
	clients map[models.DisbursementProvider]PayoutProviderClient
}

func NewPayoutProviderRouter() *PayoutProviderRouter {
	return &PayoutProviderRouter{clients: map[models.DisbursementProvider]PayoutProviderClient{}}
}

func (r *PayoutProviderRouter) Register(c PayoutProviderClient) {
	if c == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[c.Code()] = c
}

// Get returns the adapter for a provider or a 503 service error when none is
// registered.
func (r *PayoutProviderRouter) Get(code models.DisbursementProvider) (PayoutProviderClient, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clients[code]
	if !ok || c == nil {
		return nil, newPayoutError(503, "PROVIDER_UNAVAILABLE", "Payout provider is temporarily unavailable", nil)
	}
	return c, nil
}

// Has reports whether a provider adapter is registered.
func (r *PayoutProviderRouter) Has(code models.DisbursementProvider) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.clients[code]
	return ok
}

// Providers returns the list of registered provider codes.
func (r *PayoutProviderRouter) Providers() []models.DisbursementProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]models.DisbursementProvider, 0, len(r.clients))
	for k := range r.clients {
		out = append(out, k)
	}
	return out
}
