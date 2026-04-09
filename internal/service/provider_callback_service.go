package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/sse"
	"github.com/GTDGit/gtd_api/pkg/alterra"
	"github.com/GTDGit/gtd_api/pkg/kiosbank"
)

// ProviderCallbackService handles callbacks from PPOB providers
type ProviderCallbackService struct {
	providerRepo *repository.PPOBProviderRepository
	trxRepo      *repository.TransactionRepository
	callbackSvc  *CallbackService
	notifier     sse.TransactionNotifier
}

// NewProviderCallbackService creates a new ProviderCallbackService
func NewProviderCallbackService(
	providerRepo *repository.PPOBProviderRepository,
	trxRepo *repository.TransactionRepository,
	callbackSvc *CallbackService,
) *ProviderCallbackService {
	return &ProviderCallbackService{
		providerRepo: providerRepo,
		trxRepo:      trxRepo,
		callbackSvc:  callbackSvc,
	}
}

// SetNotifier sets the SSE notifier for real-time transaction updates
func (s *ProviderCallbackService) SetNotifier(notifier sse.TransactionNotifier) {
	s.notifier = notifier
}

// ProcessKiosbankCallback processes a callback from Kiosbank
func (s *ProviderCallbackService) ProcessKiosbankCallback(ctx context.Context, payload map[string]any) error {
	// Log the callback for audit
	rawPayload, _ := json.Marshal(payload)

	// Extract reference ID — Kiosbank uses "referenceID" (capital ID)
	refID, _ := payload["referenceID"].(string)
	if refID == "" {
		refID, _ = payload["referenceId"].(string)
	}
	if refID == "" {
		refID, _ = payload["ref_id"].(string)
	}
	if refID == "" {
		return fmt.Errorf("no reference ID in Kiosbank callback")
	}

	// Extract RC
	rc, _ := payload["rc"].(string)
	if rc == "" {
		rc, _ = payload["RC"].(string)
	}

	// Find transaction by provider ref ID
	trx, err := s.trxRepo.GetByProviderRefID(refID)
	if err != nil {
		// Try finding by transaction ID (ref_id might be our transaction_id)
		trx, err = s.trxRepo.GetByTransactionID(refID)
		if err != nil {
			log.Warn().Str("ref_id", refID).Msg("Transaction not found for Kiosbank callback")
			return fmt.Errorf("transaction not found: %s", refID)
		}
	}

	// Determine provider ID from transaction or lookup
	providerID := 0
	if trx.ProviderID != nil {
		providerID = *trx.ProviderID
	} else {
		// Lookup provider by code
		if p, err := s.providerRepo.GetProviderByCode(models.ProviderKiosbank); err == nil {
			providerID = p.ID
		}
	}

	// Determine status message
	var status, msg *string
	if kiosbank.IsSuccess(rc) {
		s := "success"
		status = &s
	} else if kiosbank.IsFatal(rc) {
		s := "failed"
		status = &s
		desc := kiosbank.GetRCDescription(rc)
		msg = &desc
	} else if kiosbank.IsPending(rc) {
		s := "pending"
		status = &s
	}

	// Store callback to audit log
	callback := &models.PPOBProviderCallback{
		ProviderID:    providerID,
		ProviderRefID: refID,
		TransactionID: trx.ID,
		Payload:       rawPayload,
		Status:        status,
		Message:       msg,
		IsProcessed:   false,
	}
	_ = s.providerRepo.CreateProviderCallback(callback)

	// Check if transaction is already in terminal state
	if trx.Status == models.StatusSuccess || trx.Status == models.StatusFailed {
		log.Debug().Str("transaction_id", trx.TransactionID).Str("status", string(trx.Status)).
			Msg("Kiosbank callback received for terminal transaction, ignoring")
		callback.IsProcessed = true
		_ = s.providerRepo.UpdateProviderCallbackProcessed(callback.ID, true)
		return nil
	}

	// Process based on RC
	now := time.Now()
	if kiosbank.IsSuccess(rc) {
		trx.Status = models.StatusSuccess
		// Extract serial number from data sub-object (product-specific keys)
		if data, ok := payload["data"].(map[string]any); ok {
			sn := extractKiosbankSN(data)
			if sn != "" {
				trx.SerialNumber = &sn
			}
			// Extract buy price from data (tagihan or harga)
			if bp := extractKiosbankBuyPrice(data); bp > 0 {
				trx.BuyPrice = &bp
			}
		}
		trx.ProcessedAt = &now
		if err := s.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("CRITICAL: failed to update transaction in DB from callback")
		}
		if s.notifier != nil {
			s.notifier.NotifyTransactionStatusChanged(trx)
		}
		go s.callbackSvc.SendCallback(trx, "transaction.success")
	} else if kiosbank.IsFatal(rc) {
		trx.Status = models.StatusFailed
		if msg, ok := payload["description"].(string); ok {
			trx.FailedReason = &msg
		}
		trx.FailedCode = &rc
		trx.ProcessedAt = &now
		if err := s.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("CRITICAL: failed to update transaction in DB from callback")
		}
		if s.notifier != nil {
			s.notifier.NotifyTransactionStatusChanged(trx)
		}
		go s.callbackSvc.SendCallback(trx, "transaction.failed")
	}
	// If pending, just wait for next callback

	callback.IsProcessed = true
	_ = s.providerRepo.UpdateProviderCallbackProcessed(callback.ID, true)

	return nil
}

