DROP INDEX IF EXISTS idx_clients_scopes;

ALTER TABLE clients
    DROP COLUMN IF EXISTS scopes;
