-- Note: PostgreSQL does not support removing values from an enum type, so the
-- 'dana_direct' value added to disbursement_provider cannot be dropped here.
-- payout_method_type has no dependents once 000063 is rolled back, so it is
-- safe to drop.
DROP TYPE IF EXISTS payout_method_type;
