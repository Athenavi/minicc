-- Enterprise Identity (RBAC & Groups): down
-- 按依赖反序回滚（关联表先删，主表后删；索引随表一并移除）

DROP TABLE IF EXISTS ent_group_roles;
DROP TABLE IF EXISTS ent_group_members;
DROP TABLE IF EXISTS ent_user_roles;
DROP TABLE IF EXISTS ent_groups;
DROP TABLE IF EXISTS ent_roles;
