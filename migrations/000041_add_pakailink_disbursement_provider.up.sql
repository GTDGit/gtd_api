-- Add 'pakailink' as a valid disbursement_provider so the disbursement system
-- can route via PakaiLink Bisnis (SNAP BI Service 42/43/45).
ALTER TYPE disbursement_provider ADD VALUE IF NOT EXISTS 'pakailink';
