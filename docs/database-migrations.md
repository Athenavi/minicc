# 数据库迁移指南

> MiniCC V2 使用 **Atlas** 格式管理数据库迁移。
> 迁移文件在 `migrations/` 目录，HCL schema 定义在 `atlas.hcl`。

---

## 目录

- [初始化](#1-初始化)
- [升级（应用新迁移）](#2-升级)
- [降级（回滚迁移）](#3-降级)
- [新增迁移](#4-新增迁移)
- [Architecture](#5-架构说明)
- [故障处理](#6-故障处理)

---

## 1. 初始化

### 首次创建数据库

```bash
# PostgreSQL 必须已运行
# 默认连接串: postgres://minicc:minicc@localhost:5432/minicc

# 方式 A：让应用自动创建（推荐）
export POSTGRES_DSN="postgres://minicc:minicc@localhost:5432/minicc?sslmode=disable"
go run ./cmd/minicc
# 启动时会自动：
#   1. 检查数据库是否存在，不存在则 CREATE DATABASE
#   2. 运行所有待定迁移
#   3. 监听 HTTP 端口

# 方式 B：手动创建数据库
psql -U postgres -c "CREATE DATABASE minicc;"
psql -U postgres -d minicc -f migrations/202607020001_initial.up.sql
```

### 验证初始化

```bash
# 查看迁移状态
psql -U postgres -d minicc -c "SELECT * FROM schema_migrations;"
# 应返回一条记录：version=202607020001, name=202607020001_initial.up.sql

# 查看表清单
psql -U postgres -d minicc -c "\dt"
# 应显示 11 张表：
#   agent_registry / agent_sessions / api_keys
#   audit_logs / messages / schema_migrations
#   sessions / tasks / tool_calls / users
#   workflow_definitions / workflow_executions
```

---

## 2. 升级

应用新迁移 = 正常启动服务器即可。

```bash
# 自动升级（生产推荐）
go run ./cmd/minicc
# 启动时自动检测并应用所有未执行的迁移文件

# 升级日志输出示例：
# INFO applying migration version=202607020002 file=202607020002_add_indexes.up.sql
# INFO migrations complete applied=1 total=2
```

升级过程是**幂等**的：重复运行不会重复应用已执行的迁移。

### 单迁移事务保障

每个迁移在单独的数据库事务中执行：

```
BEGIN;
  -- 迁移 SQL 在这里执行
  INSERT INTO schema_migrations (...);
COMMIT;  -- 或 ROLLBACK（失败时）
```

迁移失败时自动回滚，不会留下半应用状态。

### 校验和验证

启动时自动验证 `atlas.sum` 中的 SHA-256 校验和：

```
migrations/
├── 202607020001_initial.up.sql      ← 内容
├── 202607020001_initial.down.sql    ← 内容
└── atlas.sum                        ← SHA-256 校验和
```

如果迁移文件被篡改（文件内容与 `atlas.sum` 不匹配），服务器**拒绝启动**。

---

## 3. 降级

### 回滚最后一步迁移

```bash
# 启动时自动检测 schema_migrations 表并按版本降序回滚
# 目前通过 API 触发（开发中）
```

### 手动回滚

```bash
# 1. 找到要回滚的迁移版本
psql -d minicc -c "SELECT * FROM schema_migrations ORDER BY version DESC;"

# 2. 执行对应的 down.sql
psql -d minicc -f migrations/202607020001_initial.down.sql

# 3. 删除 schema_migrations 记录
psql -d minicc -c "DELETE FROM schema_migrations WHERE version = 202607020001;"
```

### 通过代码回滚（CLI 开发中）

```bash
# 未来可通过 CLI 回滚
# go run ./cmd/migrate down          # 回滚最后一步
# go run ./cmd/migrate down 0        # 回滚全部
# go run ./cmd/migrate down 3        # 回滚三步
```

---

## 4. 新增迁移

### 流程总览

```
1. 修改 atlas.hcl（声明式 schema 变更）
2. 运行 atlas migrate diff（自动生成迁移 SQL）
3. 验证生成的 SQL
4. 更新 atlas.sum（或由 atlas CLI 自动更新）
5. 提交代码
6. 部署后自动执行升级
```

### 方式 A：使用 Atlas CLI（推荐）

```bash
# 安装 Atlas CLI
curl -fsSL https://atlas.run/install | sh

# 1. 修改 atlas.hcl 中的表定义

# 2. 生成迁移 diff
atlas migrate diff add_new_table \
  --dir "file://migrations" \
  --dev-url "postgres://minicc:minicc@localhost:5432/minicc" \
  --to "file://atlas.hcl"

# 3. 验证生成的迁移文件
cat migrations/202607020002_add_new_table.up.sql
cat migrations/202607020002_add_new_table.down.sql

# 4. Atlas 自动更新 atlas.sum
```

### 方式 B：手写迁移（无 CLI）

```bash
# 1. 创建迁移文件
cat > migrations/202607020002_add_indexes.up.sql << 'SQL'
CREATE INDEX idx_messages_created_at ON messages(created_at);
SQL

cat > migrations/202607020002_add_indexes.down.sql << 'SQL'
DROP INDEX IF EXISTS idx_messages_created_at;
SQL

# 2. 手动编辑 atlas.hcl 添加索引定义
#    index "idx_messages_created_at" { columns = [column.created_at] }

# 3. 更新 atlas.sum（计算 SHA-256）
#    atlas.sum 格式：
#    h1:base64hash migrations/202607020002_add_indexes.up.sql
#    h1:base64hash migrations/202607020002_add_indexes.down.sql
```

### 迁移文件命名规则

```
migrations/
├── {YYYYMMDD}{NNNN}_{name}.up.sql      # 升级 SQL
├── {YYYYMMDD}{NNNN}_{name}.down.sql    # 降级 SQL
└── atlas.sum                           # 校验和

示例：
  202607020001_initial.up.sql
  202607020001_initial.down.sql
  202607020002_add_indexes.up.sql
  202607020002_add_indexes.down.sql
```

命名规范：
- 时间戳前缀：`YYYYMMDD`（日期）+ `NNNN`（当日序号）
- 名称：小写字母 + 下划线，描述变更内容
- 一一对应：每个 `.up.sql` 必须有对应的 `.down.sql`

---

## 5. 架构说明

### 三层定义

```
atlas.hcl                  ← 声明式目标状态（HCL）
    │
    ▼
migrations/*.sql           ← 版本式迁移文件（自动/手动生成）
    │
    ▼
schema_migrations 表       ← 迁移执行记录（数据库内）
```

### 关键文件

| 文件 | 用途 | 手动修改？ |
|:-----|:-----|:----------|
| `atlas.hcl` | 完整的表结构定义 | ✅ 是 |
| `migrations/*.up.sql` | 升级 SQL 脚本 | ⚠️ 用 CLI 生成或手写 |
| `migrations/*.down.sql` | 降级 SQL 脚本 | ⚠️ 必须与 up 一一对应 |
| `migrations/atlas.sum` | SHA-256 校验和 | ✅ CLI 自动更新，或手动计算 |
| `internal/db/atlas.go` | Go 迁移执行器 | ❌ 不需要 |

### Go 执行器行为

```go
// 启动时的自动流程
1. ConnectPostgres()          → 连接池
2. EnsureDatabase()           → 检查/创建数据库
3. RunAtlasMigrations()       → 校验校验和 → 执行待定迁移（事务包裹）
4. 服务就绪
```

---

## 6. 故障处理

### 迁移文件被修改（校验和不匹配）

```bash
# 错误信息：checksum mismatch for 202607020001_initial.up.sql

# 方案 A：恢复原始文件（推荐）
git checkout migrations/

# 方案 B：重新计算校验和
# 只在确定文件内容正确时使用
cd migrations
sha256sum *.sql | while read hash file; do
  echo "h1:$(echo $hash | xxd -r -p | base64 -w0) $file"
done > atlas.sum
```

### 迁移执行失败

```bash
# 查看失败时的数据库状态
psql -d minicc -c "SELECT * FROM schema_migrations ORDER BY version;"

# 手动修复后删除 schema_migrations 记录
psql -d minicc -c "DELETE FROM schema_migrations WHERE version = 202607020002;"
# 重新启动服务器自动重试
```

### 降级后需要重新升级

```bash
# 降级（回退到上一版本）
psql -d minicc -f migrations/202607020002_add_indexes.down.sql
psql -d minicc -c "DELETE FROM schema_migrations WHERE version = 202607020002;"

# 重新升级（再次应用）
# 重启服务器即可
go run ./cmd/minicc
```

### 完全重建

```bash
# 危险：删除全部数据
psql -U postgres -c "DROP DATABASE minicc;"
psql -U postgres -c "CREATE DATABASE minicc;"

# 重启后自动完成全部迁移
go run ./cmd/minicc
```
