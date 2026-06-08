-- Revert Method_Provider_Mapping table and its indexes
DROP INDEX IF EXISTS idx_pmp_provider;
DROP INDEX IF EXISTS idx_pmp_priority;
DROP INDEX IF EXISTS idx_pmp_method;
DROP TABLE IF EXISTS payment_method_providers;
