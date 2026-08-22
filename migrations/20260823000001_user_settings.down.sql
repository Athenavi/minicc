-- 20260823000001_user_settings: down
ALTER TABLE users DROP COLUMN IF EXISTS settings;
