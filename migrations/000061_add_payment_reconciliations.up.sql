-- payment_reconciliations holds payments whose inbound webhook claim did not
-- match the authoritative provider inquiry (status and/or paid amount). When a
-- webhook arrives we re-query the provider; if they disagree the payment is NOT
-- transitioned and NOT forwarded to the client. Instead a row is recorded here
-- for resolution, either automatically by the status worker once the provider
-- settles to a consistent final state, or manually by an admin operator.
--
-- This makes unverifiable SNAP webhooks (Pakailink/DANA) safe: truth always
-- comes from the provider inquiry, never from the raw webhook body.
CREATE TABLE IF NOT EXISTS payment_reconciliations (
    id              BIGSERIAL PRIMARY KEY,
    payment_id      VARCHAR(50) NOT NULL,                 -- payments.payment_id (public UUIDv4)
    provider        payment_provider NOT NULL,
    reason          VARCHAR(32) NOT NULL,                 -- status_mismatch | amount_mismatch | status_amount_mismatch
    webhook_status  VARCHAR(20),
    inquiry_status  VARCHAR(20),
    webhook_amount  BIGINT,
    inquiry_amount  BIGINT,
    expected_amount BIGINT,                               -- snapshot of payments.total_amount at detection time
    webhook_payload JSONB,
    inquiry_payload JSONB,
    status          VARCHAR(16) NOT NULL DEFAULT 'open',  -- open | resolved
    resolved_status VARCHAR(20),                          -- final status applied on resolution
    resolved_by     VARCHAR(100),                         -- admin username | 'worker' | 'webhook'
    resolution_note TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at     TIMESTAMPTZ
);

-- At most one open reconciliation per payment; repeated mismatching webhooks
-- update the existing open row rather than piling up duplicates.
CREATE UNIQUE INDEX IF NOT EXISTS uq_recon_open
    ON payment_reconciliations (payment_id) WHERE status = 'open';

CREATE INDEX IF NOT EXISTS idx_recon_status_created
    ON payment_reconciliations (status, created_at DESC);
