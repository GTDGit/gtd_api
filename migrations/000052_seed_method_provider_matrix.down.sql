-- ============================================
-- Rollback 000052: remove the seeded Method_Provider_Mapping rows
-- ============================================
-- Deletes only the payment_method_providers rows seeded by this feature.
-- The canonical payment_methods rows are left intact because they are owned by
-- earlier migrations (000043-000049) and may be referenced by existing payments.

-- QRIS (MPM, CPM)
DELETE FROM payment_method_providers pmp
USING payment_methods pm
WHERE pmp.payment_method_id = pm.id
  AND pm.type = 'QRIS'
  AND pm.code IN ('MPM', 'CPM');

-- RETAIL (ALFAMART, INDOMARET)
DELETE FROM payment_method_providers pmp
USING payment_methods pm
WHERE pmp.payment_method_id = pm.id
  AND pm.type = 'RETAIL'
  AND pm.code IN ('ALFAMART', 'INDOMARET');

-- EWALLET (DANA, GOPAY, OVO, LINKAJA, SHOPEEPAY, ASTRAPAY)
DELETE FROM payment_method_providers pmp
USING payment_methods pm
WHERE pmp.payment_method_id = pm.id
  AND pm.type = 'EWALLET'
  AND pm.code IN ('DANA', 'GOPAY', 'OVO', 'LINKAJA', 'SHOPEEPAY', 'ASTRAPAY');

-- VA (all seeded bank codes)
DELETE FROM payment_method_providers pmp
USING payment_methods pm
WHERE pmp.payment_method_id = pm.id
  AND pm.type = 'VA'
  AND pm.code IN ('014','002','009','008','490','451','022','013','147','011','016','028','019','153','110','120','213');
