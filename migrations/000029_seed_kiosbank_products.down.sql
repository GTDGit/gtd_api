-- Rollback: remove Kiosbank provider SKUs
DELETE FROM ppob_provider_skus WHERE provider_id = (SELECT id FROM ppob_providers WHERE code = 'kiosbank');

-- Note: New KB- products are not removed (may have FK refs from transactions).
