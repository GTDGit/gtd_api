package service

import (
	"context"
	"strings"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/pkg/pakailink"
)

// pakailinkEwalletCodes are the e-wallet product codes Pakailink can top up.
// channelCode is normalized to these (e.g. SHOPEEPAY -> SHOPEE).
var pakailinkEwalletCodes = map[string]string{
	"DANA":      "DANA",
	"OVO":       "OVO",
	"GOPAY":     "GOPAY",
	"LINKAJA":   "LINKAJA",
	"SHOPEE":    "SHOPEE",
	"SHOPEEPAY": "SHOPEE",
}

// pakailinkPayoutAdapter implements PayoutProviderClient for Pakailink, serving
// both BANK transfers (Service 42/43/45) and EWALLET top-ups (Service 37/38/40).
type pakailinkPayoutAdapter struct {
	client      *pakailink.Client
	callbackURL string
}

// NewPakailinkPayoutAdapter builds a PakaiLink payout adapter (bank + e-wallet).
func NewPakailinkPayoutAdapter(client *pakailink.Client, callbackURL string) PayoutProviderClient {
	return &pakailinkPayoutAdapter{client: client, callbackURL: strings.TrimSpace(callbackURL)}
}

func (a *pakailinkPayoutAdapter) Code() models.DisbursementProvider {
	return models.DisbursementProviderPakaiLink
}

func (a *pakailinkPayoutAdapter) Available() bool { return a != nil && a.client != nil }

func (a *pakailinkPayoutAdapter) Supports(mt models.MethodType, channelCode string) bool {
	switch mt {
	case models.MethodTypeBank:
		return strings.TrimSpace(channelCode) != ""
	case models.MethodTypeEwallet:
		_, ok := pakailinkEwalletCodes[strings.ToUpper(strings.TrimSpace(channelCode))]
		return ok
	default:
		return false
	}
}

// SourceAccount: Pakailink does not expose a bank account; identify by label.
func (a *pakailinkPayoutAdapter) SourceAccount(_ models.MethodType) (string, string) {
	return "PAKAILINK", "PAKAILINK"
}

func (a *pakailinkPayoutAdapter) Inquiry(ctx context.Context, in *PayoutInquiryInput) (*PayoutInquiryOutput, error) {
	if in.MethodType == models.MethodTypeEwallet {
		resp, err := a.client.EmoneyAccountInquiry(ctx, pakailink.EmoneyAccountInquiryRequest{
			PartnerReferenceNo: in.PartnerRef,
			CustomerNumber:     in.AccountNumber,
			ProductCode:        pakailinkEwalletCodes[strings.ToUpper(strings.TrimSpace(in.ChannelCode))],
			Amount:             in.Amount,
		})
		if err != nil {
			return nil, mapPayoutInquiryError(err)
		}
		return &PayoutInquiryOutput{
			AccountName: strings.TrimSpace(resp.CustomerName),
			ProviderRef: strings.TrimSpace(resp.SessionID),
			RawResponse: resp.RawResponse,
		}, nil
	}

	resp, err := a.client.BankAccountInquiry(ctx, pakailink.BankAccountInquiryRequest{
		PartnerReferenceNo:   in.PartnerRef,
		BeneficiaryAccountNo: in.AccountNumber,
		BeneficiaryBankCode:  in.ChannelCode,
	})
	if err != nil {
		return nil, mapPayoutInquiryError(err)
	}
	return &PayoutInquiryOutput{
		AccountName:  strings.TrimSpace(resp.BeneficiaryAccountName),
		BankName:     strings.TrimSpace(resp.BeneficiaryBankName),
		ProviderRef:  strings.TrimSpace(resp.SessionID),
		TransferType: models.TransferTypeInterbank,
		RawResponse:  resp.RawResponse,
	}, nil
}

func (a *pakailinkPayoutAdapter) Pay(ctx context.Context, in *PayoutExecInput) (*PayoutExecOutput, error) {
	if in.MethodType == models.MethodTypeEwallet {
		resp, err := a.client.EmoneyTopUp(ctx, pakailink.EmoneyTopUpRequest{
			PartnerReferenceNo: in.PartnerRef,
			CustomerNumber:     in.AccountNumber,
			ProductCode:        pakailinkEwalletCodes[strings.ToUpper(strings.TrimSpace(in.ChannelCode))],
			Amount:             in.Amount,
			CallbackURL:        a.callbackURL,
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

	resp, err := a.client.TransferBank(ctx, pakailink.TransferBankRequest{
		PartnerReferenceNo:       in.PartnerRef,
		BeneficiaryAccountNumber: in.AccountNumber,
		BeneficiaryBankCode:      in.ChannelCode,
		Amount:                   in.Amount,
		CallbackURL:              a.callbackURL,
		Remark:                   in.Remark,
	})
	if err != nil {
		return nil, err
	}
	return &PayoutExecOutput{
		ProviderRef: strings.TrimSpace(resp.ReferenceNo),
		Status:      models.PayoutStatusProcessing,
		Fee:         PakailinkFeeFromResponse(resp.RawResponse),
		RawResponse: resp.RawResponse,
	}, nil
}

func (a *pakailinkPayoutAdapter) Status(ctx context.Context, in *PayoutStatusInput) (*PayoutStatusOutput, error) {
	if in.MethodType == models.MethodTypeEwallet {
		resp, err := a.client.EmoneyTopUpStatus(ctx, in.PartnerRef)
		if err != nil {
			return nil, err
		}
		return &PayoutStatusOutput{
			Status:       pakailinkStatusFromCode(resp.LatestTransactionStatus),
			ProviderRef:  strings.TrimSpace(resp.OriginalReferenceNo),
			FailedReason: strings.TrimSpace(resp.LatestTransactionStatusDesc.English),
			FailedCode:   strings.TrimSpace(resp.LatestTransactionStatus),
			RawResponse:  resp.RawResponse,
		}, nil
	}

	resp, err := a.client.TransferStatus(ctx, in.PartnerRef)
	if err != nil {
		return nil, err
	}
	return &PayoutStatusOutput{
		Status:       pakailinkStatusFromCode(resp.LatestTransactionStatus),
		ProviderRef:  strings.TrimSpace(resp.OriginalReferenceNo),
		FailedReason: strings.TrimSpace(resp.LatestTransactionStatusDesc.English),
		FailedCode:   strings.TrimSpace(resp.LatestTransactionStatus),
		RawResponse:  resp.RawResponse,
	}, nil
}

func pakailinkStatusFromCode(code string) models.PayoutStatus {
	switch strings.TrimSpace(code) {
	case pakailink.TransferStatusSuccess:
		return models.PayoutStatusSuccess
	case pakailink.TransferStatusFailed:
		return models.PayoutStatusFailed
	case pakailink.TransferStatusPending:
		return models.PayoutStatusPending
	default:
		return models.PayoutStatusProcessing
	}
}
