-- 20260823000005_sharing_templates: down
DELETE FROM ent_templates WHERE name IN ('周报自动生成','知识库问答流','代码审查 Agent','研究助手 Agent','会议纪要');
DROP INDEX IF EXISTS idx_ent_templates_type;
DROP TABLE IF EXISTS ent_templates;
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS visibility;
