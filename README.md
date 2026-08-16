# MiniCC

多租户 SaaS AI Agent 平台：Go 网关 + Python AI 引擎 + Vue 前端。

## 架构

```
┌──────────────┐     ┌───────────────┐     ┌─────────────────┐
│  frontend-vue │ ──► │  Go 网关       │ ──► │  Python AI 引擎  │
│  (Vue 3)      │ SSE │  (internal/)  │     │  (python-engine) │
└──────────────┘     └──────┬────────┘     └────────┬────────┘
                            │                       │
                     ┌──────┴──────┐        ┌───────┴───────┐
                     │ PostgreSQL  │        │  LLM Provider │
                     │ Redis       │        │  (OpenAI 兼容)│
                     └─────────────┘        └───────────────┘
```

- **Go 网关**（`:8080`）：认证 / 计费 / SSE 转发 / 会话与消息落库 / 媒体 / 知识库
- **Python 引擎**（`:8000`）：Agent 循环（模式：常规/极简/PTC/创造）、工具沙箱隔离、三栅栏安全、多提供商消息格式适配
- **前端**（Vue 3 + Vite）：聊天界面（虚拟滚动 / 流式思考 / 工具链还原）

## 快速开始

```bash
# 1. 依赖：PostgreSQL + Redis
docker-compose up -d postgres redis

# 2. 配置
cp .env.example .env   # 填 LLM_API_KEY / LLM_BASE_URL / POSTGRES_DSN / REDIS_ADDR

# 3. 启动（自动构建 Go + 启动 Python 引擎 + 前端）
python run.py start

# 4. 访问 http://localhost:5173
```

## 常用命令

| 命令 | 说明 |
|---|---|
| `python run.py start/stop/status/logs` | 服务管理 |
| `go test ./...` | Go 网关测试 |
| `cd python-engine && python -m pytest tests` | Python 引擎测试 |
| `npm run build --prefix frontend-vue` | 前端构建 |
| `go run ./cmd/minicc` | 单独启动 Go 网关 |

## 关键设计

- **多租户隔离**：每用户沙箱工作区（`sandbox/{tenant}/{user}/workspace`）+ 工具层权限栅栏
- **安全三栅栏**：输入注入检测 / 工具调用三态裁决（拒绝·替换·确认）/ 输出路径脱敏
- **多提供商**：内部中立消息格式（`message_codec`），自动推断 OpenAI/Anthropic/Gemini 并转换
- **消息落库**：`messages`（user/assistant + tool_calls id 集合）、`tool_calls`（完整内容）
- **迁移**：单文件基线 `migrations/20260709000001_initial.up.sql`（从零建库）

## 目录

```
internal/       Go 网关（api/auth/billing/session/storage/...）
python-engine/  Python 引擎（agent/gateway/tools/providers/...）
frontend-vue/   前端（src/components/chat 等）
migrations/     数据库基线迁移
cmd/            Go 入口（minicc 主服务 / minicc-cli / migrate）
```

## License

MIT
