-- tool_calls.id 改为 text。
--
-- tool_call id 来自 LLM 返回（如 OpenAI/DeepSeek 的 "call_xxx"），并非 uuid；
-- uuid 列导致 SaveToolCall 插入必失败（SQLSTATE 22P02），tool_calls 从未落库，
-- 前端刷新/切换标签后工具卡片无法还原（消息渲染丢失）。
-- 表无外键引用（日志型表，见初始迁移注释），改类型安全；
-- 与 internal/model/model.go 的 ToolCall.ID(string) 匹配。
ALTER TABLE tool_calls ALTER COLUMN id TYPE text USING id::text;
ALTER TABLE tool_calls ALTER COLUMN id SET DEFAULT (gen_random_uuid())::text;
