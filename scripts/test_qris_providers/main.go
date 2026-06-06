// test_qris_providers calls each QRIS provider directly and prints the QR string.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/GTDGit/gtd_api/pkg/dana"
	"github.com/GTDGit/gtd_api/pkg/midtrans"
	"github.com/GTDGit/gtd_api/pkg/pakailink"
	"github.com/GTDGit/gtd_api/pkg/xendit"
)

func main() {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	ctx := context.Background()

	fmt.Println("=== QRIS Provider Direct Test ===")
	fmt.Println()

	testMidtrans(ctx, ts)
	testXendit(ctx, ts)
	testPakailink(ctx, ts)
	testDana(ctx, ts)
}

// ---------- Midtrans (GoPay QRIS) ----------

func testMidtrans(ctx context.Context, ts string) {
	fmt.Println("--- [1] MIDTRANS (GoPay QRIS) ---")

	serverKey := os.Getenv("MIDTRANS_SERVER_KEY")
	baseURL := os.Getenv("MIDTRANS_BASE_URL")
	if serverKey == "" {
		fmt.Println("  SKIP: MIDTRANS_SERVER_KEY not set")
		return
	}
	if baseURL == "" {
		baseURL = "https://api.midtrans.com"
	}

	client, err := midtrans.NewClient(midtrans.Config{
		BaseURL:   baseURL,
		ServerKey: serverKey,
	})
	if err != nil {
		fmt.Printf("  INIT ERROR: %v\n\n", err)
		return
	}

	orderID := "QRIS-MDT-" + ts
	resp, err := client.ChargeQRIS(ctx, orderID, 10000, "gopay",
		&midtrans.CustomerDetails{FirstName: "John", Email: "john@example.com", Phone: "081234567890"},
	)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		if apiErr, ok := err.(*midtrans.APIError); ok {
			fmt.Printf("  RAW: %s\n\n", apiErr.RawResponse)
		}
		return
	}

	fmt.Printf("  ORDER ID      : %s\n", resp.OrderID)
	fmt.Printf("  TRANSACTION ID: %s\n", resp.TransactionID)
	fmt.Printf("  STATUS        : %s\n", resp.TransactionStatus)
	fmt.Printf("  QR STRING     : %s\n", resp.QRString)
	fmt.Printf("  EXPIRY        : %s\n", resp.ExpiryTime)
	qrURL := resp.Action("generate-qr-code")
	fmt.Printf("  QR CODE URL   : %s\n", qrURL)
	raw, _ := json.MarshalIndent(resp, "  ", "  ")
	fmt.Printf("  FULL RESPONSE:\n  %s\n\n", raw)
}

// ---------- Xendit (QRIS channel) ----------

func testXendit(ctx context.Context, ts string) {
	fmt.Println("--- [2] XENDIT (QRIS) ---")

	apiKey := os.Getenv("XENDIT_API_KEY")
	if apiKey == "" {
		fmt.Println("  SKIP: XENDIT_API_KEY not set")
		return
	}

	client, err := xendit.NewClient(xendit.Config{
		APIKey: apiKey,
	})
	if err != nil {
		fmt.Printf("  INIT ERROR: %v\n\n", err)
		return
	}

	refID := "QRIS-XDT-" + ts
	resp, err := client.CreatePaymentRequest(ctx, xendit.PaymentRequestCreate{
		ReferenceID:   refID,
		Type:          "PAY",
		Country:       "ID",
		Currency:      "IDR",
		ChannelCode:   "QRIS",
		RequestAmount: 10000,
		ChannelProperties: xendit.PaymentRequestChannelProperties{
			ExpiresAt: time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
		},
		Description: "Test QRIS Xendit",
	})
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		if apiErr, ok := err.(*xendit.APIError); ok {
			fmt.Printf("  RAW: %s\n\n", apiErr.RawResponse)
		}
		return
	}

	fmt.Printf("  PAYMENT REQUEST ID: %s\n", resp.PaymentRequestID)
	fmt.Printf("  REFERENCE ID      : %s\n", resp.ReferenceID)
	fmt.Printf("  STATUS            : %s\n", resp.Status)

	// QR string is in actions array (type=PRESENT_TO_CUSTOMER, descriptor=QR_STRING)
	qrString := ""
	if len(resp.Actions) > 0 {
		for _, a := range resp.Actions {
			if m, ok := a.(map[string]interface{}); ok {
				if m["descriptor"] == "QR_STRING" || m["type"] == "PRESENT_TO_CUSTOMER" {
					if v, ok := m["value"].(string); ok {
						qrString = v
					}
				}
			}
		}
	}
	// Also check channel_properties
	if qrString == "" {
		qrString = resp.ChannelProperties.QRString
	}

	fmt.Printf("  QR STRING         : %s\n", qrString)
	fmt.Printf("  QR IMAGE URL      : %s\n", resp.ChannelProperties.QRImageURL)
	raw, _ := json.MarshalIndent(resp, "  ", "  ")
	fmt.Printf("  FULL RESPONSE:\n  %s\n\n", raw)
}

// ---------- Pakailink (QRIS MPM) ----------

