-- 015_phonetic_index.sql
-- Adds a Soundex phonetic index column to the users table.
-- Populated by the application when full_name is set; enables privacy-preserving
-- fuzzy duplicate detection without storing or comparing plaintext names.

ALTER TABLE users ADD COLUMN full_name_phonetic TEXT;
