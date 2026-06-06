-- Migration 042: Add QRIS payment methods for all 4 providers
-- Providers: pakailink, midtrans, xendit, dana_direct
-- The existing MPM row (dana_direct) is kept as-is.
-- New rows use unique codes to distinguish providers.

-- QRIS via Pakailink
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, fee_percent, min_amount, max_amount, expired_duration, display_order, is_active, payment_instruction)
VALUES (
  'QRIS', 'MPM_PKL', 'QRIS (Pakailink)', 'pakailink',
  'percent', 0, 0.70, 1000, 10000000, 1800, 16, true,
  '{
    "steps": ["Buka aplikasi e-wallet atau mobile banking", "Pilih menu Scan QR / QRIS", "Scan QR code yang ditampilkan", "Konfirmasi pembayaran"],
    "supportedApps": ["GoPay", "OVO", "DANA", "LinkAja", "ShopeePay", "Mobile Banking"]
  }'::jsonb
)
ON CONFLICT (type, code) DO NOTHING;

-- QRIS via Midtrans (GoPay QR / QRIS MPM)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, fee_percent, min_amount, max_amount, expired_duration, display_order, is_active, payment_instruction)
VALUES (
  'QRIS', 'MPM_MDT', 'QRIS (Midtrans)', 'midtrans',
  'percent', 0, 0.70, 1000, 10000000, 1800, 17, true,
  '{
    "steps": ["Buka aplikasi e-wallet atau mobile banking", "Pilih menu Scan QR / QRIS", "Scan QR code yang ditampilkan", "Konfirmasi pembayaran"],
    "supportedApps": ["GoPay", "OVO", "DANA", "LinkAja", "ShopeePay", "Mobile Banking"]
  }'::jsonb
)
ON CONFLICT (type, code) DO NOTHING;

-- QRIS via Xendit
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, fee_percent, min_amount, max_amount, expired_duration, display_order, is_active, payment_instruction)
VALUES (
  'QRIS', 'MPM_XDT', 'QRIS (Xendit)', 'xendit',
  'percent', 0, 0.70, 1000, 10000000, 1800, 18, true,
  '{
    "steps": ["Buka aplikasi e-wallet atau mobile banking", "Pilih menu Scan QR / QRIS", "Scan QR code yang ditampilkan", "Konfirmasi pembayaran"],
    "supportedApps": ["GoPay", "OVO", "DANA", "LinkAja", "ShopeePay", "Mobile Banking"]
  }'::jsonb
)
ON CONFLICT (type, code) DO NOTHING;

-- Rename existing QRIS MPM (dana_direct) to be consistent with naming
UPDATE payment_methods
SET name = 'QRIS (DANA)'
WHERE type = 'QRIS' AND code = 'MPM' AND provider = 'dana_direct';