// ProcessAlterraCallback processes a callback from Alterra
func (s *ProviderCallbackService) ProcessAlterraCallback(ctx context.Context, payload map[string]any) error {
	// Log the callback for audit
	rawPayload, _ := json.Marshal(payload)

	// Extract order ID (our reference)
	orderID, _ := payload["order_id"].(string)
	if orderID == "" {
		orderID, _ = payload["orderID"].(string)
		if orderID == "" {
			return fmt.Errorf("no order ID in Alterra callback")
		}
	}

	// Extract response code
	rc, _ := payload["response_code"].(string)
	if rc == "" {
		rc, _ = payload["responseCode"].(string)
	}

	// Find transaction
	trx, err := s.trxRepo.GetByProviderRefID(orderID)
	if err != nil {
		trx, err = s.trxRepo.GetByTransactionID(orderID)
		if err != nil {
			log.Warn().Str("order_id", orderID).Msg("Transaction not found for Alterra callback")
			return fmt.Errorf("transaction not found: %s", orderID)
		}
	}

	// Determine provider ID from transaction or lookup
	providerID := 0
	if trx.ProviderID != nil {
		providerID = *trx.ProviderID
	} else {
		if p, err := s.providerRepo.GetProviderByCode(models.ProviderAlterra); err == nil {
			providerID = p.ID
		}
	}

	// Determine status message
	var status, msg *string
	if alterra.IsSuccess(rc) {
		s := "success"
		status = &s
	} else if alterra.IsFatal(rc) {
		s := "failed"
		status = &s
		m, _ := payload["message"].(string)
		if m != "" {
			msg = &m
		}
	} else if alterra.IsPending(rc) {
		s := "pending"
		status = &s
	}

	// Store callback
	callback := &models.PPOBProviderCallback{
		ProviderID:    providerID,
		ProviderRefID: orderID,
		TransactionID: trx.ID,
		Payload:       rawPayload,
		Status:        status,
		Message:       msg,
		IsProcessed:   false,
	}
	_ = s.providerRepo.CreateProviderCallback(callback)

	// Check terminal state
	if trx.Status == models.StatusSuccess || trx.Status == models.StatusFailed {
		log.Debug().Str("transaction_id", trx.TransactionID).Str("status", string(trx.Status)).
			Msg("Alterra callback received for terminal transaction, ignoring")
		callback.IsProcessed = true
		_ = s.providerRepo.UpdateProviderCallbackProcessed(callback.ID, true)
		return nil
	}

	// Process based on RC
	now := time.Now()
	if alterra.IsSuccess(rc) {
		trx.Status = models.StatusSuccess
		if sn, ok := payload["serial_number"].(string); ok && sn != "" {
			trx.SerialNumber = &sn
		}
		// Set buy_price from callback
		if price, ok := payload["price"].(float64); ok && price > 0 {
			bp := int(price)
			trx.BuyPrice = &bp
		}
		trx.ProcessedAt = &now
		if err := s.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("CRITICAL: failed to update transaction in DB from callback")
		}
		if s.notifier != nil {
			s.notifier.NotifyTransactionStatusChanged(trx)
		}
		go s.callbackSvc.SendCallback(trx, "transaction.success")
	} else if alterra.IsFatal(rc) {
		trx.Status = models.StatusFailed
		if msg, ok := payload["message"].(string); ok {
			trx.FailedReason = &msg
		}
		trx.FailedCode = &rc
		trx.ProcessedAt = &now
		if err := s.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("CRITICAL: failed to update transaction in DB from callback")
		}
		if s.notifier != nil {
			s.notifier.NotifyTransactionStatusChanged(trx)
		}
		go s.callbackSvc.SendCallback(trx, "transaction.failed")
	}
	// If pending, wait for next callback

	callback.IsProcessed = true
	_ = s.providerRepo.UpdateProviderCallbackProcessed(callback.ID, true)

	return nil
}

// extractKiosbankSN extracts serial number from Kiosbank callback data object.
// Different products use different field names for the serial/token.
func extractKiosbankSN(data map[string]any) string {
	// PLN Token
	if tk, ok := data["TK"].(string); ok && tk != "" {
		return tk
	}
	// Generic SN
	if sn, ok := data["sn"].(string); ok && sn != "" {
		return sn
	}
	// Voucher code (streaming, game voucher)
	if kv, ok := data["kodeVoucher"].(string); ok && kv != "" {
		return kv
	}
	// Biller reference number
	if nr, ok := data["noReferensi"].(string); ok && nr != "" {
		return nr
	}
	return ""
}

// extractKiosbankBuyPrice extracts buy price from Kiosbank callback data.
func extractKiosbankBuyPrice(data map[string]any) int {
	// Try tagihan (postpaid)
	if v, ok := data["tagihan"].(string); ok && v != "" {
		return parseCallbackAmount(v)
	}
	// Try harga (prepaid/singlepayment)
	if v, ok := data["harga"].(string); ok && v != "" {
		return parseCallbackAmount(v)
	}
	// Try RS (PLN token - rupiah)
	if v, ok := data["RS"].(string); ok && v != "" {
		return parseCallbackAmount(v)
	}
	// Try as float64 (JSON number)
	if v, ok := data["tagihan"].(float64); ok && v > 0 {
		return int(v)
	}
	return 0
}

// parseCallbackAmount parses a Kiosbank amount string (may have leading zeros) to int.
func parseCallbackAmount(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return v
}

// ProcessGenericCallback processes a generic provider callback
func (s *ProviderCallbackService) ProcessGenericCallback(ctx context.Context, providerCode string, payload map[string]any) error {
	switch models.ProviderCode(providerCode) {
	case models.ProviderKiosbank:
		return s.ProcessKiosbankCallback(ctx, payload)
	case models.ProviderAlterra:
		return s.ProcessAlterraCallback(ctx, payload)
	default:
		log.Warn().Str("provider", providerCode).Msg("Unknown provider code in callback")
		return fmt.Errorf("unknown provider: %s", providerCode)
	}
}
