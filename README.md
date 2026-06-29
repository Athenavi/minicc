# MiniCC — Minimal Engineering-grade AI Coding Assistant

<div align="center">

![MiniCC](https://img.shields.io/badge/MiniCC-v0.1-blue)
![Python](https://img.shields.io/badge/Python-3.14%2B-blue)
![Next.js](https://img.shields.io/badge/Next.js-16-black)
![License](https://img.shields.io/badge/License-MIT-green)
![Tests](https://img.shields.io/badge/Tests-120-passing-brightgreen)

**MiniCC** 是一个在浏览器中运行的极简工程级 AI 编程助手，具备完整的"计划→执行→审批"闭环。它参考 Claude Code 的架构设计，实现了 8 大核心模块和 25 个工具。

[English](#) · [简体中文](#) · [报告 Bug](https://github.com/Athenavi/minicc/issues) · [功能请求](https://github.com/Athenavi/minicc/issues)

</div>

---

## 🌟 特性

### 🧠 Agent 主循环
- 完整的 `QueryEngine` 会话级主循环编排器
- 跨轮次状态管理（消息历史、权限记忆、文件缓存、用量统计）
- 流式 LLM 响应 → 工具调用 → 结果回流闭环
- 支持 Anthropic / OpenAI / Ollama 三种 Provider

### 🔧 25 个工具
| 分类 | 工具 |
|:-----|:-----|
| 文件 | `read_file`, `write_to_file`, `str_replace_editor`, `notebook_edit` |
| Shell | `bash` |
| 搜索 | `glob`, `grep`, `tool_search` |
| 网络 | `web_fetch`, `web_search` |
| 会话 | `ask_user_question`, `todo_write`, `enter_plan_mode`, `exit_plan_mode` |
| Agent | `agent`, `send_message`, `skill`, `task_*` (6) |
| 扩展 | `list_mcp_resources`, `read_mcp_resource`, `lsp_*` (3) |

### 🔒 权限系统
- 三级权限：READ（自动放行）/ WRITE（审批）/ EXECUTE（严格审批）
- 审批等待模型（`asyncio.Event` + WebSocket 推送）
- "始终允许"记忆 + 拒绝记忆

### 📦 上下文压缩
- 6 层分级压缩管线：Budget → Snip → MicroCompact → Collapse → AutoCompact
- 先局部瘦身，再折叠视图，最后才全局摘要
- Token 预算管理 + 自动触发

### 🌐 MCP + LSP 扩展
- MCP 外部能力接入总线（Stdio / HTTP / WebSocket）
- LSP 语言智能（跳转定义、查找引用、悬停提示）
- Python 插件热加载

### 💬 WebSocket 实时通信
- 流式消息实时渲染
- 工具调用卡片 + 审批按钮
- 暗黑模式 / 响应式布局
- Slash 命令系统（/help, /status, /tools, /clear, /config）

---

## 🏗 架构

```
用户 → WebSocket → main.py
  ├── /command → CommandDispatcher
  └── user_message → QueryEngine.submit_message()
       ├── ContextBuilder (Git + Rules + Memory)
       ├── 6 层压缩管线
       ├── LLM Provider (Anthropic/OpenAI/Ollama)
       ├── 工具调用 → PermissionHandler → 工具执行
       └── 结果回流 → 下一轮
```

### 模块架构

| 模块 | 功能 | 状态 |
|:-----|:-----|:-----|
| **Core Loop** | QueryEngine 主循环编排 | ✅ |
| **Context Assembly** | Git/Memory/Rules 注入 | ✅ |
| **Prompt Engineering** | 6 层分层提示词系统 | ✅ |
| **Tool System** | 25 工具 + ToolUseContext | ✅ |
| **Permission System** | READ/WRITE/EXECUTE 三级 | ✅ |
| **State Layer** | Redis + SQLite 持久化 | ✅ |
| **Context Compression** | 6 层分级压缩 | ✅ |
| **Extensions** | MCP + LSP + Plugin | ✅ |

---

## 🚀 快速开始

### 前提

- Python 3.14+
- Node.js 22+
- (可选) Redis 7+

### 安装

```bash
# 克隆
git clone https://github.com/Athenavi/minicc.git
cd minicc

# 后端
cd backend
pip install -e ".[dev]"

# 前端
cd ../frontend
npm install
```

### 配置

```bash
# 复制环境变量模板
cp backend/.env.example backend/.env

# 编辑 .env
MINICC_LLM_PROVIDER=anthropic       # 或 openai
MINICC_LLM_API_KEY=sk-ant-...        # 你的 API Key
MINICC_LLM_MODEL=claude-sonnet-4-20250514
```

### 启动

```bash
# 终端 1: 后端
cd backend
uvicorn app.main:app --reload --port 8000

# 终端 2: 前端
cd frontend
npm run dev
```

打开 http://localhost:3000 即可使用。

### 运行测试

```bash
cd backend
pytest tests/ -v --ignore=tests/test_session.py --ignore=tests/test_shell_executor.py
```

---

## 📸 截图

> 等待补充

---

## 📚 文档

- [规划文档](docs/规划文档.md) — 项目总体规划与技术选型
- [任务清单](docs/tasks/00-任务索引.md) — 14 个详细任务清单

---

## 🧩 项目结构

```
minicc/
├── backend/                # FastAPI 后端
│   ├── app/
│   │   ├── core/           # 核心抽象层（Context, Permission, MCP, LSP）
│   │   ├── engine/         # QueryEngine, TaskManager, Compactor
│   │   ├── tools/          # 25 个工具实现
│   │   ├── models/         # Pydantic 数据模型
│   │   └── utils/          # Config, Logger, Security, Redis, SQLite
│   └── tests/              # 120 测试
├── frontend/               # Next.js 前端
│   └── src/
│       ├── app/            # App Router
│       ├── components/     # UI 组件
│       └── hooks/          # WebSocket 钩子
├── docs/                   # 文档
│   └── tasks/             # 14 个任务清单
└── .minicc/                # 项目规则与技能
    ├── rules/              # Ponytail 规则
    ├── commands/           # Slash 命令
    └── skills/             # 可执行技能
```

---

## 🛣 路线图

- [x] Phase 0: 基础骨架与协议定义
- [x] Phase 1: 会话主循环与上下文装配
- [x] Phase 2: 真实工具落地与权限系统
- [x] Phase 3: 状态持久化与任务系统
- [x] Phase 4: MCP/LSP 扩展系统
- [x] Phase 5: 打磨与优化
- [ ] 端到端 LLM 联调
- [ ] 多 Agent 协作完善
- [ ] WebSearch API 集成
- [ ] 联网搜索增强

---

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

---

## 📄 许可

[MIT](LICENSE)

---

## 🙏 致谢

- [Claude Code](https://docs.anthropic.com/en/docs/claude-code/overview) — 架构参考
- [Ponytail](https://github.com/DietrichGebert/ponytail) — YAGNI 开发哲学
- [Reasonix](https://github.com/Athenavi/DeepSeek-Reasonix) — Reasonix 运行时环境
- [Claw Code](https://github.com/ultraworkers/claw-code) — Rust 参考实现
- Xuanyuan Code 社区的 Claude Code 源码解析系列
