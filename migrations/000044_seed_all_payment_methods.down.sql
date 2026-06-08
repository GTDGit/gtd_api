-- Rollback 000044: remove added payment methods and column
DELETE FROM payment_methods WHERE type IN ('EWALLET', 'RETAIL') AND code IN (
    'PAYDANA','PAYGOPAY','PAYOVO','PAYLINKAJA','PAYSHOPEE','PAYASTRAPAY',
    'ALFAMART','INDOMARET'
);
DELETE FROM payment_methods WHERE type = 'VA' AND code IN (
    '002','009','490','008','451','022','013','147','011','016','028','019','153','110','120','213'
) AND id NOT IN (SELECT id FROM payment_methods WHERE type = 'VA' AND code = '014');

ALTER TABLE payment_methods DROP COLUMN IF EXISTS provider_display_name;
