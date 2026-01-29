INSERT INTO clients (client_id, name, api_key, sandbox_key, callback_url, callback_secret, ip_whitelist) VALUES
('ppob-id', 'PPOB.id', 'gb_live_zVXgcbRbtwhWSkf0b28x58Kq0BM1oWcF', 'gb_sandbox_uG0zclLoCSto4QaEAO6DM0wc7XZ1Da1P', 'https://ppob.id/api/callback/gtd', 'gb_secret_arneMhAZ81FNYhHE0VY75dxdKE6JV0xG', ARRAY['103.xxx.xxx.xxx']),
('seaply', 'Seaply.co', 'gb_live_g918lFhQY9RmhPkV750ZboVkOBgp3dWr', 'gb_sandbox_9D3K2MyrIPjjiSb0LItdFD8H0Rg2WupH', 'https://seaply.co/api/callback/gtd', 'gb_secret_e7S67BUCqaUTmBb1ANrpPadRDY9zgnuq', ARRAY['103.yyy.yyy.yyy'])
ON CONFLICT (client_id) DO NOTHING;

-- ============================================
-- PAYMENT METHODS
-- ============================================

INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, fee_percent, min_amount, max_amount, expired_duration, display_order, is_active, payment_instruction) VALUES
-- Virtual Account - Direct
('VA', '014', 'BCA Virtual Account', 'pakailink', 'flat', 4500, 0, 10000, 50000000, 86400, 1, true,
'{
  "atm": ["Masukkan kartu ATM dan PIN", "Pilih menu Transfer > BCA Virtual Account", "Masukkan nomor VA", "Konfirmasi pembayaran"],
  "mobileBanking": ["Login ke BCA Mobile", "Pilih m-Transfer > BCA Virtual Account", "Masukkan nomor VA", "Konfirmasi dengan PIN"],
  "internetBanking": ["Login ke KlikBCA", "Pilih Transfer Dana > Transfer ke BCA Virtual Account", "Masukkan nomor VA", "Konfirmasi dengan KeyBCA"]
}'::jsonb),

('VA', '002', 'BRI Virtual Account', 'bri_direct', 'flat', 4000, 0, 10000, 50000000, 86400, 2, true,
'{
  "atm": ["Masukkan kartu ATM dan PIN", "Pilih menu Transaksi Lain > Pembayaran > Lainnya", "Masukkan kode BRIVA (88908) + nomor VA", "Konfirmasi pembayaran"],
  "mobileBanking": ["Login ke BRI Mobile", "Pilih Mobile Banking BRI > Pembayaran > BRIVA", "Masukkan nomor VA", "Konfirmasi dengan PIN"],
  "internetBanking": ["Login ke Internet Banking BRI", "Pilih Pembayaran > BRIVA", "Masukkan nomor VA", "Konfirmasi dengan mToken"]
}'::jsonb),

('VA', '009', 'BNI Virtual Account', 'bni_direct', 'flat', 4000, 0, 10000, 50000000, 86400, 3, true,
'{
  "atm": ["Masukkan kartu ATM dan PIN", "Pilih menu Lainnya > Transfer > Rekening Tabungan", "Masukkan nomor VA", "Konfirmasi pembayaran"],
  "mobileBanking": ["Login ke BNI Mobile Banking", "Pilih Transfer > Virtual Account Billing", "Masukkan nomor VA", "Konfirmasi dengan PIN"],
  "internetBanking": ["Login ke BNI Internet Banking", "Pilih Transfer > Transfer ke BNI Virtual Account", "Masukkan nomor VA", "Konfirmasi dengan mToken"]
}'::jsonb),

('VA', '008', 'Mandiri Virtual Account', 'mandiri_direct', 'flat', 4000, 0, 10000, 50000000, 86400, 4, true,
'{
  "atm": ["Masukkan kartu ATM dan PIN", "Pilih menu Bayar/Beli > Lainnya > Multi Payment", "Masukkan kode perusahaan (89661) + nomor VA", "Konfirmasi pembayaran"],
  "mobileBanking": ["Login ke Livin by Mandiri", "Pilih Bayar > Multipayment", "Masukkan kode perusahaan + nomor VA", "Konfirmasi dengan PIN"],
  "internetBanking": ["Login ke Mandiri Internet Banking", "Pilih Bayar > Multipayment", "Masukkan kode perusahaan + nomor VA", "Konfirmasi dengan MPIN"]
}'::jsonb),

('VA', '490', 'Bank Neo Virtual Account', 'bnc_direct', 'flat', 4000, 0, 10000, 50000000, 86400, 5, true,
'{
  "mobileBanking": ["Login ke Bank Neo Mobile", "Pilih Transfer > Virtual Account", "Masukkan nomor VA", "Konfirmasi dengan PIN"],
  "internetBanking": ["Login ke Bank Neo Internet Banking", "Pilih Transfer > Virtual Account", "Masukkan nomor VA", "Konfirmasi"]
}'::jsonb),

