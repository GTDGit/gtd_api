package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

// ----------------------------------------------------------------------------
// AdminPaymentService backs the admin payment-method + method-provider mapping
// endpoints. It wraps the PaymentRepository to list canonical methods (with
// their ordered provider bindings), edit canonical method fields, and manage
// the per-method provider bindings (priority, active, maintenance).
// ----------------------------------------------------------------------------

type AdminPaymentService struct {
	paymentRepo *repository.PaymentRepository
	router      *PaymentProviderRouter
}

func NewAdminPaymentService(paymentRepo *repository.PaymentRepository, router *PaymentProviderRouter) *AdminPaymentService {
	return &AdminPaymentService{paymentRepo: paymentRepo, router: router}
}

// ----------------------------------------------------------------------------
// Request/response DTOs
// ----------------------------------------------------------------------------

// AdminMethodView is a canonical payment method plus its ordered provider
// bindings (the Method_Provider_Mapping rows, priority ASC).
type AdminMethodView struct {
	models.PaymentMethod
	Providers []models.MethodProviderBinding `json:"providers"`
}

// AdminListMethodsResponse wraps the de-duplicated canonical method list.
type AdminListMethodsResponse struct {
	Methods []AdminMethodView `json:"methods"`
}

// AdminUpdateMethodRequest carries the editable canonical method fields. All
// fields are optional pointers; only provided fields are applied.
type AdminUpdateMethodRequest struct {
	Provider           *string         `json:"provider,omitempty"`
	FeeType            *string         `json:"feeType,omitempty"`
	FeeFlat            *int            `json:"feeFlat,omitempty"`
	FeePercent         *float64        `json:"feePercent,omitempty"`
	FeeMin             *int            `json:"feeMin,omitempty"`
	FeeMax             *int            `json:"feeMax,omitempty"`
	MinAmount          *int            `json:"minAmount,omitempty"`
	MaxAmount          *int            `json:"maxAmount,omitempty"`
	ExpiredDuration    *int            `json:"expiredDuration,omitempty"`
	LogoURL            *string         `json:"logoUrl,omitempty"`
	DisplayOrder       *int            `json:"displayOrder,omitempty"`
	PaymentInstruction json.RawMessage `json:"paymentInstruction,omitempty"`
	IsActive           *bool           `json:"isActive,omitempty"`
	IsMaintenance      *bool           `json:"isMaintenance,omitempty"`
	MaintenanceMessage *string         `json:"maintenanceMessage,omitempty"`
}

// AdminBindingUpdate is one ordered binding update in the providers PUT body.
// The provider identifies which binding row to update for the method.
type AdminBindingUpdate struct {
	Provider           string  `json:"provider"`
	Priority           int     `json:"priority"`
	IsActive           bool    `json:"isActive"`
	IsMaintenance      bool    `json:"isMaintenance"`
	MaintenanceMessage *string `json:"maintenanceMessage,omitempty"`
}

// AdminUpdateBindingsRequest is the body for PUT .../providers — the ordered
// set of bindings to apply for a method.
type AdminUpdateBindingsRequest struct {
	Providers []AdminBindingUpdate `json:"providers"`
}

// ----------------------------------------------------------------------------
// Canonical methods (de-duplicated by type+code) + provider bindings
// ----------------------------------------------------------------------------

// ListMethods returns the canonical payment methods de-duplicated by
// (type, code), each including its provider bindings ordered by priority ASC
// (Req 6.1, 6.18).
func (s *AdminPaymentService) ListMethods(ctx context.Context) (*AdminListMethodsResponse, error) {
	methods, err := s.paymentRepo.ListCanonicalMethods(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]AdminMethodView, 0, len(methods))
	for i := range methods {
		m := methods[i]
		bindings, err := s.paymentRepo.GetMethodProvidersByTypeCode(ctx, m.Type, m.Code)
		if err != nil {
			return nil, err
		}
		views = append(views, AdminMethodView{PaymentMethod: m, Providers: bindings})
	}
	return &AdminListMethodsResponse{Methods: views}, nil
}

// UpdateMethod applies the provided editable fields to the canonical method
// identified by id and returns the updated row (Req 3.3, 7.3).
func (s *AdminPaymentService) UpdateMethod(ctx context.Context, id int, req AdminUpdateMethodRequest) (*models.PaymentMethod, error) {
	m, err := s.paymentRepo.GetMethodByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYMENT_METHOD_NOT_FOUND", "Payment method not found", nil)
		}
		return nil, err
	}
	if req.Provider != nil {
		m.Provider = models.PaymentProvider(*req.Provider)
	}
	if req.FeeType != nil {
		m.FeeType = models.FeeType(*req.FeeType)
	}
	if req.FeeFlat != nil {
		m.FeeFlat = *req.FeeFlat
	}
	if req.FeePercent != nil {
		m.FeePercent = *req.FeePercent
	}
	if req.FeeMin != nil {
		m.FeeMin = *req.FeeMin
	}
	if req.FeeMax != nil {
		m.FeeMax = *req.FeeMax
	}
	if req.MinAmount != nil {
		m.MinAmount = *req.MinAmount
	}
	if req.MaxAmount != nil {
		m.MaxAmount = *req.MaxAmount
	}
	if req.ExpiredDuration != nil {
		m.ExpiredDuration = *req.ExpiredDuration
	}
	if req.LogoURL != nil {
		v := *req.LogoURL
		m.LogoURL = &v
	}
	if req.DisplayOrder != nil {
		m.DisplayOrder = *req.DisplayOrder
	}
	if len(req.PaymentInstruction) > 0 {
		if !json.Valid(req.PaymentInstruction) {
			return nil, newPaymentError(400, "INVALID_PAYMENT_INSTRUCTION", "paymentInstruction must be valid JSON", nil)
		}
		m.PaymentInstruction = models.NullableRawMessage(req.PaymentInstruction)
	}
	if req.IsActive != nil {
		m.IsActive = *req.IsActive
	}
	if req.IsMaintenance != nil {
		m.IsMaintenance = *req.IsMaintenance
	}
	if req.MaintenanceMessage != nil {
		v := *req.MaintenanceMessage
		m.MaintenanceMessage = &v
	}
	if err := s.paymentRepo.UpdateMethod(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// ListProviders returns the provider bindings for the method identified by
// (type, code), ordered by priority ASC (Req 6.1).
func (s *AdminPaymentService) ListProviders(ctx context.Context, paymentType, code string) ([]models.MethodProviderBinding, error) {
	t, c, err := normalizeMethodKey(paymentType, code)
	if err != nil {
		return nil, err
	}
	bindings, err := s.paymentRepo.GetMethodProvidersByTypeCode(ctx, t, c)
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		// Distinguish a missing method from a method with no bindings.
		if _, mErr := s.paymentRepo.GetMethodByTypeCode(ctx, t, c); mErr != nil {
			if errors.Is(mErr, sql.ErrNoRows) {
				return nil, newPaymentError(404, "PAYMENT_METHOD_NOT_FOUND", "Payment method not found", nil)
			}
			return nil, mErr
		}
	}
	return bindings, nil
}

