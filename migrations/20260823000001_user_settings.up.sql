-- 20260823000001_user_settings: up
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'users' AND column_name = 'settings') THEN
        ALTER TABLE users ADD COLUMN settings jsonb NOT NULL DEFAULT '{}'::jsonb;
    END IF;
END $$;
