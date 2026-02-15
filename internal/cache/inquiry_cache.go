package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// InquiryData represents cached inquiry data.
type InquiryData struct {
	TransactionID string          `json:"transactionId"`
	ReferenceID   string          `json:"referenceId"`
	ClientID      int             `json:"clientId"`
	ProductID     int             `json:"productId"`
	CustomerNo    string          `json:"customerNo"`
	SKUCode       string          `json:"skuCode"`
	Amount        int             `json:"amount"`
	Admin         int             `json:"admin"`
	CustomerName  string          `json:"customerName,omitempty"`
	Description   json.RawMessage `json:"description,omitempty"`
	ExpiredAt     time.Time       `json:"expiredAt"`
	CachedAt      time.Time       `json:"cachedAt"`

	// Multi-provider fields: track which provider handled the inquiry
	// so payment uses the same provider
	ProviderCode    string `json:"providerCode,omitempty"`
	ProviderSKUCode string `json:"providerSkuCode,omitempty"`
	ProviderID      int    `json:"providerId,omitempty"`
	ProviderSKUID   int    `json:"providerSkuId,omitempty"`
}

// InquiryCache provides inquiry caching operations.
type InquiryCache struct {
	redis *RedisClient
}

// NewInquiryCache creates a new InquiryCache.
func NewInquiryCache(redis *RedisClient) *InquiryCache {
	return &InquiryCache{
		redis: redis,
	}
}

// calculateTTL calculates TTL until end of day in WIB timezone.
func (c *InquiryCache) calculateTTL() time.Duration {
	now := time.Now().In(time.FixedZone("WIB", 7*3600)) // UTC+7
	eod := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.FixedZone("WIB", 7*3600))
	return time.Until(eod)
}

// keyByTransactionID returns the primary Redis key for inquiry by transaction ID.
func (c *InquiryCache) keyByTransactionID(transactionID string) string {
	return fmt.Sprintf("inquiry:trx:%s", transactionID)
}

// keyCacheKey returns the secondary Redis key for caching duplicate inquiries.
func (c *InquiryCache) keyCacheKey(clientID int, customerNo, skuCode, referenceID string) string {
	return fmt.Sprintf("inquiry:cache:%d:%s:%s:%s", clientID, customerNo, skuCode, referenceID)
}

// Set stores inquiry data in Redis with double caching strategy.
// Primary key: inquiry:trx:{transactionId}
// Secondary key: inquiry:cache:{clientId}:{customerNo}:{skuCode}:{refId}
// TTL: Until end of day (23:59:59 WIB)
func (c *InquiryCache) Set(ctx context.Context, data *InquiryData) error {
	data.CachedAt = time.Now()

	// Calculate TTL until end of day
	ttl := c.calculateTTL()

	// Serialize data
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal inquiry data: %w", err)
	}

	// Store primary key (by transactionID)
	primaryKey := c.keyByTransactionID(data.TransactionID)
	if err := c.redis.Set(ctx, primaryKey, string(jsonData), ttl); err != nil {
		return fmt.Errorf("failed to set primary key: %w", err)
	}

	// Store secondary key (cache key) - points to transactionID
	cacheKey := c.keyCacheKey(data.ClientID, data.CustomerNo, data.SKUCode, data.ReferenceID)
	if err := c.redis.Set(ctx, cacheKey, data.TransactionID, ttl); err != nil {
		return fmt.Errorf("failed to set cache key: %w", err)
	}

	return nil
}

// GetByTransactionID retrieves inquiry data by transaction ID.
func (c *InquiryCache) GetByTransactionID(ctx context.Context, transactionID string) (*InquiryData, error) {
	key := c.keyByTransactionID(transactionID)
	jsonData, err := c.redis.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var data InquiryData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inquiry data: %w", err)
	}

	return &data, nil
}

// GetByCacheKey retrieves inquiry data by composite cache key (clientID, customerNo, skuCode, refId).
// Returns the full inquiry data if found.
func (c *InquiryCache) GetByCacheKey(ctx context.Context, clientID int, customerNo, skuCode, referenceID string) (*InquiryData, error) {
	cacheKey := c.keyCacheKey(clientID, customerNo, skuCode, referenceID)

	// Get transactionID from cache key
	transactionID, err := c.redis.Get(ctx, cacheKey)
	if err != nil {
		return nil, err
	}

	// Get full data using transactionID
	return c.GetByTransactionID(ctx, transactionID)
}

// Delete removes inquiry data from Redis (both primary and cache keys).
func (c *InquiryCache) Delete(ctx context.Context, data *InquiryData) error {
	primaryKey := c.keyByTransactionID(data.TransactionID)
	cacheKey := c.keyCacheKey(data.ClientID, data.CustomerNo, data.SKUCode, data.ReferenceID)

	return c.redis.Delete(ctx, primaryKey, cacheKey)
}
