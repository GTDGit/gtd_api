-- Restore Digiflazz provider (re-enable)
UPDATE ppob_providers SET is_active = true WHERE code = 'digiflazz';

-- Note: Products are not restored. Run sync worker to re-populate from Digiflazz if needed.
