SELECT 
  p.payment_id, p.provider, p.status, p.reference_id,
  pl.action, pl.error_code, pl.error_message,
  pl.response::text
FROM payments p
JOIN payment_logs pl ON pl.payment_id = p.id
WHERE p.created_at > NOW() - INTERVAL '30 minutes'
ORDER BY pl.created_at DESC
LIMIT 10;
