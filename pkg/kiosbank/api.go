package kiosbank

import (
	"context"
	"fmt"
)

// SignOn performs authentication and returns session ID
func (c *Client) SignOn(ctx context.Context) (*SignOnResponse, error) {
	req := SignOnRequest{
		MerchantID: c.config.MerchantID,
		CounterID:  c.config.CounterID,
		AccountID:  c.config.AccountID,
		Mitra:      c.config.Mitra,
	}

	var resp SignOnResponse
	if err := c.doRequest(ctx, "/Services/SignOn", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Inquiry checks customer bill information
func (c *Client) Inquiry(ctx context.Context, productID, customerID, referenceID, periode string) (*InquiryResponse, error) {
	sessionID, err := c.ensureSession(ctx)
	if err != nil {
		return nil, err
	}

	req := InquiryRequest{
		SessionID:   sessionID,
		MerchantID:  c.config.MerchantID,
		ProductID:   productID,
		CustomerID:  customerID,
		ReferenceID: formatReferenceID(referenceID),
		Periode:     periode,
	}

	var resp InquiryResponse
	if err := c.doRequest(ctx, "/Services/Inquiry", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Payment pays a postpaid bill
func (c *Client) Payment(ctx context.Context, productID, customerID, referenceID string, tagihan, admin, total int, noHandphone, nama, kode string) (*PaymentResponse, error) {
	sessionID, err := c.ensureSession(ctx)
	if err != nil {
		return nil, err
	}

	req := PaymentRequest{
		SessionID:   sessionID,
		MerchantID:  c.config.MerchantID,
		ProductID:   productID,
		CustomerID:  customerID,
		ReferenceID: formatReferenceID(referenceID),
		Tagihan:     formatAmount(tagihan),
		Admin:       formatAmount(admin),
		Total:       formatAmount(total),
		NoHandphone: noHandphone,
		Nama:        nama,
		Kode:        kode,
	}

	var resp PaymentResponse
	if err := c.doRequest(ctx, "/Services/Payment", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SinglePayment performs prepaid transaction (pulsa/data)
func (c *Client) SinglePayment(ctx context.Context, productID, customerID, referenceID string, tagihan, admin, total int) (*SinglePaymentResponse, error) {
	sessionID, err := c.ensureSession(ctx)
	if err != nil {
		return nil, err
	}

	req := SinglePaymentRequest{
		SessionID:   sessionID,
		MerchantID:  c.config.MerchantID,
		ProductID:   productID,
		CustomerID:  customerID,
		ReferenceID: formatReferenceID(referenceID),
		Tagihan:     formatAmount(tagihan),
		Admin:       formatAmount(admin),
		Total:       formatAmount(total),
	}

	var resp SinglePaymentResponse
	if err := c.doRequest(ctx, "/Services/SinglePayment", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CheckStatus checks transaction status
// tglTransaksi must be YYYY-MM-DD format (date of original payment)
func (c *Client) CheckStatus(ctx context.Context, productID, customerID, referenceID string, tagihan, admin, total int, tglTransaksi, noHandphone, nama, kode string) (*CheckStatusResponse, error) {
	sessionID, err := c.ensureSession(ctx)
	if err != nil {
		return nil, err
	}

	req := CheckStatusRequest{
		SessionID:    sessionID,
		MerchantID:   c.config.MerchantID,
		ProductID:    productID,
		CustomerID:   customerID,
		ReferenceID:  formatReferenceID(referenceID),
		Tagihan:      formatAmount(tagihan),
		Admin:        formatAmount(admin),
		Total:        formatAmount(total),
		NoHandphone:  noHandphone,
		Nama:         nama,
		Kode:         kode,
		TglTransaksi: tglTransaksi,
	}

	var resp CheckStatusResponse
	if err := c.doRequest(ctx, "/Services/Check-Status", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetPriceListPulsa gets prepaid (pulsa/data) price list
func (c *Client) GetPriceListPulsa(ctx context.Context) (*PriceListResponse, error) {
	sessionID, err := c.ensureSession(ctx)
	if err != nil {
		return nil, err
	}

	prefixes := []string{"11", "21", "31", "41", "51", "81"}
	merged := make([]PriceListItem, 0)
	seen := make(map[string]struct{})

	for _, prefixID := range prefixes {
		req := PriceListRequest{
			SessionID:  sessionID,
			MerchantID: c.config.MerchantID,
			PrefixID:   prefixID,
		}

		var resp PriceListResponse
		if err := c.doRequest(ctx, "/Services/getPulsa-Prabayar", req, &resp); err != nil {
			return nil, fmt.Errorf("prefix %s: %w", prefixID, err)
		}
		for _, item := range resp.Record {
			if _, ok := seen[item.Code]; ok {
				continue
			}
			seen[item.Code] = struct{}{}
			merged = append(merged, item)
		}
	}

	return &PriceListResponse{
		BaseResponse: BaseResponse{RC: RCSuccess},
		Record:       merged,
	}, nil
}

// GetPriceList gets general price list
func (c *Client) GetPriceList(ctx context.Context) (*PriceListResponse, error) {
	sessionID, err := c.ensureSession(ctx)
	if err != nil {
		return nil, err
	}

	req := PriceListRequest{
		SessionID:  sessionID,
		MerchantID: c.config.MerchantID,
	}

	var resp PriceListResponse
	if err := c.doRequest(ctx, "/Services/getDaftar-Harga", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
