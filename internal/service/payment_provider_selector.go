package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

// ----------------------------------------------------------------------------
// ProviderSelector encapsulates Method_Provider_Mapping lookup and
// health-based provider selection (see design "Provider Selection & Fallback").
//
// Resolve loads the canonical method group (display + ordered bindings).
// Select returns the lowest-priority healthy binding for the group.
// Next advances to the following healthy binding for bounded fallback.
// ----------------------------------------------------------------------------

// ProviderSelector reads provider bindings for a (type, code) group and picks
// the preferred healthy provider, honoring priority order and health flags.
type ProviderSelector struct {
	repo   *repository.PaymentRepository
	router *PaymentProviderRouter
}

// NewProviderSelector builds a ProviderSelector over the payment repository and
// the provider adapter router.
func NewProviderSelector(repo *repository.PaymentRepository, router *PaymentProviderRouter) *ProviderSelector {
	return &ProviderSelector{repo: repo, router: router}
}

// Resolve returns the MethodGroup for a (type, code): the canonical display
// data plus the provider bindings ordered by priority ASC. It returns a
// PAYMENT_METHOD_NOT_FOUND error when no canonical method exists for the pair.
func (s *ProviderSelector) Resolve(ctx context.Context, t models.PaymentType, code string) (*models.MethodGroup, error) {
	method, err := s.repo.GetMethodByTypeCode(ctx, t, code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYMENT_METHOD_NOT_FOUND",
				"Payment method not found for "+string(t)+"/"+code, nil)
		}
		return nil, err
	}

	bindings, err := s.repo.GetMethodProvidersByTypeCode(ctx, t, code)
	if err != nil {
		return nil, err
	}

	return &models.MethodGroup{
		Type:      method.Type,
		Code:      method.Code,
		Display:   methodDisplayFrom(method),
		Providers: bindings,
	}, nil
}

// Select returns the highest-priority (lowest priority value) binding that is
// active, not in maintenance, has a registered adapter, and whose adapter
// reports Available(). Bindings are already ordered by priority ASC from the
// repository. When none qualify it returns a PAYMENT_METHOD_UNAVAILABLE error.
func (s *ProviderSelector) Select(g *models.MethodGroup) (*models.MethodProviderBinding, error) {
	if g != nil {
		for i := range g.Providers {
			if s.healthy(&g.Providers[i]) {
				return &g.Providers[i], nil
			}
		}
	}
	return nil, newPaymentError(503, "METHOD_UNAVAILABLE",
		"Payment method is temporarily unavailable", nil)
}

// Next returns the next healthy binding after the given one, used for bounded
// fallback when creation against the chosen provider fails with a retryable
// error. It returns nil when no further healthy binding exists.
func (s *ProviderSelector) Next(g *models.MethodGroup, after *models.MethodProviderBinding) *models.MethodProviderBinding {
	if g == nil || after == nil {
		return nil
	}
	seen := false
	for i := range g.Providers {
		b := &g.Providers[i]
		if !seen {
			if b.ID == after.ID {
				seen = true
			}
			continue
		}
		if s.healthy(b) {
			return b
		}
	}
	return nil
}

// healthy reports whether a binding can currently serve requests: active in the
// mapping, not in maintenance, backed by a registered adapter, and the adapter
// reports itself available (credentials configured).
func (s *ProviderSelector) healthy(b *models.MethodProviderBinding) bool {
	if b == nil || !b.IsActive || b.IsMaintenance {
		return false
	}
	adapter, err := s.router.Get(b.Provider)
	if err != nil || adapter == nil {
		return false
	}
	return adapter.Available()
}

// methodDisplayFrom projects the canonical payment method onto the
// provider-agnostic display/limit/fee data carried by a MethodGroup.
func methodDisplayFrom(m *models.PaymentMethod) models.PaymentMethodDisplay {
	return models.PaymentMethodDisplay{
		Name:               m.Name,
		LogoURL:            m.LogoURL,
		FeeType:            m.FeeType,
		FeeFlat:            m.FeeFlat,
		FeePercent:         m.FeePercent,
		FeeMin:             m.FeeMin,
		FeeMax:             m.FeeMax,
		MinAmount:          m.MinAmount,
		MaxAmount:          m.MaxAmount,
		ExpiredDuration:    m.ExpiredDuration,
		DisplayOrder:       m.DisplayOrder,
		PaymentInstruction: m.PaymentInstruction,
	}
}
