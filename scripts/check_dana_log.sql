SELECT 
  p.payment_id, p.provider, p.status,
  pl.error_message,
  pl.request::text as req,
  pl.response::text as resp
FROM payments p
JOIN payment_logs pl ON pl.payment_id = p.id
WHERE p.provider = 'dana_direct'
  AND p.created_at > NOW() - INTERVAL '10 minutes'
ORDER BY pl.created_at DESC
LIMIT 3;
