package service

import (
	"context"
	"strings"
	"time"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/bnc"
)

// bankSNAPClient is the common surface implemented by both bnc.Client and
// bri.Client (BRI aliases the bnc request/response types). The bank payout
// adapter is written once against this interface and instantiated per provider.
type bankSNAPClient interface {
	ExternalAccountInquiry(ctx context.Context, bankCode, accountNo string) (*bnc.AccountInquiryResponse, error)
	InternalAccountInquiry(ctx context.Context, accountNo string) (*bnc.AccountInquiryResponse, error)
	InterbankTransfer(ctx context.Context, req bnc.TransferRequest) (*bnc.TransferResponse, error)
	IntrabankTransfer(ctx context.Context, req bnc.TransferRequest) (*bnc.TransferResponse, error)
	TransferStatus(ctx context.Context, req bnc.TransferStatusRequest) (*bnc.TransferStatusResponse, error)
}

// bankPayoutAdapter implements PayoutProviderClient for a direct-bank SNAP
// provider (BNC, BRI). It only serves BANK payouts. Intrabank vs interbank is
// decided by comparing the destination bank code to the provider's own bank.
type bankPayoutAdapter struct {
	provider       models.DisbursementProvider
	client         bankSNAPClient
	sourceBankCode string // provider's own bank code (intrabank destination)
	sourceAccount  string
}

// NewBankPayoutAdapter builds a bank adapter. A nil client or empty source
// account makes the adapter report itself unavailable.
func NewBankPayoutAdapter(provider models.DisbursementProvider, client bankSNAPClient, sourceBankCode, sourceAccount string) PayoutProviderClient {
	return &bankPayoutAdapter{
		provider:       provider,
		client:         client,
		sourceBankCode: strings.TrimSpace(sourceBankCode),
		sourceAccount:  strings.TrimSpace(sourceAccount),
	}
}

func (a *bankPayoutAdapter) Code() models.DisbursementProvider { return a.provider }

func (a *bankPayoutAdapter) Available() bool {
	return a != nil && a.client != nil && a.sourceAccount != ""
}

// Supports: direct-bank adapters serve BANK payouts to any bank code.
func (a *bankPayoutAdapter) Supports(mt models.MethodType, channelCode string) bool {
	return mt == models.MethodTypeBank && strings.TrimSpace(channelCode) != ""
}

func (a *bankPayoutAdapter) SourceAccount(_ models.MethodType) (string, string) {
	return a.sourceBankCode, a.sourceAccount
}

func (a *bankPayoutAdapter) transferType(bankCode string) models.TransferType {
	if strings.TrimSpace(bankCode) == a.sourceBankCode {
		return models.TransferTypeIntrabank
	}
	return models.TransferTypeInterbank
}

func (a *bankPayoutAdapter) Inquiry(ctx context.Context, in *PayoutInquiryInput) (*PayoutInquiryOutput, error) {
	tt := a.transferType(in.ChannelCode)
	var (
		resp *bnc.AccountInquiryResponse
		err  error
	)
	if tt == models.TransferTypeIntrabank {
		resp, err = a.client.InternalAccountInquiry(ctx, in.AccountNumber)
	} else {
		resp, err = a.client.ExternalAccountInquiry(ctx, in.ChannelCode, in.AccountNumber)
	}
	if err != nil {
		return nil, mapPayoutInquiryError(err)
	}
	return &PayoutInquiryOutput{
		AccountName:  strings.TrimSpace(resp.BeneficiaryAccountName),
		BankName:     strings.TrimSpace(resp.BeneficiaryBankName),
		ProviderRef:  strings.TrimSpace(resp.ReferenceNo),
		TransferType: tt,
		RawResponse:  resp.RawResponse,
	}, nil
}

func (a *bankPayoutAdapter) Pay(ctx context.Context, in *PayoutExecInput) (*PayoutExecOutput, error) {
	tt := in.TransferType
	if tt == "" {
		tt = a.transferType(in.ChannelCode)
	}
	req := bnc.TransferRequest{
		PartnerReferenceNo:     in.PartnerRef,
		Amount:                 in.Amount,
		BeneficiaryAccountNo:   in.AccountNumber,
		BeneficiaryBankCode:    in.ChannelCode,
		BeneficiaryAccountName: in.AccountName,
		Remark:                 in.Remark,
		PurposeCode:            in.Purpose,
		TransactionDate:        time.Now(),
	}
	var (
		resp *bnc.TransferResponse
		err  error
	)
	if tt == models.TransferTypeIntrabank {
		resp, err = a.client.IntrabankTransfer(ctx, req)
	} else {
		resp, err = a.client.InterbankTransfer(ctx, req)
	}
	if err != nil {
		return nil, err // raw error; service decides pending vs failed vs fallback
	}
	return &PayoutExecOutput{
		ProviderRef: strings.TrimSpace(resp.ReferenceNo),
		Status:      models.PayoutStatusProcessing,
		RawResponse: resp.RawResponse,
	}, nil
}

func (a *bankPayoutAdapter) Status(ctx context.Context, in *PayoutStatusInput) (*PayoutStatusOutput, error) {
	serviceCode := "18" // interbank
	if in.TransferType == models.TransferTypeIntrabank {
		serviceCode = "17"
	}
	resp, err := a.client.TransferStatus(ctx, bnc.TransferStatusRequest{
		OriginalPartnerReferenceNo: in.PartnerRef,
		ServiceCode:                serviceCode,
		TransactionDate:            time.Now(),
	})
	if err != nil {
		return nil, err
	}
	return &PayoutStatusOutput{
		Status:       bankStatusFromCode(resp.LatestTransactionStatus),
		ProviderRef:  strings.TrimSpace(resp.OriginalReferenceNo),
		FailedReason: strings.TrimSpace(resp.TransactionStatusDesc),
		FailedCode:   strings.TrimSpace(resp.LatestTransactionStatus),
		RawResponse:  resp.RawResponse,
	}, nil
}

// bankStatusFromCode maps SNAP latestTransactionStatus to a PayoutStatus.
func bankStatusFromCode(code string) models.PayoutStatus {
	switch strings.TrimSpace(code) {
	case "00":
		return models.PayoutStatusSuccess
	case "06", "05":
		return models.PayoutStatusFailed
	case "03", "02", "01":
		return models.PayoutStatusPending
	default:
		return models.PayoutStatusProcessing
	}
}
