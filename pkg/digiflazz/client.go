package digiflazz

import (
    "bytes"
    "context"
    "crypto/md5"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

const (
    // BaseURL is the Digiflazz API base URL.
    BaseURL = "https://api.digiflazz.com/v1"
)

// Client is a minimal HTTP client for interacting with the Digiflazz API.
type Client struct {
    httpClient *http.Client
    username   string
    apiKey     string
}

// NewClient constructs a new Digiflazz client with sane defaults.
func NewClient(username, apiKey string) *Client {
    return &Client {
        httpClient: &http.Client{ Timeout: 30 * time.Second },
        username:   username,
        apiKey:     apiKey,
    }
}

// sign generates an MD5 hex digest signature per Digiflazz spec.
// sign = md5(username + apiKey + data)
func (c *Client) sign(data string) string {
    sum := md5.Sum([]byte(c.username + c.apiKey + data))
    return hex.EncodeToString(sum[:])
}

// Topup performs a prepaid transaction.
func (c *Client) Topup(ctx context.Context, skuCode, customerNo, refID string, testing bool) (*TransactionResponse, error) {
    req := TopupRequest{
        Username:     c.username,
        BuyerSkuCode: skuCode,
        CustomerNo:   customerNo,
        RefID:        refID,
        Sign:         c.sign(refID),
        Testing:      testing,
    }
    var resp TransactionResponse
    if err := c.doRequest(ctx, "/transaction", req, &resp); err != nil {
        return nil, err
    }
    return &resp, nil
}

// Inquiry checks a postpaid bill.
func (c *Client) Inquiry(ctx context.Context, skuCode, customerNo, refID string, testing bool) (*TransactionResponse, error) {
    req := InquiryRequest{
        Commands:     "inq-pasca",
        Username:     c.username,
        BuyerSkuCode: skuCode,
        CustomerNo:   customerNo,
        RefID:        refID,
        Sign:         c.sign(refID),
        Testing:      testing,
    }
    var resp TransactionResponse
    if err := c.doRequest(ctx, "/transaction", req, &resp); err != nil {
        return nil, err
    }
    return &resp, nil
}

// Payment pays a postpaid bill.
func (c *Client) Payment(ctx context.Context, skuCode, customerNo, refID string, testing bool) (*TransactionResponse, error) {
    req := PaymentRequest{
        Commands:     "pay-pasca",
        Username:     c.username,
        BuyerSkuCode: skuCode,
        CustomerNo:   customerNo,
        RefID:        refID,
        Sign:         c.sign(refID),
        Testing:      testing,
    }
    var resp TransactionResponse
    if err := c.doRequest(ctx, "/transaction", req, &resp); err != nil {
        return nil, err
    }
    return &resp, nil
}

// GetPricelist retrieves the list of products for the specified type ("prepaid" or "pasca").
func (c *Client) GetPricelist(ctx context.Context, productType string) (*PricelistResponse, error) {
    req := PricelistRequest{
        Cmd:      productType,
        Username: c.username,
        Sign:     c.sign("pricelist"),
    }
    var resp PricelistResponse
    if err := c.doRequest(ctx, "/price-list", req, &resp); err != nil {
        return nil, err
    }
    return &resp, nil
}

// GetBalance returns the current deposit balance.
func (c *Client) GetBalance(ctx context.Context) (*BalanceResponse, error) {
    req := BalanceRequest{
        Cmd:      "deposit",
        Username: c.username,
        Sign:     c.sign("depo"),
    }
    var resp BalanceResponse
    if err := c.doRequest(ctx, "/cek-saldo", req, &resp); err != nil {
        return nil, err
    }
    return &resp, nil
}

// doRequest performs the HTTP POST to the Digiflazz API with JSON payloads and
// decodes the JSON response into result.
func (c *Client) doRequest(ctx context.Context, endpoint string, body any, result any) error {
    payload, err := json.Marshal(body)
    if err != nil {
        return fmt.Errorf("failed to marshal request: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, http.MethodPost, BaseURL+endpoint, bytes.NewReader(payload))
    if err != nil {
        return fmt.Errorf("failed to create request: %w", err)
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()

    // Digiflazz often returns 200 with status encapsulated in JSON,
    // but decode regardless of status code to provide any error message.
    if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
        return fmt.Errorf("failed to decode response: %w", err)
    }
    return nil
}
