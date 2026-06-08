package service

import (
	"context"

	"github.com/GTDGit/gtd_api/internal/models"
)

// OVOProviderClient implements PaymentProviderClient for OVO Direct.
// Placeholder — full implementation pending OVO partner credential setup.
// Full docs: https://www.ovo.id/partner-integration/payment-api/tech-doc
type OVOProviderClient struct{}

func NewOVOProviderClient() *OVOProviderClient {
	return &OVOProviderClient{}
}

func (p *OVOProviderClient) Code() models.PaymentProvider {
	return models.ProviderOVODirect
}

func (p *OVOProviderClient) CreatePayment(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	return nil, newPaymentError(501, "NOT_IMPLEMENTED", "OVO Direct integration not yet available", nil)
}

func (p *OVOProviderClient) InquiryPayment(ctx context.Context, payment *models.Payment) (*PaymentInquiryResult, error) {
	return nil, newPaymentError(501, "NOT_IMPLEMENTED", "OVO Direct integration not yet available", nil)
}

func (p *OVOProviderClient) CancelPayment(ctx context.Context, payment *models.Payment, reason string) (*PaymentCancelResult, error) {
	return &PaymentCancelResult{Cancelled: true}, nil
}
