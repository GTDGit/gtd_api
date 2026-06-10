-- Store the client's return URL (redirect target after payment) per payment.
-- Mirrors callback_url. Used for e-wallet redirect flows and echoed back in the
-- payment response so the client can see what it sent.
ALTER TABLE payments ADD COLUMN IF NOT EXISTS return_url TEXT;
