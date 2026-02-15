package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/service"
)

// ProviderSyncWorker periodically syncs prices from all PPOB providers.
type ProviderSyncWorker struct {
	providerRepo    *repository.PPOBProviderRepository
	providerClients map[models.ProviderCode]service.PPOBProviderClient
	interval        time.Duration
}

// NewProviderSyncWorker constructs a ProviderSyncWorker.
func NewProviderSyncWorker(
	providerRepo *repository.PPOBProviderRepository,
	providerClients map[models.ProviderCode]service.PPOBProviderClient,
	interval time.Duration,
) *ProviderSyncWorker {
	return &ProviderSyncWorker{
		providerRepo:    providerRepo,
		providerClients: providerClients,
		interval:        interval,
	}
}

// Start begins the periodic sync loop and listens for context cancellation.
func (w *ProviderSyncWorker) Start(ctx context.Context) {
	log.Info().Dur("interval", w.interval).Msg("Starting provider price sync worker")

	// Run immediately on start
	w.run(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.run(ctx)
		case <-ctx.Done():
			log.Info().Msg("Provider sync worker stopped")
			return
		}
	}
}

func (w *ProviderSyncWorker) run(ctx context.Context) {
	log.Info().Msg("Starting provider price sync...")

	// Get all active providers
	providers, err := w.providerRepo.GetAllProviders(true)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get providers")
		return
	}

	for _, provider := range providers {
		client, ok := w.providerClients[provider.Code]
		if !ok {
			log.Warn().Str("provider", string(provider.Code)).Msg("Provider client not found")
			continue
		}

		w.syncProvider(ctx, provider, client)
	}

	log.Info().Msg("Provider price sync completed")
}

func (w *ProviderSyncWorker) syncProvider(ctx context.Context, provider models.PPOBProvider, client service.PPOBProviderClient) {
	log.Info().
		Str("provider", string(provider.Code)).
		Msg("Syncing prices from provider")

	start := time.Now()

	// Get all SKUs for this provider
	skus, err := w.providerRepo.GetProviderSKUsByProvider(provider.ID)
	if err != nil {
		log.Error().
			Err(err).
			Str("provider", string(provider.Code)).
			Msg("Failed to get provider SKUs")
		return
	}

	if len(skus) == 0 {
		log.Debug().
			Str("provider", string(provider.Code)).
			Msg("No SKUs configured for provider")
		return
	}

	// Get price list from provider
	priceList, err := client.GetPriceList(ctx, "")
	if err != nil {
		log.Error().
			Err(err).
			Str("provider", string(provider.Code)).
			Msg("Failed to get price list from provider")

		// Mark all SKUs with sync error
		for _, sku := range skus {
			_ = w.providerRepo.UpdateProviderSKUSyncError(sku.ID, err.Error())
		}
		return
	}

	// Create a map for quick lookup
	priceMap := make(map[string]service.ProviderProduct)
	for _, p := range priceList {
		priceMap[p.SKUCode] = p
	}

	// Update each SKU
	updated := 0
	unavailable := 0
	errors := 0

	for _, sku := range skus {
		product, found := priceMap[sku.ProviderSKUCode]
		if !found {
			// Product not found in provider's list - mark unavailable
			if err := w.providerRepo.UpdateProviderSKUPrice(sku.ID, sku.Price, sku.Admin, false); err != nil {
				errors++
				log.Error().
					Err(err).
					Int("sku_id", sku.ID).
					Msg("Failed to update SKU availability")
			} else {
				unavailable++
			}
			continue
		}

		// Update price and availability
		isAvailable := product.IsActive
		if err := w.providerRepo.UpdateProviderSKUPrice(sku.ID, product.Price, product.Admin, isAvailable); err != nil {
			errors++
			log.Error().
				Err(err).
				Int("sku_id", sku.ID).
				Msg("Failed to update SKU price")
		} else {
			updated++
		}
	}

	log.Info().
		Str("provider", string(provider.Code)).
		Int("updated", updated).
		Int("unavailable", unavailable).
		Int("errors", errors).
		Dur("duration", time.Since(start)).
		Msg("Provider sync completed")
}

// SyncSingleProvider syncs prices for a single provider (can be called on-demand)
func (w *ProviderSyncWorker) SyncSingleProvider(ctx context.Context, providerCode models.ProviderCode) error {
	provider, err := w.providerRepo.GetProviderByCode(providerCode)
	if err != nil {
		return err
	}

	client, ok := w.providerClients[providerCode]
	if !ok {
		return fmt.Errorf("provider client not found: %s", providerCode)
	}

	w.syncProvider(ctx, *provider, client)
	return nil
}
