-- 20260823000006_user_memory_profile: up
-- 目的：创建用户记忆档案表（L2层），用于存储跨会话稳定的结构化事实。
-- 设计：
--   1. 四类槽位：identity（身份）、preference（偏好）、decision（关键决策）、fact（长期事实）
--   2. 冲突检测：user_confirmed 来源的条目不自动覆盖，产出 MemoryConflict 事件
--   3. 衰退归档：180天未引用且confidence<80的条目归档（由后台任务处理）
--   4. 容量限制：每用户200条目软限，超出按confidence×recency淘汰
-- 依赖：无

-- ── user_memory_profile 表 ───────────────────────────────────────
CREATE TABLE IF NOT EXISTS user_memory_profile (
    tenant_id   VARCHAR(64)  NOT NULL,
    user_id     VARCHAR(64)  NOT NULL,
    slot        VARCHAR(32)  NOT NULL,   -- identity / preference / decision / fact
    item_key    VARCHAR(128) NOT NULL,   -- 槽位内键，如 "timezone" / "preferred_language"
    item_value  JSONB        NOT NULL,   -- 值，允许对象（如 {"value": "UTC+8", "note": "上海时区"}）
    confidence  SMALLINT     NOT NULL DEFAULT 50,        -- 0-100
    source      VARCHAR(16)  NOT NULL DEFAULT 'derived', -- user_confirmed / derived / tool_written
    version     INTEGER      NOT NULL DEFAULT 1,
    confirmed_at TIMESTAMPTZ,            -- 用户最后确认时间（NULL=未确认）
    last_referenced_at TIMESTAMPTZ,      -- 最近被召回引用时间（衰退依据）
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, slot, item_key)
);

-- 索引：支持按租户/用户查询 + 衰退归档
CREATE INDEX idx_ump_reference ON user_memory_profile (tenant_id, user_id, last_referenced_at);

-- 索引：支持冲突查询
CREATE INDEX idx_ump_conflict ON user_memory_profile (tenant_id, user_id, source)
    WHERE source = 'user_confirmed';

-- 约束：slot 必须为预定义值
ALTER TABLE user_memory_profile
    ADD CONSTRAINT chk_ump_slot
    CHECK (slot IN ('identity', 'preference', 'decision', 'fact'));

-- 约束：source 必须为预定义值
ALTER TABLE user_memory_profile
    ADD CONSTRAINT chk_ump_source
    CHECK (source IN ('user_confirmed', 'derived', 'tool_written'));

-- 约束：confidence 必须在 0-100 范围内
ALTER TABLE user_memory_profile
    ADD CONSTRAINT chk_ump_confidence
    CHECK (confidence >= 0 AND confidence <= 100);

-- 触发器：自动更新 updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_ump_updated_at
    BEFORE UPDATE ON user_memory_profile
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- 注释
COMMENT ON TABLE user_memory_profile IS '用户记忆档案表（L2层）：存储跨会话稳定的结构化事实';
COMMENT ON COLUMN user_memory_profile.slot IS '槽位类型：identity（身份）、preference（偏好）、decision（关键决策）、fact（长期事实）';
COMMENT ON COLUMN user_memory_profile.item_key IS '槽位内键，如 "timezone" / "preferred_language"';
COMMENT ON COLUMN user_memory_profile.item_value IS '值，允许 JSONB 对象';
COMMENT ON COLUMN user_memory_profile.confidence IS '置信度 0-100，用于淘汰排序';
COMMENT ON COLUMN user_memory_profile.source IS '来源：user_confirmed（用户确认）、derived（提炼）、tool_written（工具写入）';
COMMENT ON COLUMN user_memory_profile.version IS '版本号，每次 upsert 递增';
COMMENT ON COLUMN user_memory_profile.confirmed_at IS '用户最后确认时间，NULL 表示未确认';
COMMENT ON COLUMN user_memory_profile.last_referenced_at IS '最近被召回引用时间，用于衰退归档（180天未引用且confidence<80）';