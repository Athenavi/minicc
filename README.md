# MiniCC V2

**企业级 AI 引擎 — Go 实现，零 Python 依赖**

> 从"编码助手"到"企业 AI 操作系统"的 Go 重构版本。
> 原 Python 版（V0.1-V1.5）已全部迁移到 Go。

---

## 🏗 架构

```
cmd/minicc/main.go  ← 单二进制入口 (~15MB, scratch 镜像)
internal/
├── agent/          Agent 路由 + 会话管理
├── api/            HTTP 层 (router/middleware/response/auth/chat)
├── auth/           JWT + API Key + RBAC
├── brain/          企业大脑 (query/decision/predict/compliance)
├── broadcast/      Redis Pub/Sub 跨实例事件广播
├── collab/         协作平台 (task/wiki/okr/message/meeting)
├── commands/       Slash 命令系统
├── db/             PostgreSQL + Redis 连接池 + 迁移
├── devops/         DevOps 工具 (architect/coding/test/review/ci)
├── engine/         引擎主循环
├── graph/          StateGraph DAG 构建+执行
├── llm/            LLM 网关 (多Provider路由+熔断+缓存)
├── model/          数据模型
├── monitor/        监控计数
├── pm/             产品经理 (PRD/techdesign/taskdecomp)
├── queue/          Redis Streams 消息队列
├── storage/        文件存储抽象 (Local/S3)
├── support/        客服营销 (ticket/kb/chatbot/campaign)
├── tools/          工具注册表 (文件读写等)
└── workflow/       工作流引擎 (DSL/执行器/状态追踪)
config/config.go    环境变量配置
```

---

## 🚀 快速开始

### 前置要求

- Go 1.24+
- PostgreSQL 17+（可选，无 DB 时运行在降级模式）
- Redis 7+（可选，无 Redis 时运行在降级模式）

### 数据库初始化

首次使用需要初始化数据库。详见完整的 [数据库迁移指南](docs/database-migrations.md)。

快速初始化：

```bash
# 确保 PostgreSQL 运行中
# 启动服务（自动建库 + 自动迁移）
export JWT_SECRET=$(openssl rand -hex 32)
go run ./cmd/minicc
```

### 启动

```bash
# 设置 JWT_SECRET（必须）
export JWT_SECRET=$(openssl rand -hex 32)

# 启动
go run ./cmd/minicc

# 或构建后启动
make build
./build/minicc
```

### 环境变量

| 变量 | 默认值 | 说明 |
|:-----|:-------|:-----|
| `PORT` | `8080` | 监听端口 |
| `JWT_SECRET` | **必填** | JWT 签名密钥，32+ 字节 hex |
| `JWT_EXPIRATION` | `24h` | Token 有效期 |
| `CORS_ORIGINS` | `http://localhost:3000` | 允许的 CORS 来源（逗号分隔） |
| `POSTGRES_DSN` | `postgres://minicc:minicc@localhost:5432/minicc?sslmode=disable` | PostgreSQL 连接串 |
| `REDIS_ADDR` | `localhost:6379` | Redis 地址 |
| `LLM_API_KEY` | — | LLM 提供商 API Key |
| `LLM_MODEL` | `gpt-4o` | 默认模型 |
| `STORAGE_BACKEND` | `local` | 存储后端 (local/s3) |
| `LOG_LEVEL` | `info` | 日志级别 (debug/info/warn/error) |

---

## 📡 API

| 方法 | 路径 | 说明 | 认证 |
|:-----|:-----|:-----|:-----|
| GET | `/health` | 健康检查 | 否 |
| GET | `/ready` | 就绪检查 | 否 |
| GET | `/events` | SSE 事件流 | 否 |
| POST | `/v1/auth/login` | 登录 | 否 |
| POST | `/v1/auth/register` | 注册 | 否 |
| POST | `/v1/auth/refresh` | 刷新 token | Cookie |
| POST | `/v1/auth/logout` | 登出 | Cookie |
| GET | `/v1/profile` | 用户资料 | Cookie |
| GET | `/v1/status` | 系统状态 | Cookie |
| POST | `/v1/chat` | 发送消息 | Cookie |
| GET | `/v1/metrics` | 监控指标 | Cookie |
| GET | `/v1/llm/metrics` | LLM 指标 | Cookie |

认证方式：HTTP-only Cookie（浏览器）或 `Authorization: Bearer <token>`（API 客户端）。

---

## 🖥️ 前端

```bash
cd frontend
npm install
npm run dev  # → http://localhost:3000
```

| 页面 | 路由 | 说明 |
|:-----|:-----|:------|
| 聊天 | `/` | AI 对话界面 |
| Agent 控制台 | `/agents` | Agent 调度与监控 |
| 代码编辑器 | `/editor` | AI 代码编辑器 |
| 工作流 | `/workflow` | 可视化工作流编辑器 |
| RPA | `/rpa` | 自动化控制台 |
| 企业 OS | `/enterprise` | Collab/Brain/Support |
| DevOps | `/devops` | DevOps 平台 |
| 系统 | `/system` | 可观测性仪表盘 |
| 登录 | `/login` | 登录页 |
| 注册 | `/register` | 注册页 |
| 个人 | `/profile` | 用户资料 |

---

## 🔒 安全特性

- **口令**: bcrypt 哈希（无明文存储，无 dev 绕过）
- **会话**: JWT 存储于 HTTP-only Secure SameSite=Strict Cookie
- **CORS**: 白名单模式（默认仅 localhost:3000）
- **CSP**: Content-Security-Policy 头保护
- **限流**: 令牌桶 per-IP（生产建议切换 Redis 滑动窗口）
- **请求体**: 1MB 上限
- **路径安全**: 文件路径遍历防护

---

## 🧪 测试

```bash
make test      # Go 单元测试
make lint      # go vet + golangci-lint
make build     # 编译
```

## 🗄️ 数据库迁移

参考 [docs/database-migrations.md](docs/database-migrations.md)：

- **初始化**：首次创建数据库 + 应用全部迁移
- **升级**：启动时自动应用新迁移
- **降级**：执行 `.down.sql` 回滚
- **新增**：修改 `atlas.hcl` → `atlas migrate diff`

---

## 📦 构建

```bash
make build             # 本地编译 → build/minicc
make docker-build      # Docker 多阶段构建 → ~15MB
```

---

## 📄 License

MIT
