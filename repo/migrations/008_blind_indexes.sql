-- 008_blind_indexes.sql
--
-- Adds deterministic HMAC blind-index columns for full_name and external_id.
-- AES-256-GCM produces a unique ciphertext per encryption (randomized IV), so
-- SQL equality comparisons on the ciphertext columns are useless for duplicate
-- detection.  The blind-index columns store HMAC-SHA256(derived_key, plaintext)
-- which is deterministic and can be compared with = in SQL while the primary
-- columns remain randomized and opaque.
--
-- Index derivation: HMAC-SHA256(encryptionKey, "blind_index_v1") → sub-key;
--                   HMAC-SHA256(sub-key, plaintext) → index value (hex).

ALTER TABLE users ADD COLUMN full_name_idx    TEXT;
ALTER TABLE users ADD COLUMN external_id_idx  TEXT;

-- Index columns for fast equality scans in FindDuplicateUsers.
CREATE INDEX IF NOT EXISTS idx_users_full_name_idx   ON users(full_name_idx);
CREATE INDEX IF NOT EXISTS idx_users_external_id_idx ON users(external_id_idx);
