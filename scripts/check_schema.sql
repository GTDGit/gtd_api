SELECT column_name, is_nullable, column_default 
FROM information_schema.columns 
WHERE table_name='payments' AND column_name IN ('payment_detail','payment_instruction')
ORDER BY column_name;
