-- Rollback migration 042
DELETE FROM payment_methods WHERE type = 'QRIS' AND code IN ('MPM_PKL', 'MPM_MDT', 'MPM_XDT');

UPDATE payment_methods
SET name = 'QRIS'
WHERE type = 'QRIS' AND code = 'MPM' AND provider = 'dana_direct';
