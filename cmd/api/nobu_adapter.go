package main

import (
	"context"

	"github.com/GTDGit/gtd_api/pkg/nobu"
)

// nobuGeneratorAdapter adapts *nobu.Client to service.QRISStaticQRGenerator so
// the registration service can request a static QR string without importing the
// provider package directly.
type nobuGeneratorAdapter struct {
	client *nobu.Client
}

func (a *nobuGeneratorAdapter) GenerateStaticQR(ctx context.Context, partnerReferenceNo, subMerchantID, storeID, terminalID, merchantName string) (string, []byte, error) {
	resp, err := a.client.GenerateStaticQR(ctx, nobu.GenerateQRRequest{
		PartnerReferenceNo: partnerReferenceNo,
		MerchantID:         subMerchantID,
		StoreID:            storeID,
		TerminalID:         terminalID,
		MerchantName:       merchantName,
	})
	if err != nil {
		return "", nil, err
	}
	return resp.QRContent, resp.RawResponse, nil
}
