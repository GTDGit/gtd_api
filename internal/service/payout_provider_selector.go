package service

import (
	"context"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

// ----------------------------------------------------------------------------
// PayoutSelector encapsulates per-method_type provider routing and
// health/capability-based selection with bounded fallback. It mirrors the
// payment ProviderSelector but routes by method_type (BANK/EWALLET) instead of
// (type, code), and additionally filters by adapter capability (Supports) so a
// provider that cannot serve a given channel is skipped automatically.
// ----------------------------------------------------------------------------

type PayoutSelector struct {
	repo   *repository.PayoutRepository
	router *PayoutProviderRouter
}

func NewPayoutSelector(repo *repository.PayoutRepository, router *PayoutProviderRouter) *PayoutSelector {
	return &PayoutSelector{repo: repo, router: router}
}

// Candidates returns the priority-ordered adapters that are healthy and able to
// serve the given method_type + channel. It returns a 503 error when none
// qualify.
func (s *PayoutSelector) Candidates(ctx context.Context, mt models.MethodType, channelCode string) ([]PayoutProviderClient, error) {
	routes, err := s.repo.ListRoutesByMethodType(ctx, mt)
	if err != nil {
		return nil, err
	}

	out := make([]PayoutProviderClient, 0, len(routes))
	for i := range routes {
		r := &routes[i]
		if !r.IsActive || r.IsMaintenance {
			continue
		}
		adapter, err := s.router.Get(r.Provider)
		if err != nil || adapter == nil {
			continue
		}
		if !adapter.Available() {
			continue
		}
		if !adapter.Supports(mt, channelCode) {
			continue
		}
		out = append(out, adapter)
	}

	if len(out) == 0 {
		return nil, newPayoutError(503, "PAYOUT_METHOD_UNAVAILABLE",
			"No payout provider is available for "+string(mt)+"/"+channelCode, nil)
	}
	return out, nil
}
