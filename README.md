# MiniCC

> 多租户 SaaS AI Agent 平台 — Go 网关 + Python AI 引擎 + Vue 3 前端。
> A multi-tenant SaaS AI Agent platform powered by a Go gateway, a Python AI engine and a Vue 3 frontend.

[![CI](https://img.shields.io/github/actions/workflow/status/athenavi/minicc/ci.yml?branch=main&logo=github&label=CI)](https://github.com/athenavi/minicc/actions)
[![Coverage](https://img.shields.io/codecov/c/github/athenavi/minicc?logo=codecov&label=coverage)](https://codecov.io/gh/athenavi/minicc)
[![Go Version](https://img.shields.io/github/go-mod/go-version/athenavi/minicc?logo=go)](https://github.com/athenavi/minicc/blob/main/go.mod)
[![Release](https://img.shields.io/github/v/release/athenavi/minicc?logo=github)](https://github.com/athenavi/minicc/releases)
[![License](https://img.shields.io/github/license/athenavi/minicc)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

MiniCC 是一套开箱即用的**多租户 AI Agent 控制台**:对话、Agent、工作流、技能、知识库与插件六大工作台互联互通,让 LLM 在真实场景中持续工作。它采用 **Go 网关 + Python AI 引擎**的分离架构——Go 负责认证、限流、计费与流式转发,Python 承载 Agent 推理、任务编排与 RAG。

## ✨ 核心亮点

- **多租户安全隔离** — 数据在租户/用户两级强制隔离:PostgreSQL 行级 `tenant_id` 过滤、Redis 分租户 key 命名空间、Milvus 向量检索 `tenant_id` filter、媒体资源归属校验、每用户插件配置目录、每租户独立配额与限流。
- **六大工作台互联互通** — 对话 / Agent / 工作流 / 技能 / 知识库 / 插件通过 Capability Registry + TaskRouter 统一编排:自然语言任务自动分解为子任务 DAG,跨工作台协同执行并聚合结果(`POST /v1/chat/submit`)。
- **网关 + 引擎架构** — Go 网关是唯一对外入口(认证 / API Key / 限流 / 计费 / SSE 转发),Python 引擎专注 AI 推理;内部通过 `X-Internal-Token` 双向鉴权,**fail-close** 拒绝身份透传绕过。

## 🚀 快速开始

### Docker Compose(推荐,一条命令起全栈)

```bash
cp .env.example .env        # 至少填写 JWT_SECRET 与一个 LLM API Key
docker compose up -d --build
```

启动后访问 <http://localhost:3000>(前端),API 网关在 <http://localhost:8080>,健康检查 `GET /health`。

> 首次启动会自动执行数据库迁移(`migrate` 服务)。生产环境请先修改 `.env` 中的默认口令,并将 `COOKIE_SECURE=true`。

### 本地开发模式

```bash
# 1. 只启动基础设施(PostgreSQL + Redis,可选 MinIO/Milvus/Temporal)
docker compose up -d postgres redis

# 2. 配置
cp .env.example .env        # 填写 LLM_API_KEY / POSTGRES_DSN / REDIS_ADDR 等

# 3. 启动 Go 网关 + Python 引擎 + 前端(自动构建)
python run.py start
```

访问 <http://localhost:5173>。常用命令:`python run.py start|stop|restart|status|logs|build|setup`。

### 环境变量速查

| 变量 | 必填 | 说明 |
|---|---|---|
| `JWT_SECRET` | ✅ | JWT HS256 签名密钥,**至少 32 字符**,`openssl rand -base64 48` 生成;未设置网关直接拒绝启动 |
| `POSTGRES_DSN` | ✅ | PostgreSQL 连接串,如 `postgres://minicc:minicc@localhost:5432/minicc?sslmode=disable` |
| `INTERNAL_TOKEN` | ✅(多租户) | Go 网关 → Python 引擎内部身份透传密钥(`X-Internal-Token`);未配置时 Python 拒绝 query 身份透传(fail-close) |
| `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` / `DEEPSEEK_API_KEY` | 至少一个 | LLM Provider 密钥,网关与引擎共享 |
| `REDIS_PASSWORD` | 视部署 | Redis 密码;docker-compose 中启用 `--requirepass` 后必须设置 |
| `PLUGIN_COMMAND_ALLOWLIST` | 安全建议 | MCP 插件可执行命令白名单(逗号分隔 basename);**留空 = 禁止所有自定义插件命令** |
| `LLM_GATEWAY_KEY` | 可选 | 引擎内置 LLM Gateway 的内部端点鉴权密钥 |
| `COOKIE_SECURE` | 生产必填 `true` | JWT cookie 追加 `Secure` 标志(HTTPS 部署) |
| `DISABLE_REGISTRATION` | 可选 | `true` 关闭公开注册,仅管理员经 `/v1/install/setup` 创建用户 |
| `TRUSTED_PROXY_CIDRS` / `METRICS_TOKEN` | 生产加固 | 可信反向代理网段(防 XFF 伪造);Prometheus 抓取 `/metrics` 的 Bearer token |
| `LLM_PROVIDER` / `LLM_MODEL` | 可选 | 默认 `openai` / `gpt-4o` |

完整变量见 [.env.example](.env.example)(含生产加固清单)。

## 🧩 特性总览

**AI 对话(4 种模式)**

- 常规 `normal` / 极简 `minimal` / PTC `ptc` / 创造 `creative` 四种推理模式,按任务自由切换
- 工具调用全程可视化(工具链还原、SSE 流式输出、会话可取消)

**Agent**

- 多智能体协同、任务分发与结果追踪(`POST /v1/agents/dispatch`)
- 上下文压缩、工具三态裁决(拒绝 / 替换 / 确认)、沙箱工作区 `sandbox/{tenant}/{user}/workspace`

**工作流 DAG**

- 可视化编排多步任务,节点自由连线,运行时编辑;支持并行优化与依赖调度

**技能市场 / Agent 市场 / MCP 市场**

- 企业能力市场(`/v1/ent/market/items`),技能 / Agent / MCP 三类条目统一目录、租户级安装与启停

**知识库 RAG**

- 文档入库、分块与向量化(pgvector / Milvus 双后端)、HNSW 余弦检索、`tenant_id` 级向量数据隔离

**媒体库**

- 上传 / 分片上传 / S3 预签名上传,媒体资源**签名 URL** 访问(HMAC-SHA256,15 分钟有效期,归属校验)

**多租户与安全**

- 租户 / 用户两级数据隔离、JWT + API Key + OAuth/OIDC/SMS 认证、Prompt 注入检测、SSRF 端口白名单、插件命令白名单、分布式限流、可信代理 CIDR、审计日志

**企业版**

- 配额管理、成本中心、RBAC 角色 / 群组、操作审计、模型策略、隐私模式、多域名管理、独立安装向导(`/v1/install/setup`)

## 🏗️ 架构

```
┌──────────────┐   HTTP / SSE / WS    ┌───────────────────┐   Internal API     ┌──────────────────────────┐
│ frontend-vue │ ───────────────────▶ │   Go Gateway :8080 │ ─────────────────▶ │   Python AI Engine :8000 │
│   (Vue 3)    │ ◀─────────────────── │  JWT / API Key     │  X-Internal-Token  │  FastAPI / Agent / RAG   │
└──────────────┘    SSE 流式返回       │  限流 / 计费 / CORS │ ◀───────────────── │  TaskRouter / Workflow   │
                                      └─────────┬─────────┘    SSE / streaming   └────────────┬─────────────┘
                                                │                                             │
                ┌───────────────────────────────┼──────────────────────────┐      ┌──────────┼──────────┐
          PostgreSQL (pgvector)           Redis (必需)                MinIO / S3     Redis     PostgreSQL   Milvus
          会话 / 媒体元数据 / 市场      限流 / 队列 / 语义缓存      媒体对象存储    队列/缓存    pgvector    向量检索
                └───────────────────────────────┼──────────────────────────┘                │
                                                └── Temporal(工作流) ────────────────────────┘
```

- **Go 网关**(`:8080`,`internal/`):认证 / 计费 / SSE 转发 / 会话与消息落库 / 媒体 / 知识库 / 市场 / 企业管控
- **Python 引擎**(`:8000`,`python-engine/`):Agent 循环(4 模式)、TaskRouter 统一编排、工具沙箱隔离、三栏安全(输入净化 / 工具三态 / 输出脱敏)、多 Provider 消息格式适配
- **前端**(Vue 3 + Vite):聊天界面(虚拟滚动 / 流式思维链 / 工具链还原)、六大工作台、管理后台

## 🧰 技术栈

| 层 | 技术 |
|---|---|
| 网关 | Go 1.26,标准库 `net/http`(1.22+ 路由),`pgx/v5`, `go-redis/v9`, `minio-go/v7`, `golang-jwt/v5`, `gorilla/websocket`, `cobra` |
| AI 引擎 | Python 3.11, FastAPI, uvicorn, anthropic / openai SDK, pymilvus / qdrant-client, asyncpg, redis, PyJWT, structlog, Prometheus + OpenTelemetry |
| 前端 | Vue 3.5, Vite, TypeScript, Ant Design Vue 4, Pinia, Vue Router, @vue-flow(DAG 画布), mermaid, KaTeX, ECharts, vitest |
| 存储 | PostgreSQL 16(pgvector), Redis 7, MinIO / S3, Milvus 2.5, Temporal |

## 📁 目录结构

```
├── cmd/                  # Go 入口(minicc 网关 / migrate / minicc-cli / stress)
├── config/               # 网关配置加载(.env + config.json)
├── internal/             # Go 网关:api / auth / billing / broadcast / db / engine
│                         #          / enterprise / id / model / monitor / session / storage
├── python-engine/        # Python AI 引擎:app/{agent, core, gateway, llm, mcp, rag,
│                         #          skill, workflow, tools, knowledge, memory, media, ...}
├── frontend-vue/         # Vue 3 + Vite + TS 前端(src/{views, components, router, stores, api})
├── migrations/           # 数据库迁移(Atlas,单文件基线 + 增量)
├── skills/               # 内置技能定义
├── docs/                 # openapi.yaml、架构文档
├── deploy/               # 部署辅助
├── data/plugins/         # 每用户 MCP 插件配置(运行时)
├── docker-compose.yml    # 全栈编排:postgres/redis/minio/temporal/milvus/gateway/engine/frontend
├── run.py                # 本地开发编排(start/stop/status/logs/build/setup)
├── Makefile              # build / test / lint / fmt / docker-build
└── atlas.hcl             # 迁移工具配置
```

## 🗺️ 路线图

| Now(已完成) | Next | Later |
|---|---|---|
| 六大工作台互联互通(TaskRouter 统一编排) | CI 集成测试 job(Postgres/Redis services) | 插件 SDK 与开发者生态 |
| Redis 必需化(fail-fast,无降级模式) | 覆盖率接入 Codecov 与 badge | 容器级沙箱(gVisor)强化 |
| 技能 / Agent / MCP 三大市场 | GHCR 镜像发布与版本化 Release | Helm / K8s 生产部署 |
| 媒体签名 URL 全链路 | 前端 i18n 国际化 | 多区域高可用与只读副本 |
| 多租户隔离 + 安全加固(SSRF / 命令白名单 / 注入检测) | 文档站与更多内置技能 | 计费支付生产化(支付宝 / 微信 / PayPal) |
| 三端 CI 流水线(go vet/test / ruff+pytest / vue-tsc+build) | 企业 SSO / SCIM 完善 | 语义缓存与 RAG 效果评估工具 |

## 🤝 参与贡献

欢迎提交 Issue 与 PR!请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md)(开发环境、代码规范、提交信息约定)与 [SECURITY.md](SECURITY.md)(漏洞报告流程)。

## 📸 截图

截图占位:请将界面截图(首页 / 对话 / 工作流画布 / 知识库 / 管理后台等)放入 `docs/screenshots/` 目录,并在 `README` 的 Screenshots 小节引用。我们计划收录:登录与安装向导、六工作台首页、对话流式输出与工具链还原、工作流 DAG 画布、媒体库与签名分享、企业市场与配额看板。

## 📚 文档

- [架构文档](docs/ARCHITECTURE.md)(分层、认证链路、统一入口、多租户隔离矩阵、媒体签名流程)
- [OpenAPI](docs/openapi.yaml)

## License

[Apache-2.0](LICENSE)
