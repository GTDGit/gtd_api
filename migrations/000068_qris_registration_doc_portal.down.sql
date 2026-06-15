ALTER TABLE qris_registrations
    DROP COLUMN IF EXISTS doc_portal_url,
    DROP COLUMN IF EXISTS doc_portal_token;
