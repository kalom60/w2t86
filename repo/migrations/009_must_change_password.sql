-- 009_must_change_password.sql
--
-- Adds must_change_password flag to users.  When set to 1 the auth handler
-- redirects the user to a mandatory password-reset page immediately after login
-- instead of the normal dashboard.
--
-- The seeded default admin account ships with a known password; it must rotate
-- that credential before accessing any portal functionality.

ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0;

-- Force the seeded admin account to change their password on first login.
UPDATE users SET must_change_password = 1 WHERE username = 'admin';
