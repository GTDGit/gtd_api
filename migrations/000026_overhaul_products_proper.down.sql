-- Rollback: restore Digiflazz, products need manual re-seed
UPDATE ppob_providers SET is_active = true WHERE code = 'digiflazz';

-- Note: Products are not restored. Run sync worker or re-apply previous migration.
