-- QRIS registration → file-delivery portal link.
--
-- Onboarding documents (KTP, selfie+KTP, business-location photo) are uploaded
-- to the GTD file-delivery portal (dev-files.gtd.co.id) at registration intake,
-- alongside the canonical S3 copy. The portal returns a token-gated bundle URL
-- that is embedded in the Nobu Excel batch so Nobu can fetch the merchant's
-- documents without a shared drive. We persist the bundle URL + token here so
-- the link is reproducible (Excel re-render, admin view) without re-uploading.
--
-- Both columns are nullable: portal upload is best-effort and optional (an empty
-- FILES_PORTAL_URL disables it), so a registration without a portal link is valid.

ALTER TABLE qris_registrations
    ADD COLUMN doc_portal_url   TEXT,
    ADD COLUMN doc_portal_token VARCHAR(64);
