DROP INDEX IF EXISTS idx_qris_doc_access_bundle;
DROP TABLE IF EXISTS qris_doc_access_logs;

DROP INDEX IF EXISTS idx_qris_doc_file_bundle;
DROP INDEX IF EXISTS uq_qris_doc_file_token;
DROP TABLE IF EXISTS qris_doc_files;

DROP INDEX IF EXISTS idx_qris_doc_bundle_merchant;
DROP INDEX IF EXISTS uq_qris_doc_bundle_token;
DROP TABLE IF EXISTS qris_doc_bundles;
