BEGIN;

-- For postpaid products, the cost is admin fee, not price.
-- Migration 000026 incorrectly stored postpaid costs in the price field.
-- Move price → admin where product type is postpaid AND admin is currently 0.
UPDATE ppob_provider_skus ps
SET admin = ps.price, price = 0
FROM products p
WHERE ps.product_id = p.id
  AND p.type = 'postpaid'
  AND ps.price > 0
  AND ps.admin = 0;

-- Disable BRI provider (not ready yet)
UPDATE ppob_providers SET is_active = false WHERE code = 'bri';

-- Remove BRI provider SKU mappings
DELETE FROM ppob_provider_skus
WHERE provider_id = (SELECT id FROM ppob_providers WHERE code = 'bri');

COMMIT;
