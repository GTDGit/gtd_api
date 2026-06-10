-- Migration 000057 (down): rename payment status 'Success' -> 'Paid'

ALTER TYPE payment_status RENAME VALUE 'Success' TO 'Paid';
