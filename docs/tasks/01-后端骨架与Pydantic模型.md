# 任务 01：后端骨架搭建与 Pydantic 协议模型定义

> **所属阶段**：Phase 0 - 基础骨架与协议定义
> **对应模块**：模块 3 (统一协议层) + 后端基建
> **预估工时**：3-4 天
> **依赖**：无

---

## 1. 任务目标

搭建完整的后端 Python 项目骨架，定义所有核心数据契约（Pydantic V2 模型），为后续所有模块提供可依赖的类型系统和协议标准。

## 2. 详细子任务

### 2.1 项目结构与依赖管理

- [ ] 创建 `backend/pyproject.toml`，用 `uv` 或 `poetry` 管理依赖
- [ ] 创建 `backend/.python-version`，锁定 Python 3.14.6
- [ ] 创建 `backend/app/__init__.py` 包结构
- [ ] 配置核心依赖清单：

```toml
[project]
name = "minicc-backend"
version = "0.1.0"
requires-python = ">=3.14"

dependencies = [
    "fastapi>=0.115.0",
    "uvicorn[standard]>=0.32.0",
    "pydantic>=2.10.0",
    "pydantic-settings>=2.6.0",
    "httpx>=0.28.0",
    "websockets>=14.0",
    "gitpython>=3.1.44",
    "redis>=5.2.0",
    "aiosqlite>=0.20.0",
    "anthropic>=0.47.0",
    "openai>=1.55.0",
    "python-dotenv>=1.0.0",
    "rich>=13.9.0",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.3.0",
    "pytest-asyncio>=0.24.0",
    "pytest-cov>=6.0.0",
    "httpx>=0.28.0",
    "ruff>=0.7.0",
    "mypy>=1.13.0",
]
```

### 2.2 目录结构实现

```
backend/
├── pyproject.toml
├── .python-version
├── app/
│   ├── __init__.py
│   ├── main.py                  # FastAPI 入口（Phase 3 前保持最简）
│   ├── core/
│   │   ├── __init__.py
│   │   ├── protocol.py          # ★ 本任务核心：统一协议模型
│   │   ├── context_builder.py   # 骨架 (Phase 1 实现)
│   │   ├── permission.py        # 骨架 (Phase 2 实现)
│   │   └── extensions.py        # 骨架 (Phase 4 实现)
│   ├── engine/
│   │   ├── __init__.py
│   │   ├── session.py           # 骨架
│   │   ├── query_engine.py      # 骨架
│   │   └── task_manager.py      # 骨架
│   ├── tools/
│   │   ├── __init__.py
│   │   ├── base.py              # BaseTool 抽象类
│   │   ├── file_system.py       # 骨架
│   │   ├── shell_executor.py    # 骨架
│   │   └── search_provider.py   # 骨架
│   ├── models/
│   │   ├── __init__.py
│   │   ├── chat.py              # 聊天/消息模型
│   │   ├── tool.py              # 工具调用模型
│   │   ├── session.py           # 会话模型
│   │   └── permission.py        # 权限模型
│   └── utils/
│       ├── __init__.py
│       ├── config.py            # pydantic-settings 配置加载
│       ├── logger.py            # 结构化日志
│       └── security.py          # 路径安全沙箱（骨架）
└── tests/
    ├── __init__.py
    ├── conftest.py
    ├── test_protocol.py
    └── test_models.py
```

### 2.3 核心 Pydantic 模型定义 (`app/models/`)

#### `models/chat.py` - 消息历史模型

- [ ] `class Role(str, Enum)`: `system`, `user`, `assistant`, `tool`
- [ ] `class ContentBlock(BaseModel)`:
  - `type`: Literal["text", "image", "file", "tool_use", "tool_result"]
  - `text`: Optional[str]
  - `source`: Optional[dict] (image/file source)
  - `id`: Optional[str] (tool_use id)
  - `name`: Optional[str] (tool name)
  - `input`: Optional[dict] (tool_use input)
  - `tool_use_id`: Optional[str] (tool_result reference)
  - `content`: Optional[list["ContentBlock"]] (nested for tool_result)
- [ ] `class Message(BaseModel)`:
  - `role`: Role
  - `content`: str | list[ContentBlock]
  - `created_at`: datetime
  - `model`: Optional[str]

#### `models/tool.py` - 工具调用模型

- [ ] `class ToolCall(BaseModel)`:
  - `id`: str (唯一 ID)
  - `type`: Literal["function", "bash", "file_edit", "file_read", ...]
  - `name`: str
  - `input`: dict[str, Any]
  - `status`: Literal["pending", "approved", "rejected", "running", "completed", "failed"]
