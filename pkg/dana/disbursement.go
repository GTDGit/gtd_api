package dana

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// DANA Disbursement (SNAP) endpoints. All use the symmetric (access-token)
// signing scheme via doSNAPRequest.
//
// Bank disbursement:
//   - Service 42: Transfer to Bank Account Inquiry  /v1.0/emoney/bank-account-inquiry.htm
//   - Service 43: Transfer to Bank                   /v1.0/emoney/transfer-bank.htm
//   - Service 00: Transfer to Bank Inquiry Status    /v1.0/emoney/transfer-bank-status.htm
//   - Service 43: Transfer to Bank Notify (inbound)  /v1.0/debit/emoney/transfer-bank/notify.htm
//
// E-wallet (DANA balance) disbursement:
//   - Service 37: Account Inquiry                    /rest/v1.0/emoney/account-inquiry
//   - Service 38: Customer Top Up                    /rest/v1.0/emoney/topup
//   - Service 39: Customer Top Up Inquiry Status     /rest/v1.0/emoney/topup-status
const (
	BankAccountInquiryPath = "/v1.0/emoney/bank-account-inquiry.htm"
	TransferBankPath       = "/v1.0/emoney/transfer-bank.htm"
	TransferBankStatusPath = "/v1.0/emoney/transfer-bank-status.htm"

	EmoneyAccountInquiryPath = "/rest/v1.0/emoney/account-inquiry"
	EmoneyTopUpPath          = "/rest/v1.0/emoney/topup"
	EmoneyTopUpStatusPath    = "/rest/v1.0/emoney/topup-status"

	DisbServiceCodeAccountInquiry = "37"
	DisbServiceCodeTopUp          = "38"
	DisbServiceCodeTopUpStatus    = "39"
	DisbServiceCodeBankInquiry    = "42"
	DisbServiceCodeTransferBank   = "43"
	DisbServiceCodeBankStatus     = "00"

	FundTypeUserSettle       = "AGENT_TOPUP_FOR_USER_SETTLE"
	FundTypeMerchantWithdraw = "MERCHANT_WITHDRAW_FOR_CORPORATE"

	// Disbursement transaction status codes (latestTransactionStatus).
	DisbStatusSuccess   = "00"
	DisbStatusInitiated = "01"
	DisbStatusPaying    = "02"
	DisbStatusPending   = "03"
	DisbStatusRefunded  = "04"
	DisbStatusCancelled = "05"
	DisbStatusFailed    = "06"
	DisbStatusNotFound  = "07"
)

// ---------------------------------------------------------------------------
// Bank disbursement: Service 42 (account inquiry)
// ---------------------------------------------------------------------------

// BankAccountInquiryRequest is the input for DANA Service 42.
type BankAccountInquiryRequest struct {
	PartnerReferenceNo       string
	CustomerNumber           string // merchant business phone (628xxx)
	BeneficiaryAccountNumber string
	BeneficiaryBankCode      string
	Amount                   int64
}

type BankAccountInquiryResponse struct {
	ResponseCode             string          `json:"responseCode"`
	ResponseMessage          string          `json:"responseMessage"`
	ReferenceNo              string          `json:"referenceNo"`
	PartnerReferenceNo       string          `json:"partnerReferenceNo"`
	AccountType              string          `json:"accountType,omitempty"`
	BeneficiaryAccountNumber string          `json:"beneficiaryAccountNumber"`
	BeneficiaryAccountName   string          `json:"beneficiaryAccountName"`
	BeneficiaryBankCode      string          `json:"beneficiaryBankCode,omitempty"`
	BeneficiaryBankShortName string          `json:"beneficiaryBankShortName,omitempty"`
	BeneficiaryBankName      string          `json:"beneficiaryBankName,omitempty"`
	Amount                   Amount          `json:"amount,omitempty"`
	AdditionalInfo           map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse              json.RawMessage `json:"-"`
}

// FeeAmount extracts additionalInfo.feeAmount.value as int64 (0 if absent).
func (r *BankAccountInquiryResponse) FeeAmount() int64 {
	if r == nil || r.AdditionalInfo == nil {
		return 0
	}
	fee, ok := r.AdditionalInfo["feeAmount"].(map[string]any)
	if !ok {
		return 0
	}
	v, _ := fee["value"].(string)
	n, _ := ParseWebhookAmount(Amount{Value: v})
	return n
}

