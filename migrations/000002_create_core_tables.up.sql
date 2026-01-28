-- ============================================
-- Migration 000002: Core Tables (clients, admin_users)
-- ============================================

CREATE TABLE clients (
    id SERIAL PRIMARY KEY,
    client_id VARCHAR(50) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    api_key VARCHAR(100) NOT NULL UNIQUE,
    sandbox_key VARCHAR(100) NOT NULL UNIQUE,
    callback_url VARCHAR(255) NOT NULL,
    callback_secret VARCHAR(100) NOT NULL,
    ip_whitelist TEXT[] NOT NULL DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_clients_client_id ON clients(client_id);
CREATE INDEX idx_clients_api_key ON clients(api_key);
CREATE INDEX idx_clients_sandbox_key ON clients(sandbox_key);

CREATE TABLE admin_users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(100) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(100) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'admin',
    is_active BOOLEAN NOT NULL DEFAULT true,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_admin_users_email ON admin_users(email);
