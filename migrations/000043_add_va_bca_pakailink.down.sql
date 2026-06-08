-- Remove BCA Virtual Account via Pakailink provider
DELETE FROM payment_methods WHERE type = 'VA' AND code = '014' AND provider = 'pakailink';
