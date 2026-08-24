-- 20260824000002_system_settings_encrypted: up
-- system_settings 增加 encrypted 标记：true 表示 value 已用 APP_SECRET 派生密钥 AES-GCM 加密

ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS encrypted boolean NOT NULL DEFAULT false;