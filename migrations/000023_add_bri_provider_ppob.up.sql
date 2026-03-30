INSERT INTO ppob_providers (code, name, is_active, is_backup, priority, config)
VALUES ('bri', 'BRI', false, false, 3, '{"products": ["brizzi"], "notes": "activate after BRIZZI SKU mapping is configured"}')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    priority = EXCLUDED.priority,
    config = EXCLUDED.config,
    updated_at = NOW();
