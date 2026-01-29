INSERT INTO clients (
  client_id, name, api_key, sandbox_key,
  callback_url, callback_secret, ip_whitelist
) VALUES
('ppob-id', 'PPOB.id',
 'gb_live_zVXgcbRbtwhWSkf0b28x58Kq0BM1oWcF',
 'gb_sandbox_uG0zclLoCSto4QaEAO6DM0wc7XZ1Da1P',
 'https://ppob.id/api/callback/gtd',
 'gb_secret_arneMhAZ81FNYhHE0VY75dxdKE6JV0xG',
 ARRAY['103.xxx.xxx.xxx']),
('seaply', 'Seaply.co',
 'gb_live_g918lFhQY9RmhPkV750ZboVkOBgp3dWr',
 'gb_sandbox_9D3K2MyrIPjjiSb0LItdFD8H0Rg2WupH',
 'https://seaply.co/api/callback/gtd',
 'gb_secret_e7S67BUCqaUTmBb1ANrpPadRDY9zgnuq',
 ARRAY['103.yyy.yyy.yyy'])
ON CONFLICT (client_id) DO NOTHING;