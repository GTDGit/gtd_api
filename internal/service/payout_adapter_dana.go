package service

import (
	"context"
	"strings"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/dana"
)

// danaPayoutAdapter implements PayoutProviderClient for DANA Direct, serving
// BANK transfers (Service 42/43/00) and DANA-wallet top-ups (Service 37/38/39).
// DANA's e-wallet disbursement only targets the DANA wallet itself.
type danaPayoutAdapter struct {
	client        *dana.Client
	merchantPhone string // business phone used as customerNumber for bank ops
}

// NewDanaPayoutAdapter builds a DANA Direct payout adapter (bank + DANA wallet).
func NewDanaPayoutAdapter(client *dana.Client, merchantPhone string) PayoutProviderClient {
	return &danaPayoutAdapter{client: client, merchantPhone: strings.TrimSpace(merchantPhone)}
}

func (a *danaPayoutAdapter) Code() models.DisbursementProvider {
	return models.DisbursementProviderDANA
}

func (a *danaPayoutAdapter) Available() bool { return a != nil && a.client != nil }

func (a *danaPayoutAdapter) Supports(mt models.MethodType, channelCode string) bool {
	switch mt {
	case models.MethodTypeBank:
		return strings.TrimSpace(channelCode) != ""
	case models.MethodTypeEwallet:
		// DANA disbursement only tops up the DANA wallet.
		return strings.EqualFold(strings.TrimSpace(channelCode), "DANA")
	default:
		return false
	}
}

func (a *danaPayoutAdapter) SourceAccount(_ models.MethodType) (string, string) {
	return "DANA", "DANA"
}

func (a *danaPayoutAdapter) Inquiry(ctx context.Context, in *PayoutInquiryInput) (*PayoutInquiryOutput, error) {
	if in.MethodType == models.MethodTypeEwallet {
		resp, err := a.client.EmoneyAccountInquiry(ctx, dana.EmoneyAccountInquiryRequest{
			PartnerReferenceNo: in.PartnerRef,
			CustomerNumber:     in.AccountNumber,
			Amount:             in.Amount,
		})
		if err != nil {
			return nil, mapPayoutInquiryError(err)
		}
		return &PayoutInquiryOutput{
			AccountName: strings.TrimSpace(resp.CustomerName),
			ProviderRef: strings.TrimSpace(nonEmptyOrDefault(resp.SessionID, resp.ReferenceNo)),
			RawResponse: resp.RawResponse,
		}, nil
	}

	resp, err := a.client.BankAccountInquiry(ctx, dana.BankAccountInquiryRequest{
		PartnerReferenceNo:       in.PartnerRef,
		CustomerNumber:           a.merchantPhone,
		BeneficiaryAccountNumber: in.AccountNumber,
		BeneficiaryBankCode:      in.ChannelCode,
		Amount:                   in.Amount,
	})
	if err != nil {
		return nil, mapPayoutInquiryError(err)
	}
	return &PayoutInquiryOutput{
		AccountName:  strings.TrimSpace(resp.BeneficiaryAccountName),
		BankName:     strings.TrimSpace(resp.BeneficiaryBankName),
		ProviderRef:  strings.TrimSpace(resp.ReferenceNo),
		TransferType: models.TransferTypeInterbank,
		Fee:          resp.FeeAmount(),
		RawResponse:  resp.RawResponse,
	}, nil
}

func (a *danaPayoutAdapter) Pay(ctx context.Context, in *PayoutExecInput) (*PayoutExecOutput, error) {
	if in.MethodType == models.MethodTypeEwallet {
		resp, err := a.client.EmoneyTopUp(ctx, dana.EmoneyTopUpRequest{
			PartnerReferenceNo: in.PartnerRef,
			CustomerNumber:     in.AccountNumber,
			Amount:             in.Amount,
			Notes:              in.Remark,
		})
		if err != nil {
			return nil, err
		}
		return &PayoutExecOutput{
			ProviderRef: strings.TrimSpace(resp.ReferenceNo),
			Status:      models.PayoutStatusProcessing,
			RawResponse: resp.RawResponse,
		}, nil
	}

	resp, err := a.client.TransferBank(ctx, dana.TransferBankRequest{
		PartnerReferenceNo:       in.PartnerRef,
		CustomerNumber:           a.merchantPhone,
		BeneficiaryAccountNumber: in.AccountNumber,
		BeneficiaryBankCode:      in.ChannelCode,
		BeneficiaryAccountName:   in.AccountName,
		Amount:                   in.Amount,
	})
	if err != nil {
		return nil, err
	}
	return &PayoutExecOutput{
		ProviderRef: strings.TrimSpace(resp.ReferenceNo),
		Status:      models.PayoutStatusProcessing,
		RawResponse: resp.RawResponse,
	}, nil
}

func (a *danaPayoutAdapter) Status(ctx context.Context, in *PayoutStatusInput) (*PayoutStatusOutput, error) {
	if in.MethodType == models.MethodTypeEwallet {
		resp, err := a.client.EmoneyTopUpStatus(ctx, in.PartnerRef)
		if err != nil {
			return nil, err
		}
		return &PayoutStatusOutput{
			Status:       danaStatusFromCode(resp.LatestTransactionStatus),
			ProviderRef:  strings.TrimSpace(resp.OriginalReferenceNo),
			FailedReason: strings.TrimSpace(resp.TransactionStatusDesc),
			FailedCode:   strings.TrimSpace(resp.LatestTransactionStatus),
			RawResponse:  resp.RawResponse,
		}, nil
	}

	resp, err := a.client.TransferBankStatus(ctx, in.PartnerRef)
	if err != nil {
		return nil, err
	}
	return &PayoutStatusOutput{
		Status:       danaStatusFromCode(resp.LatestTransactionStatus),
		ProviderRef:  strings.TrimSpace(resp.OriginalReferenceNo),
		FailedReason: strings.TrimSpace(resp.TransactionStatusDesc),
		FailedCode:   strings.TrimSpace(resp.LatestTransactionStatus),
		RawResponse:  resp.RawResponse,
	}, nil
}

func danaStatusFromCode(code string) models.PayoutStatus {
	switch strings.TrimSpace(code) {
	case dana.DisbStatusSuccess:
		return models.PayoutStatusSuccess
	case dana.DisbStatusFailed, dana.DisbStatusCancelled, dana.DisbStatusRefunded:
		return models.PayoutStatusFailed
	case dana.DisbStatusPending, dana.DisbStatusInitiated, dana.DisbStatusPaying:
		return models.PayoutStatusPending
	default:
		return models.PayoutStatusProcessing
	}
}