func testPakailink(ctx context.Context, ts string) {
	fmt.Println("--- [3] PAKAILINK (QRIS MPM) ---")

	baseURL := os.Getenv("PAKAILINK_BASE_URL")
	clientID := os.Getenv("PAKAILINK_CLIENT_ID")
	clientSecret := os.Getenv("PAKAILINK_CLIENT_SECRET")
	partnerID := os.Getenv("PAKAILINK_PARTNER_ID")
	privateKeyPath := os.Getenv("PAKAILINK_PRIVATE_KEY_PATH")

	if baseURL == "" {
		baseURL = "https://api.pakailink.id" // production
	}
	if clientID == "" {
		clientID = "ee7f8fc2564211f0a993fa163e117483"
	}
	if clientSecret == "" {
		clientSecret = "921988da7032bc8683c795dba81e4e84"
	}
	if partnerID == "" {
		partnerID = "PTR00000TI"
	}
	if privateKeyPath == "" {
		privateKeyPath = "keys/pakailink/private_key.pem"
	}

	client, err := pakailink.NewClient(pakailink.Config{
		BaseURL:        baseURL,
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		PartnerID:      partnerID,
		PrivateKeyPath: privateKeyPath,
	})
	if err != nil {
		fmt.Printf("  INIT ERROR: %v\n\n", err)
		return
	}

	refNo := "QRIS-PKL-" + ts
	expiry := time.Now().In(time.FixedZone("WIB", 7*3600)).Add(30 * time.Minute).Format("2006-01-02T15:04:05+07:00")
	resp, err := client.GenerateQRMPM(ctx, pakailink.GenerateQRRequest{
		PartnerReferenceNo: refNo,
		Amount:             10000,
		MerchantName:       "GTD Gateway",
		Description:        "Test QRIS Pakailink",
		CallbackURL:        "https://dev-api.gtd.co.id/v1/webhook/pakailink",
		ExpiredDate:        expiry,
	})
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		if apiErr, ok := err.(*pakailink.APIError); ok {
			fmt.Printf("  RAW: %s\n\n", apiErr.RawResponse)
		}
		return
	}

	qrString := resp.QRContent
	if qrString == "" {
		if v, ok := resp.AdditionalInfo["paymentQrString"].(string); ok {
			qrString = v
		}
	}

	fmt.Printf("  REFERENCE NO : %s\n", resp.ReferenceNo)
	fmt.Printf("  PARTNER REF  : %s\n", resp.PartnerReferenceNo)
	fmt.Printf("  QR CONTENT   : %s\n", qrString)
	fmt.Printf("  ADDITIONAL   : %v\n", resp.AdditionalInfo)
	raw, _ := json.MarshalIndent(resp, "  ", "  ")
	fmt.Printf("  FULL RESPONSE:\n  %s\n\n", raw)
}

// ---------- DANA (QRIS via NetworkPay) ----------

func testDana(ctx context.Context, ts string) {
	fmt.Println("--- [4] DANA (QRIS MPM) ---")

	baseURL := os.Getenv("DANA_BASE_URL")
	merchantID := os.Getenv("DANA_MERCHANT_ID")
	clientID := os.Getenv("DANA_CLIENT_ID")
	clientSecret := os.Getenv("DANA_CLIENT_SECRET")
	partnerID := os.Getenv("DANA_PARTNER_ID")
	privateKeyPath := os.Getenv("DANA_PRIVATE_KEY_PATH")

	if baseURL == "" {
		baseURL = "https://m.dana.id"
	}
	if merchantID == "" {
		merchantID = "216620080001022529053"
	}
	if clientID == "" {
		clientID = "2025080113571777143806"
	}
	if clientSecret == "" {
		fmt.Println("  SKIP: DANA_CLIENT_SECRET not set")
		return
	}
	if partnerID == "" {
		partnerID = clientID
	}

	client, err := dana.NewClient(dana.Config{
		BaseURL:        baseURL,
		MerchantID:     merchantID,
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		PartnerID:      partnerID,
		PrivateKeyPath: privateKeyPath,
	})
	if err != nil {
		fmt.Printf("  INIT ERROR: %v\n\n", err)
		return
	}

	refNo := "QRIS-DANA-" + ts
	expiry := time.Now().In(time.FixedZone("WIB", 7*3600)).Add(30 * time.Minute).Format("2006-01-02T15:04:05+07:00")
	resp, err := client.CreateOrder(ctx, dana.CreateOrderRequest{
		PartnerReferenceNo: refNo,
		Amount:             10000,
		ValidUpTo:          expiry,
		NotificationURL:    "https://dev-api.gtd.co.id/v1/webhook/dana",
		ReturnURL:          "https://dev-api.gtd.co.id",
		PayMethod:          dana.PayMethodNetworkPay,
		PayOption:          dana.PayOptionQRIS,
		OrderTitle:         "Test QRIS DANA",
	})
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		if apiErr, ok := err.(*dana.APIError); ok {
			fmt.Printf("  RAW: %s\n\n", apiErr.RawResponse)
		}
		return
	}

	qrString := resp.PaymentCode()
	fmt.Printf("  REFERENCE NO : %s\n", resp.ReferenceNo)
	fmt.Printf("  PARTNER REF  : %s\n", resp.PartnerReferenceNo)
	fmt.Printf("  QR STRING    : %s\n", qrString)
	fmt.Printf("  ADDITIONAL   : %v\n", resp.AdditionalInfo)
	raw, _ := json.MarshalIndent(resp, "  ", "  ")
	fmt.Printf("  FULL RESPONSE:\n  %s\n\n", raw)
}
