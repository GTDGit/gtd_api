-- ============================================
-- Migration 000018: Add commission column to ppob_provider_skus
-- Commission is what the provider gives us (reduces effective admin fee)
-- Effective admin = admin - commission
-- Example: admin 5000, commission 3500 → effective admin = 1500
-- ============================================

ALTER TABLE ppob_provider_skus
ADD COLUMN commission INT NOT NULL DEFAULT 0;

-- Add comment for clarity
COMMENT ON COLUMN ppob_provider_skus.commission IS 'Commission from provider (reduces effective admin fee)';

-- Create index for postpaid provider selection (sort by effective admin)
CREATE INDEX idx_ppob_provider_skus_effective_admin ON ppob_provider_skus((admin - commission));
