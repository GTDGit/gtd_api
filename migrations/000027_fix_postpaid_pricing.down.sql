BEGIN;

-- Reverse: move admin back to price for postpaid products
UPDATE ppob_provider_skus ps
SET price = ps.admin, admin = 0
FROM products p
WHERE ps.product_id = p.id
  AND p.type = 'postpaid'
  AND ps.admin > 0
  AND ps.price = 0;

-- Re-enable BRI provider
UPDATE ppob_providers SET is_active = true WHERE code = 'bri';

COMMIT;
