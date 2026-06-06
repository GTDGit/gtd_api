// test_qris_vps — run from inside the VPS container to test providers that
// require VPS IP whitelisting (Pakailink). Also re-tests Midtrans & Xendit.
// Usage: go run ./scripts/test_qris_vps/ (with env vars set)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/GTDGit/gtd_api/pkg/midtrans"
	"github.com/GTDGit/gtd_api/pkg/pakailink"
	"github.com/GTDGit/gtd_api/pkg/xendit"
)

func main() {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	ctx := context.Background()

	fmt.Println("=== QRIS Provider Test (from VPS) ===")
	fmt.Println()

	testMidtrans(ctx, ts)
	testXendit(ctx, ts)
	testPakailink(ctx, ts)
}

func testMidtrans(ctx context.Context, ts string) {
	fmt.Println("--- [1] MIDTRANS ---")
	serverKey := os.Getenv("MIDTRANS_SERVER_KEY")
	if serverKey == "" {
		fmt.Println("  SKIP: MIDTRANS_SERVER_KEY not set\n")
		return
	}
	client, err := midtrans.NewClient(midtrans.Config{
		BaseURL:   "https://api.midtrans.com",
		ServerKey: serverKey,
	})
	if err != nil {
		fmt.Printf("  INIT ERROR: %v\n\n", err)
		return
	}
	resp, err := client.ChargeQRIS(ctx, "QRIS-MDT-"+ts, 10000, "gopay", &midtrans.CustomerDetails{
		FirstName: "John Doe", Email: "john@example.com", Phone: "081234567890",
	})
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		if e, ok := err.(*midtrans.APIError); ok {
			fmt.Printf("  RAW: %s\n\n", e.RawResponse)
		}
		return
	}
	fmt.Printf("  OK  order_id=%s txn_id=%s\n", resp.OrderID, resp.TransactionID)
	fmt.Printf("  QR_STRING: %s\n", resp.QRString)
	fmt.Printf("  EXPIRY   : %s\n\n", resp.ExpiryTime)
}

func testXendit(ctx context.Context, ts string) {
	fmt.Println("--- [2] XENDIT ---")
	apiKey := os.Getenv("XENDIT_API_KEY")
	if apiKey == "" {
		fmt.Println("  SKIP: XENDIT_API_KEY not set\n")
		return
	}
	client, err := xendit.NewClient(xendit.Config{APIKey: apiKey})
	if err != nil {
		fmt.Printf("  INIT ERROR: %v\n\n", err)
		return
	}
	resp, err := client.CreatePaymentRequest(ctx, xendit.PaymentRequestCreate{
		ReferenceID: "QRIS-XDT-" + ts, Type: "PAY", Country: "ID", Currency: "IDR",
		ChannelCode: "QRIS", RequestAmount: 10000,
		ChannelProperties: xendit.PaymentRequestChannelProperties{
			ExpiresAt: time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
		},
		Description: "Test QRIS Xendit",
	})
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		if e, ok := err.(*xendit.APIError); ok {
			fmt.Printf("  RAW: %s\n\n", e.RawResponse)
		}
		return
	}
	qr := resp.ChannelProperties.QRString
	if qr == "" {
		for _, a := range resp.Actions {
			if m, ok := a.(map[string]interface{}); ok {
				if v, ok := m["value"].(string); ok && v != "" {
					qr = v
					break
				}
			}
		}
	}
	fmt.Printf("  OK  payment_request_id=%s status=%s\n", resp.PaymentRequestID, resp.Status)
	fmt.Printf("  QR_STRING: %s\n\n", qr)
}

func testPakailink(ctx context.Context, ts string) {
	fmt.Println("--- [3] PAKAILINK ---")
	baseURL := os.Getenv("PAKAILINK_BASE_URL")
	clientID := os.Getenv("PAKAILINK_CLIENT_ID")
	clientSecret := os.Getenv("PAKAILINK_CLIENT_SECRET")
	partnerID := os.Getenv("PAKAILINK_PARTNER_ID")
	keyPath := os.Getenv("PAKAILINK_PRIVATE_KEY_PATH")

	if baseURL == "" {
		baseURL = "https://api.pakailink.id"
	}
	if clientID == "" || clientSecret == "" {
		fmt.Println("  SKIP: PAKAILINK_CLIENT_ID or PAKAILINK_CLIENT_SECRET not set\n")
		return
	}
	if keyPath == "" {
		keyPath = "keys/pakailink/private_key.pem"
	}

	client, err := pakailink.NewClient(pakailink.Config{
		BaseURL:        baseURL,
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		PartnerID:      partnerID,
		PrivateKeyPath: keyPath,
	})
	if err != nil {
		fmt.Printf("  INIT ERROR: %v\n\n", err)
		return
	}

	refNo := "QRIS-PKL-" + ts
	// Pakailink partnerReferenceNo max 25 chars for QRIS
	if len(refNo) > 25 {
		refNo = refNo[:25]
	}
	expiry := time.Now().In(time.FixedZone("WIB", 7*3600)).Add(30 * time.Minute).Format("2006-01-02T15:04:05+07:00")

	resp, err := client.GenerateQRMPM(ctx, pakailink.GenerateQRRequest{
		PartnerReferenceNo: refNo,
		Amount:             10000,
		MerchantName:       "GTD Gateway",
		Description:        "Test QRIS Pakailink",
		CallbackURL:        os.Getenv("PAKAILINK_CALLBACK_URL"),
		ExpiredDate:        expiry,
	})
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		if e, ok := err.(*pakailink.APIError); ok {
			fmt.Printf("  RAW: %s\n\n", e.RawResponse)
		}
		return
	}

	qr := resp.QRContent
	if qr == "" {
		if v, ok := resp.AdditionalInfo["paymentQrString"].(string); ok {
			qr = v
		}
	}
	fmt.Printf("  OK  referenceNo=%s partnerRef=%s\n", resp.ReferenceNo, resp.PartnerReferenceNo)
	fmt.Printf("  QR_STRING  : %s\n", qr)
	fmt.Printf("  ADDITIONAL : %v\n", resp.AdditionalInfo)
	raw, _ := json.MarshalIndent(resp, "  ", "  ")
	fmt.Printf("  FULL:\n  %s\n\n", raw)
}
