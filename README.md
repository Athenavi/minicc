# MiniCC - 企业级 AI Agent 平台

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Python](https://img.shields.io/badge/Python-3.11+-3776AB?style=flat&logo=python)](https://python.org)
[![Vue](https://img.shields.io/badge/Vue-3.5-4FC08D?style=flat&logo=vue.js)](https://vuejs.org)
[![License](https://img.shields.io/badge/License-Apache%202.0-green)](LICENSE)

**Go 高并发网关 + Python AI 数据面 · HTTP 代理架构**

</div>

---

## 🏗️ 架构概览

```
┌─────────────────────────────────────────────────────────────┐
│                    Vue 3.5 前端                              │
└───────────────────────────┬─────────────────────────────────┘
                            │ HTTP / SSE / WebSocket
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  Go 网关（纯代理层）                                          │
│  认证/授权 │ 计费拦截 │ 会话持久化 │ 限流/CORS │ SSE 代理     │
└──────────────┬──────────────────────────────────────────────┘
               │ HTTP 代理（所有业务请求转发到 Python）
               ▼
┌──────────────────────────────────────────────────────────────┐
│  Python AI 引擎（数据面）                                     │
│  Agent Runtime │ LLM Gateway │ Graph/Workflow │ MCP 工具     │
│  RAG │ 记忆管理 │ Skill 系统 │ 工具注册表 │ 可观测性         │
└──────────────┬───────────────────────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────────────────────┐
│  PostgreSQL │ Redis │ Milvus │ MinIO                         │
└─────────────────────────────────────────────────────────────┘
```

## 🚀 核心特性

### Go 网关（纯代理层）
- 🔐 **认证授权**: JWT + API Key，RBAC 权限控制
- 💰 **计费拦截**: Token 用量追踪，余额预检，阶梯计费，Stripe/PayPal
- 📡 **SSE/WebSocket 代理**: 零丢失转发 Python 引擎事件到前端
- 🛡️ **中间件链**: 限流、CORS、日志、追踪、安全头
- 💾 **会话持久化**: 对话历史 PostgreSQL 存储

### Python AI 引擎（数据面）
- 🤖 **Agent Runtime**: 完整 ReAct 推理循环，多轮工具调用
- 🔄 **LLM Gateway**: 多 Provider 路由/熔断/语义缓存/预算/限流
- 📊 **Graph/Workflow**: DAG 工作流编排，PostgreSQL 持久化
- 🔌 **MCP 工具**: Model Context Protocol 支持，动态工具发现
- 🔍 **RAG 检索**: 文档分块、嵌入、Milvus 向量召回
- 🧠 **记忆管理**: 短期记忆（Redis）、长期记忆（Milvus）
- 🛠️ **Skill 系统**: 动态技能安装、生成、执行
- 📊 **可观测性**: OpenTelemetry + Prometheus + structlog

## 📁 项目结构

```
minicc/
├── cmd/minicc/             # Go 网关入口
├── internal/               # Go 网关代码（11 个包）
│   ├── api/                #   API 路由 + 中间件 + Handler
│   ├── auth/               #   JWT 认证
│   ├── billing/            #   计费系统
│   ├── broadcast/          #   SSE 事件广播
│   ├── db/                 #   PostgreSQL + Redis 连接
│   ├── engine/             #   Python 引擎客户端
│   ├── id/                 #   Snowflake ID 生成
│   ├── model/              #   数据模型
│   ├── monitor/            #   监控指标
│   ├── session/            #   会话管理
│   └── storage/            #   文件存储
├── python-engine/          # Python AI 引擎
│   └── app/
│       ├── agent/          #   Agent Runtime（ReAct 推理）
│       ├── api/            #   HTTP API 端点
│       ├── gateway/        #   LLM Gateway（路由/缓存/预算）
│       ├── mcp/            #   MCP 客户端
│       ├── memory/         #   记忆管理
│       ├── providers/      #   LLM Provider 适配器
│       ├── rag/            #   RAG 检索
│       ├── skill/          #   Skill 存储
│       ├── tools/          #   工具注册表 + 核心工具
│       ├── workflow/       #   工作流引擎
│       ├── db.py           #   PostgreSQL 连接池
│       └── config.py       #   配置管理
├── frontend-vue/           # Vue 3.5 前端
├── migrations/             # 数据库迁移
├── docker-compose.yml      # Docker 编排
└── Dockerfile              # Go 网关镜像
```

## 🛠️ 快速开始

### 1. 克隆项目

```bash
git clone https://github.com/Athenavi/minicc.git
cd minicc
```

### 2. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env，填入 API Key 和数据库连接信息
```

### 3. 启动所有服务

```bash
docker-compose up -d
```

### 4. 本地开发

```bash
# Go 网关
go run ./cmd/minicc

# Python 引擎
cd python-engine
pip install -r requirements.txt
python -m app.main

# 前端
cd frontend-vue
npm install && npm run dev
```

## 📖 API 文档

### Go 网关端点（代理到 Python）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/submit` | 发送消息（SSE 流式响应）→ Python Agent |
| `GET` | `/v1/agents` | 列出 Agent → Python |
| `POST` | `/v1/agents/dispatch` | 调度 Agent → Python |
| `GET/POST` | `/v1/graphs` | 工作流 CRUD → Python |
| `POST` | `/v1/graphs/{id}/execute` | 执行工作流 → Python |
| `GET/POST` | `/v1/skills` | Skill 管理 → Python |
| `GET/POST` | `/v1/tools` | 工具管理 → Python |

### Go 网关端点（本地处理）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/v1/auth/login` | 用户登录 |
| `POST` | `/v1/auth/register` | 用户注册 |
| `GET` | `/v1/billing/balance` | 查询余额 |
| `GET` | `/v1/conversations` | 会话列表 |
| `GET` | `/v1/kb` | 知识库管理 |
| `GET` | `/v1/admin/*` | 管理后台 |

### Python 引擎端点

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/v1/agent/submit` | Agent 执行（ReAct 循环） |
| `POST` | `/v1/agent/run` | Agent 推理（单轮） |
| `GET` | `/v1/graphs` | 工作流 CRUD |
| `POST` | `/v1/graphs/{id}/execute` | 工作流执行 |
| `GET` | `/v1/skills` | Skill 管理 |
| `GET` | `/v1/tools` | 工具列表 |
| `GET` | `/healthz` | 健康检查 |

## 📄 许可证

本项目采用 [Apache 2.0 许可证](LICENSE)。

---

<div align="center">
  <sub>Built with ❤️ by the MiniCC Team</sub>
</div>