// BankAccountInquiry calls Service 42 to validate a bank account and fetch the
// account holder name before a bank transfer.
func (c *Client) BankAccountInquiry(ctx context.Context, req BankAccountInquiryRequest) (*BankAccountInquiryResponse, error) {
	body := map[string]any{
		"partnerReferenceNo":       req.PartnerReferenceNo,
		"customerNumber":           req.CustomerNumber,
		"beneficiaryAccountNumber": req.BeneficiaryAccountNumber,
		"amount": Amount{
			Value:    formatAmount(req.Amount),
			Currency: "IDR",
		},
		"additionalInfo": map[string]any{
			"fundType":            FundTypeMerchantWithdraw,
			"beneficiaryBankCode": req.BeneficiaryBankCode,
		},
	}
	var resp BankAccountInquiryResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, BankAccountInquiryPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// ---------------------------------------------------------------------------
// Bank disbursement: Service 43 (transfer to bank)
// ---------------------------------------------------------------------------

// TransferBankRequest is the input for DANA Service 43.
type TransferBankRequest struct {
	PartnerReferenceNo       string
	CustomerNumber           string // merchant business phone (628xxx), optional
	BeneficiaryAccountNumber string
	BeneficiaryBankCode      string
	BeneficiaryAccountName   string
	Amount                   int64
}

type TransferBankResponse struct {
	ResponseCode       string          `json:"responseCode"`
	ResponseMessage    string          `json:"responseMessage"`
	ReferenceNo        string          `json:"referenceNo"`
	PartnerReferenceNo string          `json:"partnerReferenceNo"`
	TransactionDate    string          `json:"transactionDate,omitempty"`
	ReferenceNumber    string          `json:"referenceNumber,omitempty"`
	AdditionalInfo     map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse        json.RawMessage `json:"-"`
}

// TransferBank calls Service 43 to disburse funds to a bank account. needNotify
// is set so DANA delivers an async result notification to our webhook.
func (c *Client) TransferBank(ctx context.Context, req TransferBankRequest) (*TransferBankResponse, error) {
	add := map[string]any{
		"fundType":   FundTypeMerchantWithdraw,
		"needNotify": "true",
	}
	if req.BeneficiaryAccountName != "" {
		add["beneficiaryAccountName"] = req.BeneficiaryAccountName
	}
	body := map[string]any{
		"partnerReferenceNo":       req.PartnerReferenceNo,
		"beneficiaryAccountNumber": req.BeneficiaryAccountNumber,
		"beneficiaryBankCode":      req.BeneficiaryBankCode,
		"amount": Amount{
			Value:    formatAmount(req.Amount),
			Currency: "IDR",
		},
		"additionalInfo": add,
	}
	if req.CustomerNumber != "" {
		body["customerNumber"] = req.CustomerNumber
	}
	var resp TransferBankResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, TransferBankPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// ---------------------------------------------------------------------------
// Bank disbursement: Service 00 (transfer to bank status)
// ---------------------------------------------------------------------------

type DisbStatusResponse struct {
	ResponseCode               string          `json:"responseCode"`
	ResponseMessage            string          `json:"responseMessage"`
	OriginalPartnerReferenceNo string          `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo        string          `json:"originalReferenceNo,omitempty"`
	OriginalExternalID         string          `json:"originalExternalId,omitempty"`
	ServiceCode                string          `json:"serviceCode"`
	Amount                     Amount          `json:"amount,omitempty"`
	LatestTransactionStatus    string          `json:"latestTransactionStatus"`
	TransactionStatusDesc      string          `json:"transactionStatusDesc,omitempty"`
	AdditionalInfo             map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse                json.RawMessage `json:"-"`
}

// TransferBankStatus calls Service 00 to query the latest bank transfer status.
func (c *Client) TransferBankStatus(ctx context.Context, originalPartnerReferenceNo string) (*DisbStatusResponse, error) {
	body := map[string]any{
		"originalPartnerReferenceNo": originalPartnerReferenceNo,
		"serviceCode":                DisbServiceCodeBankStatus,
	}
	var resp DisbStatusResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, TransferBankStatusPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// ---------------------------------------------------------------------------
// E-wallet disbursement: Service 37 (account inquiry)
// ---------------------------------------------------------------------------

// EmoneyAccountInquiryRequest is the input for DANA Service 37.
type EmoneyAccountInquiryRequest struct {
	PartnerReferenceNo string
	CustomerNumber     string // recipient DANA number (628xxx)
	Amount             int64
}

type EmoneyAccountInquiryResponse struct {
	ResponseCode       string          `json:"responseCode"`
	ResponseMessage    string          `json:"responseMessage"`
	ReferenceNo        string          `json:"referenceNo"`
	PartnerReferenceNo string          `json:"partnerReferenceNo"`
	SessionID          string          `json:"sessionId,omitempty"`
	CustomerNumber     string          `json:"customerNumber"`
	CustomerName       string          `json:"customerName"`
	MinAmount          Amount          `json:"minAmount,omitempty"`
	MaxAmount          Amount          `json:"maxAmount,omitempty"`
	Amount             Amount          `json:"amount,omitempty"`
	FeeAmount          Amount          `json:"feeAmount,omitempty"`
	FeeType            string          `json:"feeType,omitempty"`
	AdditionalInfo     map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse        json.RawMessage `json:"-"`
}

// EmoneyAccountInquiry calls Service 37 to validate a DANA account and fetch the
// account holder name before a balance top-up.
func (c *Client) EmoneyAccountInquiry(ctx context.Context, req EmoneyAccountInquiryRequest) (*EmoneyAccountInquiryResponse, error) {
	body := map[string]any{
		"partnerReferenceNo": req.PartnerReferenceNo,
		"customerNumber":     req.CustomerNumber,
		"amount": Amount{
			Value:    formatAmount(req.Amount),
			Currency: "IDR",
		},
		"transactionDate": formatTimestamp(time.Now()),
		"additionalInfo": map[string]any{
			"fundType": FundTypeUserSettle,
		},
	}
	var resp EmoneyAccountInquiryResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, EmoneyAccountInquiryPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// ---------------------------------------------------------------------------
// E-wallet disbursement: Service 38 (customer top up)
// ---------------------------------------------------------------------------

// EmoneyTopUpRequest is the input for DANA Service 38.
type EmoneyTopUpRequest struct {
	PartnerReferenceNo string
	CustomerNumber     string // recipient DANA number (628xxx)
	Amount             int64
	FeeAmount          int64
	Notes              string
}

type EmoneyTopUpResponse struct {
	ResponseCode       string          `json:"responseCode"`
	ResponseMessage    string          `json:"responseMessage"`
	ReferenceNo        string          `json:"referenceNo"`
	PartnerReferenceNo string          `json:"partnerReferenceNo"`
	SessionID          string          `json:"sessionId,omitempty"`
	CustomerNumber     string          `json:"customerNumber,omitempty"`
	Amount             Amount          `json:"amount,omitempty"`
	AdditionalInfo     map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse        json.RawMessage `json:"-"`
}

// EmoneyTopUp calls Service 38 to top up a DANA balance (disbursement to balance).
func (c *Client) EmoneyTopUp(ctx context.Context, req EmoneyTopUpRequest) (*EmoneyTopUpResponse, error) {
	body := map[string]any{
		"partnerReferenceNo": req.PartnerReferenceNo,
		"customerNumber":     req.CustomerNumber,
		"amount": Amount{
			Value:    formatAmount(req.Amount),
			Currency: "IDR",
		},
		"feeAmount": Amount{
			Value:    formatAmount(req.FeeAmount),
			Currency: "IDR",
		},
		"transactionDate": formatTimestamp(time.Now()),
		"additionalInfo": map[string]any{
			"fundType": FundTypeUserSettle,
		},
	}
	if req.Notes != "" {
		body["notes"] = req.Notes
	}
	var resp EmoneyTopUpResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, EmoneyTopUpPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// EmoneyTopUpStatus calls Service 39 to query the latest top-up status.
func (c *Client) EmoneyTopUpStatus(ctx context.Context, originalPartnerReferenceNo string) (*DisbStatusResponse, error) {
	body := map[string]any{
		"originalPartnerReferenceNo": originalPartnerReferenceNo,
		"serviceCode":                DisbServiceCodeTopUp,
	}
	var resp DisbStatusResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, EmoneyTopUpStatusPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// ---------------------------------------------------------------------------
// Inbound notify (Service 43 Transfer to Bank Notify)
// ---------------------------------------------------------------------------

// DisbursementNotify is the inbound Transfer-to-Bank Notify payload.
type DisbursementNotify struct {
	OriginalPartnerReferenceNo string          `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo        string          `json:"originalReferenceNo"`
	LatestTransactionStatus    string          `json:"latestTransactionStatus"`
	TransactionStatusDesc      string          `json:"transactionStatusDesc,omitempty"`
	CreatedTime                string          `json:"createdTime,omitempty"`
	FinishedTime               string          `json:"finishedTime,omitempty"`
	Amount                     Amount          `json:"amount,omitempty"`
	AdditionalInfo             map[string]any  `json:"additionalInfo,omitempty"`
	RawBody                    json.RawMessage `json:"-"`
}

// ParseDisbursementNotify decodes a raw Transfer-to-Bank Notify body.
func ParseDisbursementNotify(body []byte) (*DisbursementNotify, error) {
	var n DisbursementNotify
	if err := json.Unmarshal(body, &n); err != nil {
		return nil, err
	}
	n.RawBody = json.RawMessage(body)
	n.OriginalPartnerReferenceNo = strings.TrimSpace(n.OriginalPartnerReferenceNo)
	return &n, nil
}

// DisbStatusTimestamp parses the X-TIMESTAMP header into time.Time.
func DisbStatusTimestamp(value string) time.Time {
	t, err := time.Parse("2006-01-02T15:04:05-07:00", value)
	if err != nil {
		return time.Time{}
	}
	return t
}