// AvailableProviders returns providers that: (1) have adapter registered in router,
// (2) Available() == true, (3) not globally maintained. (Fix #8)
func (s *AdminPaymentService) AvailableProviders(ctx context.Context, paymentType, code string) ([]models.MethodProviderBinding, error) {
	t, c, err := normalizeMethodKey(paymentType, code)
	if err != nil {
		return nil, err
	}
	all, err := s.paymentRepo.GetMethodProvidersByTypeCode(ctx, t, c)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		if _, mErr := s.paymentRepo.GetMethodByTypeCode(ctx, t, c); mErr != nil {
			if errors.Is(mErr, sql.ErrNoRows) {
				return nil, newPaymentError(404, "PAYMENT_METHOD_NOT_FOUND", "Payment method not found", nil)
			}
			return nil, mErr
		}
	}
	if s.router == nil {
		return all, nil
	}
	out := make([]models.MethodProviderBinding, 0, len(all))
	for _, b := range all {
		if !b.IsActive || b.IsMaintenance {
			continue
		}
		if !s.router.Has(b.Provider) {
			continue
		}
		client, _ := s.router.Get(b.Provider)
		if client == nil || !client.Available() {
			continue
		}
		out = append(out, b)
	}
	return out, nil
}

// UpdateProviders applies the ordered binding updates (priority, is_active,
// is_maintenance, maintenance_message) for the method identified by
// (type, code) and returns the refreshed, priority-ordered bindings (Req 6.1).
func (s *AdminPaymentService) UpdateProviders(ctx context.Context, paymentType, code string, req AdminUpdateBindingsRequest) ([]models.MethodProviderBinding, error) {
	t, c, err := normalizeMethodKey(paymentType, code)
	if err != nil {
		return nil, err
	}
	existing, err := s.paymentRepo.GetMethodProvidersByTypeCode(ctx, t, c)
	if err != nil {
		return nil, err
	}
	if len(existing) == 0 {
		if _, mErr := s.paymentRepo.GetMethodByTypeCode(ctx, t, c); mErr != nil {
			if errors.Is(mErr, sql.ErrNoRows) {
				return nil, newPaymentError(404, "PAYMENT_METHOD_NOT_FOUND", "Payment method not found", nil)
			}
			return nil, mErr
		}
	}

	// Index existing bindings by provider for lookup.
	byProvider := make(map[models.PaymentProvider]*models.MethodProviderBinding, len(existing))
	for i := range existing {
		byProvider[existing[i].Provider] = &existing[i]
	}

	for _, u := range req.Providers {
		prov := models.PaymentProvider(strings.TrimSpace(u.Provider))
		binding, ok := byProvider[prov]
		if !ok {
			return nil, newPaymentError(400, "INVALID_PROVIDER_BINDING",
				"provider '"+u.Provider+"' is not bound to this payment method", nil)
		}
		binding.Priority = u.Priority
		binding.IsActive = u.IsActive
		binding.IsMaintenance = u.IsMaintenance
		if u.MaintenanceMessage != nil {
			v := *u.MaintenanceMessage
			binding.MaintenanceMessage = &v
		} else {
			binding.MaintenanceMessage = nil
		}
		if err := s.paymentRepo.UpdateMethodProviderBinding(ctx, binding); err != nil {
			return nil, err
		}
	}

	// Return the updated bindings ordered by priority ASC.
	sort.SliceStable(existing, func(i, j int) bool {
		if existing[i].Priority != existing[j].Priority {
			return existing[i].Priority < existing[j].Priority
		}
		return existing[i].ID < existing[j].ID
	})
	return existing, nil
}

// normalizeMethodKey upper-cases the type, trims the code, and validates the
// payment type against the known set.
func normalizeMethodKey(paymentType, code string) (models.PaymentType, string, error) {
	t := models.PaymentType(strings.ToUpper(strings.TrimSpace(paymentType)))
	c := strings.TrimSpace(code)
	switch t {
	case models.PaymentTypeVA, models.PaymentTypeEwallet, models.PaymentTypeQRIS, models.PaymentTypeRetail:
	default:
		return "", "", newPaymentError(400, "INVALID_PARAM", "Unknown payment method type: "+paymentType, nil)
	}
	if c == "" {
		return "", "", newPaymentError(400, "MISSING_FIELD", "payment method code is required", nil)
	}
	return t, c, nil
}
