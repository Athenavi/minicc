-- MiniCC V2.0: Media Assets — store AI-generated content (text, images, documents)
CREATE TABLE IF NOT EXISTS media_assets (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    type VARCHAR(16) NOT NULL DEFAULT 'text',  -- image / text / document / code / audio / video
    name VARCHAR(255) NOT NULL,
    content TEXT DEFAULT '',
    file_path VARCHAR(512) DEFAULT '',
    mime_type VARCHAR(64) DEFAULT '',
    thumbnail VARCHAR(512) DEFAULT '',
    metadata JSONB DEFAULT '{}',
    tags TEXT[] DEFAULT '{}',
    category VARCHAR(64) DEFAULT '',  -- writing / image / office / translation / etc.
    size BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_media_user ON media_assets(user_id);
CREATE INDEX idx_media_type ON media_assets(type);
CREATE INDEX idx_media_category ON media_assets(category);
CREATE INDEX idx_media_created ON media_assets(created_at);
