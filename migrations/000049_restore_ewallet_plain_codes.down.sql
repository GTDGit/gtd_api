-- Rollback: delete the restored plain codes
DELETE FROM payment_methods WHERE type = 'EWALLET' AND code IN ('DANA','GOPAY','OVO','LINKAJA','SHOPEEPAY','ASTRAPAY');
