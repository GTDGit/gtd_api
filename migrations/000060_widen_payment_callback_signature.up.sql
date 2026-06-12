-- payment_callbacks.signature and transfer_callbacks.signature were varchar(255),
-- too small for SNAP BI providers (Pakailink, DANA, BNC) whose X-SIGNATURE header
-- is an RSA-2048 base64 signature (~344 chars). The oversized value caused INSERT
-- in persistRawCallback / CreateTransferCallback to fail with "value too long for
-- type character varying(255)", returning HTTP 500 to the provider so the callback
-- was never recorded or processed. TEXT removes the bound since signature length
-- varies with key size.
ALTER TABLE payment_callbacks ALTER COLUMN signature TYPE TEXT;
ALTER TABLE transfer_callbacks ALTER COLUMN signature TYPE TEXT;
