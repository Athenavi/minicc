-- 20260823000003_drop_dead: up
-- 清理死表 episodes（无任何代码读取；模型层使用内存版 EpisodeMemory）
DROP TABLE IF EXISTS episodes;
