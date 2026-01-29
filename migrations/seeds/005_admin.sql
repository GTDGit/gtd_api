INSERT INTO admin_users (email, password_hash, name, role)
VALUES
('admin@gtd.co.id',
 '$2a$10$SiNxKFs3aeHUgkpWCnrSWedXRFtK/QuBdF7HpFIIlQsr9jANd35u2',
 'Super Admin',
 'superadmin')
ON CONFLICT (email) DO NOTHING;