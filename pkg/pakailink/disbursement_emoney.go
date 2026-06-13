package pakailink

import (
	"context"
	"encoding/json"
	"net/http"
)

// Pakailink E-wallet disbursement (Customer Top Up) endpoints, SNAP symmetric.
//
//   - Service 37: Account Inquiry - Customer Top up E-wallet  /snap/v1.0/emoney/account-inquiry
//   - Service 38: Payment - Customer Top up E-Wallet          /snap/v1.0/emoney/topup
//   - Service 40: Inquiry Status - Customer Topup             /snap/v1.0/emoney/topup/status
//   - Service 39: Callback Pending - Customer Topup (inbound)
//
// NOTE: these share the same response/status conventions as the bank
// disbursement endpoints in disbursement.go (status codes 00/03/06).
const (
	EmoneyDisbAccountInquiryPath = "/snap/v1.0/emoney/account-inquiry"
	EmoneyDisbTopUpPath          = "/snap/v1.0/emoney/topup"
	EmoneyDisbTopUpStatusPath    = "/snap/v1.0/emoney/topup/status"

	EmoneyDisbServiceInquiry  = "37"
	EmoneyDisbServiceTopUp    = "38"
	EmoneyDisbServiceStatus   = "40"
	EmoneyDisbServiceCallback = "39"
)

// EmoneyAccountInquiryRequest is the input for Service 37 (e-wallet inquiry).
type EmoneyAccountInquiryRequest struct {
	PartnerReferenceNo string
	CustomerNumber     string // recipient e-wallet phone number
	ProductCode        string // DANA, OVO, GOPAY, LINKAJA, SHOPEE
	Amount             int64
}

type EmoneyAccountInquiryResponse struct {
	ResponseCode       string          `json:"responseCode"`
	ResponseMessage    string          `json:"responseMessage"`
	SessionID          string          `json:"sessionId"`
	PartnerReferenceNo string          `json:"partnerReferenceNo"`
	CustomerNumber     string          `json:"customerNumber"`
	CustomerName       string          `json:"customerName"`
	AdditionalInfo     map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse        json.RawMessage `json:"-"`
}

// EmoneyAccountInquiry calls Service 37 to validate an e-wallet account and
// fetch the holder name before a top-up.
func (c *Client) EmoneyAccountInquiry(ctx context.Context, req EmoneyAccountInquiryRequest) (*EmoneyAccountInquiryResponse, error) {
	body := map[string]any{
		"partnerReferenceNo": req.PartnerReferenceNo,
		"customerNumber":     req.CustomerNumber,
		"amount": Amount{
			Value:    formatAmount(req.Amount),
			Currency: "IDR",
		},
		"additionalInfo": map[string]any{
			"productCode": req.ProductCode,
		},
	}
	var resp EmoneyAccountInquiryResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, EmoneyDisbAccountInquiryPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// EmoneyTopUpRequest is the input for Service 38 (e-wallet top up).
type EmoneyTopUpRequest struct {
	PartnerReferenceNo string
	CustomerNumber     string
	ProductCode        string // DANA, OVO, GOPAY, LINKAJA, SHOPEE
	SessionID          string // from inquiry (optional)
	Amount             int64
	CallbackURL        string
}

type EmoneyTopUpResponse struct {
	ResponseCode       string          `json:"responseCode"`
	ResponseMessage    string          `json:"responseMessage"`
	ReferenceNo        string          `json:"referenceNo"`
	PartnerReferenceNo string          `json:"partnerReferenceNo"`
	CustomerNumber     string          `json:"customerNumber"`
	CustomerName       string          `json:"customerName"`
	Amount             Amount          `json:"amount"`
	FeeAmount          Amount          `json:"feeAmount"`
	AdditionalInfo     TransferAddInfo `json:"additionalInfo"`
	RawResponse        json.RawMessage `json:"-"`
}

// EmoneyTopUp calls Service 38 to top up an e-wallet balance (disbursement).
func (c *Client) EmoneyTopUp(ctx context.Context, req EmoneyTopUpRequest) (*EmoneyTopUpResponse, error) {
	add := map[string]any{}
	if req.CallbackURL != "" {
		add["callbackUrl"] = req.CallbackURL
	}
	body := map[string]any{
		"partnerReferenceNo": req.PartnerReferenceNo,
		"customerNumber":     req.CustomerNumber,
		"productCode":        req.ProductCode,
		"amount": Amount{
			Value:    formatAmount(req.Amount),
			Currency: "IDR",
		},
		"additionalInfo": add,
	}
	if req.SessionID != "" {
		body["sessionId"] = req.SessionID
	}
	var resp EmoneyTopUpResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, EmoneyDisbTopUpPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// EmoneyTopUpStatusResponse decodes the Service 40 status inquiry response.
type EmoneyTopUpStatusResponse struct {
	ResponseCode                string                `json:"responseCode"`
	ResponseMessage             string                `json:"responseMessage"`
	OriginalPartnerReferenceNo  string                `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo         string                `json:"originalReferenceNo"`
	OriginalExternalID          string                `json:"originalExternalId"`
	ServiceCode                 string                `json:"serviceCode"`
	TransactionDate             string                `json:"transactionDate"`
	Amount                      Amount                `json:"amount"`
	CustomerNumber              string                `json:"customerNumber"`
	CustomerName                string                `json:"customerName"`
	ProductType                 string                `json:"productType"`
	LatestTransactionStatus     string                `json:"latestTransactionStatus"`
	LatestTransactionStatusDesc TransactionStatusDesc `json:"latestTransactionStatusDesc"`
	AdditionalInfo              map[string]any        `json:"additionalInfo,omitempty"`
	RawResponse                 json.RawMessage       `json:"-"`
}

// EmoneyTopUpStatus calls Service 40 for the latest e-wallet top-up status.
func (c *Client) EmoneyTopUpStatus(ctx context.Context, originalPartnerReferenceNo string) (*EmoneyTopUpStatusResponse, error) {
	body := map[string]any{
		"originalPartnerReferenceNo": originalPartnerReferenceNo,
	}
	var resp EmoneyTopUpStatusResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, EmoneyDisbTopUpStatusPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}
