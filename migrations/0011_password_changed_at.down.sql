-- 0011_password_changed_at down

ALTER TABLE users DROP COLUMN IF EXISTS password_changed_at;