- [ ] `class ToolResult(BaseModel)`:
  - `tool_call_id`: str
  - `output`: str
  - `is_error`: bool = False
  - `metadata`: dict[str, Any] = {}

#### `models/session.py` - 会话模型

- [ ] `class SessionState(BaseModel)`:
  - `session_id`: str (UUID)
  - `created_at`: datetime
  - `updated_at`: datetime
  - `messages`: list[Message]
  - `pending_tool_calls`: list[ToolCall]
  - `approved_tool_calls`: list[ToolCall]
  - `metadata`: dict[str, Any] = {}

#### `models/permission.py` - 权限模型

- [ ] `class PermissionLevel(str, Enum)`:
  - `READ` = "read" (自动允许)
  - `WRITE` = "write" (需审批)
  - `EXECUTE` = "execute" (需严格审批)
- [ ] `class PermissionRequest(BaseModel)`:
  - `id`: str
  - `tool_name`: str
  - `tool_input`: dict
  - `level`: PermissionLevel
  - `reason`: str (AI 解释为什么需要这个操作)
  - `diff_preview`: Optional[str] (对 WRITE 操作展示 diff)
  - `status`: Literal["pending", "approved", "rejected", "always_allowed"]

### 2.4 BaseTool 抽象类 (`tools/base.py`)

- [ ] `class BaseTool(ABC)`:
  - `name: str` — 工具名称（类变量）
  - `description: str` — 工具描述（给 LLM 的提示）
  - `input_schema: type[BaseModel]` — Pydantic 输入模型
  - `permission_level: PermissionLevel` — 默认权限等级
  - `@abstractmethod async def execute(self, input: BaseModel) -> ToolResult`
- [ ] `class ToolRegistry`:
  - `register(tool: BaseTool)`
  - `get(name: str) -> BaseTool`
  - `list_tools() -> list[BaseTool]`
  - 支持 LLM 的 tools 定义序列化（转 Anthropic/OpenAI 格式）

### 2.5 配置加载 (`utils/config.py`)

- [ ] `class Settings(BaseSettings)`:
  - `model_config = SettingsConfigDict(env_prefix="MINICC_")`
  - `llm_provider`: str = "anthropic"
  - `llm_api_key`: str = ""
  - `llm_model`: str = "claude-sonnet-4-20250514"
  - `workspace_dir`: str = "."
  - `redis_url`: str = "redis://localhost:6379/0"
  - `max_tool_rounds`: int = 25
  - `max_tokens`: int = 8192
  - `log_level`: str = "INFO"

### 2.6 FastAPI 入口骨架 (`main.py`)

- [ ] 带 `lifespan` 的 FastAPI 应用
- [ ] 健康检查端点 `GET /health`
- [ ] 注册 WebSocket 路由 `/ws/{session_id}`（骨架，Phase 1 填充）
- [ ] CORS 中间件配置（允许 Next.js 开发端口）
- [ ] `GET /api/tools` — 返回已注册工具列表（供前端动态渲染）

## 3. 验收标准 (Acceptance Criteria)

| # | 检查项 | 验证方式 |
|:-|:-|:-|
| 1 | `uv run uvicorn app.main:app` 启动无报错 | 启动日志 |
| 2 | `GET /health` 返回 `{"status": "ok"}` | curl 测试 |
| 3 | 所有 Pydantic 模型可被 import 且通过 `model_validate` 验证 | pytest |
| 4 | `BaseTool` 子类可被 `ToolRegistry` 注册和查找 | pytest |
| 5 | `Settings` 从环境变量正确加载 | pytest |
| 6 | CORS 中间件允许前端 `localhost:3000` 跨域 | 手动 curl 测试 |

## 4. 参考资源

- [FastAPI WebSocket 文档](https://fastapi.tiangolo.com/advanced/websockets/)
- [Pydantic V2 模型文档](https://docs.pydantic.dev/latest/)
- [pydantic-settings](https://docs.pydantic.dev/latest/concepts/pydantic_settings/)
- 规划文档 §2 目录结构、§3 Phase 0 任务 0.1-0.2

## 5. 注意事项

- Python 3.14 的 `match` 语句和 `TaskGroup` 是重要特性，尽量使用
- Pydantic V2 用 `model_validate()` 而非 `parse_obj()`
- 所有模型尽量使用 `model_config = ConfigDict(frozen=True)` 增强不可变性
- 参考 claw-code 的模块化注册思想，`ToolRegistry` 后续要支持 MCP 动态注册
