-- QRIS Nobu static-merchant onboarding (client-facing API).
--
-- A brand client (ppob.id, Seaply, …) registers a static-QRIS merchant through
-- the api service exactly like any other client request (Bearer api_key +
-- X-Client-Id). Nobu has NO registration API — onboarding is a manual Excel form
-- delivered to Nobu's WhatsApp group. So every client registration is captured
-- here as a row, accumulated, and rendered into a single Nobu-format Excel file
-- twice a day (10:00 & 15:00 WIB, Mon–Fri; Sat/Sun fold into Monday's batch 1).
--
-- Lifecycle: pending_batch -> submitted (in an Excel batch sent to Nobu)
--            -> activated (Nobu returned subMerchantId/storeId/terminalId; an
--               operator created the qris_merchants row + QR string)
--            -> rejected (Nobu declined).
-- On activation the client receives a `qris.merchant.activated` webhook; every
-- successful payment later fires `qris.payment.success` (see qris_callbacks).

CREATE TABLE IF NOT EXISTS qris_registrations (
    id                     BIGSERIAL PRIMARY KEY,
    client_id              INT REFERENCES clients(id) ON DELETE SET NULL, -- owning brand
    registration_ref       VARCHAR(64) NOT NULL,                          -- client-facing idempotency / lookup key

    -- Owner identity (Excel: NAMA LENGKAP PEMILIK USAHA / NIK / NO HP / EMAIL)
    owner_full_name        VARCHAR(128) NOT NULL,                         -- per e-KTP
    owner_nik              VARCHAR(16)  NOT NULL,                         -- 16-digit e-KTP number
    owner_phone            VARCHAR(32)  NOT NULL,                         -- WhatsApp
    email                  VARCHAR(128) NOT NULL,                         -- company / PIC email

    -- Business (Excel: NAMA USAHA / MCC / ALAMAT / KOTA / KODE POS / TOKO FISIK)
    business_name          VARCHAR(25)  NOT NULL,                         -- max 25 chars, capitalised
    mcc                    VARCHAR(8)   NOT NULL,                         -- 4-digit MCC code
    address_street         VARCHAR(255) NOT NULL,                         -- nama jalan
    address_rt             VARCHAR(8),
    address_rw             VARCHAR(8),
    address_kelurahan      VARCHAR(64),
    address_kecamatan      VARCHAR(64),
    city                   VARCHAR(64)  NOT NULL,                         -- kota / kabupaten
    postal_code            VARCHAR(8),
    has_physical_store     BOOLEAN NOT NULL DEFAULT TRUE,                 -- TOKO FISIK Ya/Tidak

    -- Classification (Excel: KATEGORI OMZET / TIPE QRIS / KATEGORI RISK)
    omzet_category         VARCHAR(8)   NOT NULL,                         -- UMI|UKE|UME|UBE|URE|PSO|BLU
    qris_type             VARCHAR(16)  NOT NULL DEFAULT 'statis',        -- statis only for now
    risk_category          VARCHAR(16)  NOT NULL DEFAULT 'Low',          -- Low|Medium|High

    -- Estimates & extras (Excel: WEBSITE / ESTIMASI SALES VOLUME / ESTIMASI TRANSAKSI)
    website                VARCHAR(255),
    estimated_sales_volume BIGINT,                                        -- estimated monthly rupiah
    estimated_tx_count     INT,                                           -- estimated monthly tx count

    -- Documents & batch linkage
    doc_bundle_id          BIGINT REFERENCES qris_doc_bundles(id) ON DELETE SET NULL,
    batch_id               BIGINT,                                        -- FK added after qris_nobu_batches below
    qris_merchant_id       BIGINT REFERENCES qris_merchants(id) ON DELETE SET NULL,

    status                 VARCHAR(20) NOT NULL DEFAULT 'pending_batch',  -- pending_batch|submitted|activated|rejected
    note                   TEXT,                                          -- operator / rejection note
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- registration_ref is unique per client (the client supplies or we generate it).
CREATE UNIQUE INDEX IF NOT EXISTS uq_qris_registration_ref
    ON qris_registrations (client_id, registration_ref);

-- The batch worker scans pending registrations; the client lists its own.
CREATE INDEX IF NOT EXISTS idx_qris_registration_status
    ON qris_registrations (status, created_at);
CREATE INDEX IF NOT EXISTS idx_qris_registration_client
    ON qris_registrations (client_id, created_at DESC);

-- qris_nobu_batches is one rendered Excel file. The worker is idempotent on
-- (batch_date, batch_seq): only one file per slot ever exists. No registrants in
-- a slot => no row (no empty file).
CREATE TABLE IF NOT EXISTS qris_nobu_batches (
    id                 BIGSERIAL PRIMARY KEY,
    batch_date         DATE NOT NULL,                       -- WIB calendar date the batch belongs to
    batch_seq          SMALLINT NOT NULL,                   -- 1 (10:00) | 2 (15:00)
    period_label       VARCHAR(64),                         -- human label e.g. "2026-06-15 Batch 1 (10:00 WIB)"
    file_storage_key   TEXT NOT NULL,                       -- storage key of the .xlsx
    file_name          VARCHAR(255) NOT NULL,
    registration_count INT NOT NULL DEFAULT 0,
    status             VARCHAR(16) NOT NULL DEFAULT 'generated', -- generated | sent
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_qris_nobu_batch_slot
    ON qris_nobu_batches (batch_date, batch_seq);

-- Wire registrations.batch_id to the batches table now that it exists.
ALTER TABLE qris_registrations
    ADD CONSTRAINT fk_qris_registration_batch
    FOREIGN KEY (batch_id) REFERENCES qris_nobu_batches(id) ON DELETE SET NULL;

-- qris_merchants gains the Nobu MID (subMerchantId) and a back-link to the
-- registration that produced it. (nmid already stores storeId/NMID; terminal_id
-- already stores the TID.)
ALTER TABLE qris_merchants
    ADD COLUMN IF NOT EXISTS sub_merchant_id VARCHAR(64);  -- Nobu MID
ALTER TABLE qris_merchants
    ADD COLUMN IF NOT EXISTS registration_id BIGINT REFERENCES qris_registrations(id) ON DELETE SET NULL;

-- qris_callbacks: outbound client webhooks for QRIS events, mirroring the
-- payment_callbacks retry model (30s/1m/5m/30m/2h, max 5 attempts). Signature is
-- X-GTD-Signature: sha256=<hmac> using the client's callback secret.
CREATE TABLE IF NOT EXISTS qris_callbacks (
    id               BIGSERIAL PRIMARY KEY,
    client_id        INT NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    qris_merchant_id BIGINT REFERENCES qris_merchants(id) ON DELETE SET NULL,
    qris_payment_id  BIGINT REFERENCES qris_payments(id) ON DELETE SET NULL,
    event            VARCHAR(48) NOT NULL,                  -- qris.merchant.activated | qris.payment.success
    target_url       TEXT NOT NULL,                         -- snapshot of client callback url at enqueue time
    payload          JSONB NOT NULL,                        -- the exact body that is signed + delivered
    status           VARCHAR(16) NOT NULL DEFAULT 'pending', -- pending | success | failed
    attempts         INT NOT NULL DEFAULT 0,
    max_attempts     INT NOT NULL DEFAULT 5,
    next_retry_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_status_code INT,
    last_error       TEXT,
    delivered_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The callback worker claims due rows: pending + next_retry_at <= now.
CREATE INDEX IF NOT EXISTS idx_qris_callback_due
    ON qris_callbacks (status, next_retry_at);
CREATE INDEX IF NOT EXISTS idx_qris_callback_client
    ON qris_callbacks (client_id, created_at DESC);
