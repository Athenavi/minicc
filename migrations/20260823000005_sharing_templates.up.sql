-- 20260823000005_sharing_templates: up
-- 1) 知识库共享（visibility: private/tenant，默认 private）
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS visibility varchar(16) NOT NULL DEFAULT 'private';

-- 2) 模板市场
CREATE TABLE IF NOT EXISTS ent_templates
(
    id          uuid                     default gen_random_uuid() not null
        primary key,
    type        varchar(16)                                       not null, -- workflow / agent / skill
    name        varchar(128)                                      not null,
    description text                     default ''::text          not null,
    payload     jsonb                                              not null,
    published   boolean                  default true              not null,
    created_at  timestamp with time zone default now()             not null,
    updated_at  timestamp with time zone default now()             not null
);
CREATE INDEX IF NOT EXISTS idx_ent_templates_type ON ent_templates (type);

-- 3) 内置模板种子（独立开发者开箱即用）
INSERT INTO ent_templates (type, name, description, payload) VALUES
    ('workflow', '周报自动生成', '每周五自动汇总本周会话与 Agent 运行记录，生成 Markdown 周报',
     '{"nodes":[{"id":"n1","type":"input","config":{"label":"输入：本周时间范围"}},{"id":"n2","type":"llm","config":{"prompt":"将以下工作记录整理成周报：$n1"}},{"id":"n3","type":"output","config":{"label":"输出：周报"}}],"edges":[{"from":"n1","to":"n2"},{"from":"n2","to":"n3"}]}'),
    ('workflow', '知识库问答流', '提问 → 知识库检索 → LLM 生成带引用的回答',
     '{"nodes":[{"id":"n1","type":"input","config":{"label":"输入：问题"}},{"id":"n2","type":"knowledge","config":{"kb_id":"","top_k":5,"prompt":"基于检索内容回答：$n1"}},{"id":"n3","type":"output","config":{"label":"输出：回答"}}],"edges":[{"from":"n1","to":"n2"},{"from":"n2","to":"n3"}]}'),
    ('agent', '代码审查 Agent', '读代码、找问题（安全/性能/正确性）、给出修复建议',
     '{"name":"code_reviewer","description":"代码审查 Agent：读代码找问题给建议","system_prompt":"你是资深代码审查员。阅读代码，指出安全、性能与正确性问题，给出修复建议。","tools":[{"name":"read_file","description":"读取文件"},{"name":"grep_files","description":"搜索文本"}],"llm_config":{"model":"deepseek-chat","max_tokens":4096,"temperature":0.3},"max_turns":6,"timeout_seconds":120}'),
    ('agent', '研究助手 Agent', '检索、抓取、归纳，输出带引用的调研报告',
     '{"name":"research_agent","description":"研究型 Agent：检索、抓取、归纳","system_prompt":"你是研究助手。检索并抓取网页，归纳要点，给出带引用的结论。","tools":[{"name":"web_fetch","description":"抓取网页"},{"name":"read_file","description":"读取文件"}],"llm_config":{"model":"deepseek-chat","max_tokens":4096,"temperature":0.4},"max_turns":6,"timeout_seconds":120}'),
    ('skill', '会议纪要', '将会议录音转写文本整理为结构化会议纪要',
     '{"name":"meeting_minutes","description":"将会议文本整理为结构化纪要（结论/待办/风险）","exec":{"type":"prompt","source":"将输入会议文本整理为结构化纪要：会议结论、待办事项（含负责人）、风险与下一步。"},"parameters":[]}')
ON CONFLICT DO NOTHING;