-- Virtual Account - Via Pakailink
('VA', '451', 'BSI Virtual Account', 'pakailink', 'flat', 4500, 0, 10000, 50000000, 86400, 6, true,
'{
  "atm": ["Masukkan kartu ATM dan PIN", "Pilih menu Transfer", "Masukkan nomor VA", "Konfirmasi pembayaran"],
  "mobileBanking": ["Login ke BSI Mobile", "Pilih Transfer", "Masukkan nomor VA", "Konfirmasi dengan PIN"]
}'::jsonb),

('VA', '028', 'OCBC Virtual Account', 'pakailink', 'flat', 4500, 0, 10000, 50000000, 86400, 7, true,
'{
  "mobileBanking": ["Login ke OCBC Mobile", "Pilih Transfer", "Masukkan nomor VA", "Konfirmasi dengan PIN"],
  "internetBanking": ["Login ke OCBC Internet Banking", "Pilih Transfer", "Masukkan nomor VA", "Konfirmasi"]
}'::jsonb),

('VA', '022', 'CIMB Niaga Virtual Account', 'pakailink', 'flat', 4500, 0, 10000, 50000000, 86400, 8, true,
'{
  "atm": ["Masukkan kartu ATM dan PIN", "Pilih menu Transfer", "Masukkan nomor VA", "Konfirmasi pembayaran"],
  "mobileBanking": ["Login ke Octo Mobile", "Pilih Transfer", "Masukkan nomor VA", "Konfirmasi dengan PIN"]
}'::jsonb),

('VA', '013', 'Permata Virtual Account', 'pakailink', 'flat', 4500, 0, 10000, 50000000, 86400, 9, true,
'{
  "atm": ["Masukkan kartu ATM dan PIN", "Pilih menu Transaksi Lainnya > Pembayaran > Pembayaran Lainnya", "Masukkan nomor VA", "Konfirmasi pembayaran"],
  "mobileBanking": ["Login ke PermataMobile X", "Pilih Bayar Tagihan", "Masukkan nomor VA", "Konfirmasi dengan PIN"]
}'::jsonb),

('VA', '147', 'Bank Muamalat Virtual Account', 'pakailink', 'flat', 4500, 0, 10000, 50000000, 86400, 10, true,
'{
  "atm": ["Masukkan kartu ATM dan PIN", "Pilih menu Transfer", "Masukkan nomor VA", "Konfirmasi pembayaran"],
  "mobileBanking": ["Login ke Muamalat DIN", "Pilih Transfer", "Masukkan nomor VA", "Konfirmasi dengan PIN"]
}'::jsonb),

-- E-Wallet
('EWALLET', 'DANA', 'DANA', 'dana_direct', 'percent', 0, 1.50, 1000, 10000000, 1800, 11, true,
'{
  "desktop": ["Klik tombol Bayar dengan DANA", "Login dengan akun DANA", "Konfirmasi pembayaran"],
  "mobile": ["Buka aplikasi DANA", "Scan QR atau klik link pembayaran", "Konfirmasi dengan PIN"]
}'::jsonb),

('EWALLET', 'OVO', 'OVO', 'ovo_direct', 'percent', 0, 2.00, 1000, 10000000, 1800, 12, true,
'{
  "steps": ["Push notification akan dikirim ke aplikasi OVO", "Buka aplikasi OVO", "Konfirmasi pembayaran dengan PIN"]
}'::jsonb),

('EWALLET', 'GOPAY', 'GoPay', 'midtrans', 'percent', 0, 2.00, 1000, 10000000, 1800, 13, true,
'{
  "steps": ["Buka aplikasi Gojek", "Tap Bayar dan scan QR code", "Konfirmasi pembayaran dengan PIN"]
}'::jsonb),

('EWALLET', 'SHOPEEPAY', 'ShopeePay', 'midtrans', 'percent', 0, 2.00, 1000, 10000000, 1800, 14, true,
'{
  "steps": ["Buka aplikasi Shopee", "Tap ShopeePay dan scan QR code", "Konfirmasi pembayaran dengan PIN"]
}'::jsonb),

-- QRIS
('QRIS', 'MPM', 'QRIS', 'dana_direct', 'percent', 0, 0.70, 1000, 10000000, 1800, 15, true,
'{
  "steps": ["Buka aplikasi e-wallet atau mobile banking", "Pilih menu Scan QR / QRIS", "Scan QR code", "Konfirmasi pembayaran"],
  "supportedApps": ["GoPay", "OVO", "DANA", "LinkAja", "ShopeePay", "Mobile Banking"]
}'::jsonb),

-- Retail
('RETAIL', 'INDOMARET', 'Indomaret', 'xendit', 'flat', 5000, 0, 10000, 5000000, 86400, 16, true,
'{
  "steps": ["Kunjungi gerai Indomaret terdekat", "Sampaikan untuk pembayaran Xendit", "Berikan kode pembayaran", "Bayar sesuai nominal", "Simpan struk sebagai bukti"]
}'::jsonb),

('RETAIL', 'ALFAMART', 'Alfamart', 'xendit', 'flat', 5000, 0, 10000, 5000000, 86400, 17, true,
'{
  "steps": ["Kunjungi gerai Alfamart terdekat", "Sampaikan untuk pembayaran Xendit", "Berikan kode pembayaran", "Bayar sesuai nominal", "Simpan struk sebagai bukti"]
}'::jsonb)
ON CONFLICT (type, code) DO NOTHING;