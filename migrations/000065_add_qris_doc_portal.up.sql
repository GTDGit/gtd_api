-- QRIS document portal: secure, token-gated delivery of merchant onboarding
-- documents (KTP, selfie with KTP, business-location photo, plus one extra
-- photo) to Nobu, whose QRIS registration is a manual process that asks for
-- files via shared drive. Instead of a shared Google Drive folder we expose a
-- private portal at files.qris.gtd.co.id: every file lives in private S3
-- (ap-southeast-3 / Jakarta, never public) and is only ever streamed through a
-- handler that validates an unguessable UUIDv4 token. There is no public URL at
-- any point in the lifecycle.
--
-- A "bundle" is one shareable link for one merchant. It owns N files. The
-- operator uploads via the gateway admin API, gets back the bundle link, and
-- sends it to Nobu. When Nobu finishes downloading they press "Konfirmasi",
-- which revokes the bundle (status=revoked): every subsequent access — bundle
-- or per-file — returns 403. Access (view/download/confirm/forbidden) is logged
-- for PDP (UU 27/2022) auditability.

CREATE TABLE IF NOT EXISTS qris_doc_bundles (
    id               BIGSERIAL PRIMARY KEY,
    token            UUID NOT NULL DEFAULT gen_random_uuid(), -- UUIDv4 link path segment
    merchant_name    VARCHAR(128) NOT NULL,
    qris_merchant_id BIGINT REFERENCES qris_merchants(id) ON DELETE SET NULL, -- optional link to registry
    status           VARCHAR(16) NOT NULL DEFAULT 'active',   -- active | revoked
    note             TEXT,                                    -- optional operator note shown in portal
    created_by       VARCHAR(128),                            -- admin email/id who uploaded
    confirmed_at     TIMESTAMPTZ,                             -- set when Nobu confirms download (revokes access)
    expires_at       TIMESTAMPTZ,                             -- optional hard expiry; NULL = no time limit
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Token is the only thing a caller presents; it must be unique and indexed for
-- the portal's per-request lookup.
CREATE UNIQUE INDEX IF NOT EXISTS uq_qris_doc_bundle_token
    ON qris_doc_bundles (token);

CREATE INDEX IF NOT EXISTS idx_qris_doc_bundle_merchant
    ON qris_doc_bundles (qris_merchant_id, created_at DESC);

-- Each file has its OWN token so a single image can be opened/downloaded
-- directly (the portal also lists all files of a bundle by the bundle token).
CREATE TABLE IF NOT EXISTS qris_doc_files (
    id           BIGSERIAL PRIMARY KEY,
    bundle_id    BIGINT NOT NULL REFERENCES qris_doc_bundles(id) ON DELETE CASCADE,
    token        UUID NOT NULL DEFAULT gen_random_uuid(),  -- UUIDv4 per-file access path
    doc_type     VARCHAR(32) NOT NULL,                     -- ktp | selfie_ktp | business_location | extra
    file_name    VARCHAR(255) NOT NULL,                    -- name provided in the upload request
    content_type VARCHAR(128) NOT NULL DEFAULT 'application/octet-stream',
    size_bytes   BIGINT NOT NULL DEFAULT 0,
    storage_key  TEXT NOT NULL,                            -- private S3 object key (never public)
    checksum     VARCHAR(64),                              -- optional sha256 hex of bytes
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_qris_doc_file_token
    ON qris_doc_files (token);

CREATE INDEX IF NOT EXISTS idx_qris_doc_file_bundle
    ON qris_doc_files (bundle_id, created_at);

-- PDP audit trail: who looked at / downloaded / confirmed what, and when.
-- forbidden rows capture rejected access attempts (revoked/expired/bad token).
CREATE TABLE IF NOT EXISTS qris_doc_access_logs (
    id         BIGSERIAL PRIMARY KEY,
    bundle_id  BIGINT REFERENCES qris_doc_bundles(id) ON DELETE SET NULL,
    file_id    BIGINT REFERENCES qris_doc_files(id) ON DELETE SET NULL,
    action     VARCHAR(16) NOT NULL,                       -- view | download | confirm | forbidden
    ip         VARCHAR(64),
    user_agent TEXT,
    detail     VARCHAR(255),                               -- optional reason (e.g. "revoked", "expired")
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_qris_doc_access_bundle
    ON qris_doc_access_logs (bundle_id, created_at DESC);
