# MiniCC V2.0

**企业级 AI 引擎 — Go 实现，零 Python 依赖**

> 从"编码助手"到"企业 AI 操作系统"的 Go V2.0 重构版本。
> 单二进制 (~19MB)，无外部运行时依赖。

---

## 🏗 架构

```
cmd/
├── minicc/main.go      主服务入口
└── migrate/main.go     数据库迁移 CLI
internal/
├── agent/              Agent 路由 + 运行时 + 会话管理
├── api/                HTTP 层 (router/middleware/handlers/SSE/WebSocket)
├── auth/               JWT + API Key + RBAC
├── broadcast/          SSE 事件广播 (Redis Pub/Sub + local fallback)
├── cache/              Redis 缓存层 + 语义缓存 (WordCount Embedder)
├── commands/           Slash 命令系统
├── db/                 PostgreSQL + Redis 连接池 + Atlas 迁移
├── engine/             TurnOrchestrator (多轮 LLM + 工具循环)
├── graph/              StateGraph DAG 构建 + 执行
├── llm/                LLM 网关 (多Provider/熔断/缓存/限流)
├── model/              数据模型 (User/Session/Message/Task/ToolCall)
├── monitor/            监控指标 + OpenTelemetry 追踪 (spans)
├── pm/                 产品经理工具 (PRD/teachdesign/taskdecomp)
├── queue/              Redis Streams 消息队列 + Worker 池
├── session/            会话管理器 (Redis 热缓存 + PG 持久化)
├── storage/            文件存储抽象 (Local / S3/MinIO)
└── tools/              工具注册表 (16+ 内置工具)
config/config.go        环境变量配置
migrations/             Atlas-format SQL 迁移 (7 个)
```

---

## 🚀 快速开始

### 前置要求

- Go 1.26+
- PostgreSQL 17+（可选，无 DB 降级运行）
- Redis 7+（可选，无 Redis 降级运行）

### 启动

```bash
# 1. 配置环境变量
cp .env.example .env
# 编辑 .env 填入你的配置

# 2. 确保 JWT_SECRET 已设置（必填，≥16 字符）
export JWT_SECRET=$(openssl rand -hex 32)

# 3. 启动服务（自动建库 + 自动迁移）
go run ./cmd/minicc

# 或构建后启动
make build
./build/minicc
```

### 数据库迁移 CLI

```bash
# 编译
go build ./cmd/migrate

# 应用全部迁移
./migrate up

# 回滚 1 步
./migrate down

# 查看迁移状态
./migrate status

# 创建新迁移
./migrate create add_user_preferences

# 创建数据库（如不存在）
./migrate ensure-db
```

### 环境变量

详见 [.env.example](.env.example)。核心变量：

| 变量 | 默认值 | 必填 | 说明 |
|:-----|:-------|:----|:------|
| `PORT` | `8080` | | 监听端口 |
| `JWT_SECRET` | — | ✅ | JWT 签名密钥，≥16 字符 |
| `POSTGRES_DSN` | `postgres://minicc:minicc@localhost:5432/minicc` | | PostgreSQL 连接串 |
| `REDIS_ADDR` | `localhost:6379` | | Redis 地址 |
| `LLM_API_KEY` | — | | LLM API Key |
| `LLM_MODEL` | `gpt-4o` | | 默认模型 |
| `STORAGE_BACKEND` | `local` | | 存储后端 (`local`/`s3`) |
| `LOG_LEVEL` | `info` | | 日志级别 |

---

## 📡 API 端点

| 方法 | 路径 | 说明 | 认证 |
|:-----|:-----|:-----|:-----|
| GET | `/health` | 健康检查 | 否 |
| GET | `/ready` | 就绪检查 | 否 |
| GET | `/events` | SSE 事件流 (实时 AI 输出) | 否 |
| GET | `/ws/{sessionId}` | WebSocket 双向通信 | 否 |
| POST | `/submit` | 发送消息 (legacy) | 可选 |
| POST | `/cancel` | 取消生成 | 否 |
| POST | `/mode` | 设置模式 | 否 |
| POST | `/approve` / `/reject` | 审批工具调用 | 否 |
| | | | |
| POST | `/v1/auth/login` | 登录 | 否 |
| POST | `/v1/auth/register` | 注册 | 否 |
| POST | `/v1/auth/refresh` | 刷新 Token | Cookie |
| POST | `/v1/auth/logout` | 登出 | Cookie |
| | | | |
| GET | `/v1/conversations` | 会话列表 | Cookie |
| POST | `/v1/conversations` | 创建会话 | Cookie |
| GET | `/v1/conversations/{id}` | 会话详情 | Cookie |
| DELETE | `/v1/conversations/{id}` | 删除会话 | Cookie |
| | | | |
| GET | `/v1/tools` | 工具列表 | 否 |
| POST | `/v1/tools/execute` | 执行工具 | 可选 |
| | | | |
| GET | `/v1/system/health` | 健康评分 | 否 |
| GET | `/v1/system/traces` | 调用追踪 | 否 |
| GET | `/v1/metrics` | 监控指标 | Cookie |
| GET | `/v1/llm/metrics` | LLM 指标 | Cookie |
| GET | `/v1/llm/cache` | 缓存统计 | Cookie |
| | | | |
| GET | `/v1/install/status` | 安装状态 | 否 |
| POST | `/v1/install/setup` | 初始设置 | 否 |
| GET | `/v1/profile` | 用户资料 | Cookie |
| GET | `/v1/tasks` | 任务列表 | Cookie |
| | | | |
| GET/POST | `/api/editor/files` | 文件列表 | 否 |
| GET | `/api/editor/read` | 读取文件 | 否 |
| POST | `/api/editor/write` | 写入文件 | 否 |
| | | | |
| GET/POST/DELETE | `/v1/media` | 媒体库 | 可选 |
| GET | `/v1/agents` | Agent 列表 | 否 |
| POST | `/v1/agents/dispatch` | 调度 Agent | 可选 |
| | | | |
| GET/POST/PUT/DELETE | `/v1/admin/...` | 管理 API | Admin |
| POST | `/v1/admin/maintenance` | 维护操作 | Admin |

认证方式：HTTP-only Cookie（浏览器）或 `Authorization: Bearer <token>`。

---

## 🧪 测试

```bash
# 运行全部测试
go test ./... -count=1 -timeout=60s

# 仅运行集成测试
go test ./internal/api/... -run TestIntegration -v

# 代码检查
go vet ./...

# 编译
go build ./cmd/minicc
go build ./cmd/migrate

# 全部
make test      # test + vet + build
```

**测试覆盖**: 18 个包，~220+ 测试用例，全部通过。

---

## 🔒 安全特性

- **口令**: bcrypt 哈希
- **会话**: JWT (HTTP-only Secure SameSite=Strict Cookie)
- **CORS**: 白名单模式
- **CSP**: Content-Security-Policy 头
- **限流**: IP 级 TokenBucket + 用户/模型级 Redis 滑动窗口
- **请求体**: 1MB 上限
- **路径安全**: 文件遍历防护

---

## 📦 构建

```bash
make build             # → build/minicc (~19MB)
make test              # 测试 + lint
```

---

## 🖥️ 前端

```bash
cd frontend
npm install
npm run dev  # → http://localhost:3000
```

---

## 📄 License

MIT
