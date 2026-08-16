-- 会话置顶 + 分享
--
-- pinned: 前端会话菜单"置顶/取消置顶"（列表 ORDER BY pinned DESC, updated_at DESC）
ALTER TABLE sessions ADD COLUMN pinned boolean NOT NULL DEFAULT false;

-- conversation_shares: 会话分享（类似 chat.deepseek.com/share/{id}）
--   id         分享 token（随机 base62，公开 URL 的一部分，不可枚举）
--   session_id 被分享的会话
--   message_ids 创建分享时用户选中的消息 id（仅文本消息，按 created_at 升序渲染）
--   revoked_at 非空 = 分享已取消（前端"取消分享"）
CREATE TABLE conversation_shares (
    id varchar(32) PRIMARY KEY,
    session_id varchar(128) NOT NULL,
    user_id uuid NOT NULL,
    title varchar(255) NOT NULL DEFAULT '',
    message_ids text[] NOT NULL DEFAULT '{}',
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    revoked_at timestamp with time zone
);
CREATE INDEX idx_conversation_shares_session ON conversation_shares (session_id) WHERE revoked_at IS NULL;
