# 任务 11：会话持久化与 TaskManager

> **所属阶段**：Phase 3 - 状态持久化与任务系统 (State & Task)
> **对应模块**：模块 7 (State Layer / Task Manager)
> **预估工时**：3-4 天
> **依赖**：任务 05 (QueryEngine)、任务 07 (FileSystem Tools)、任务 08 (Shell Executor)

---

## 1. 任务目标

实现会话状态的持久化存储（Redis 热数据 + SQLite 历史记录）和后台任务管理系统（TaskManager），支持长任务在用户断开后继续运行、会话恢复等功能。

## 2. 详细子任务

### 2.1 存储方案设计

```
热数据 (Redis)                   冷数据 (SQLite)
┌──────────────────────┐      ┌──────────────────────┐
│ 当前会话状态          │      │ 历史消息归档          │
│ 最近 N 条消息         │      │ 完成的工具调用记录     │
│ 待审批权限请求        │      │ Token 用量统计         │
│ "始终允许"缓存        │      │ 审批日志               │
│ 会话 TTL: 2h         │      │ 永久保留               │
└──────────────────────┘      └──────────────────────┘
```

### 2.2 Redis 存储层

- [ ] 文件：`backend/app/utils/redis_client.py`

```python
class RedisClient:
    """Redis 连接池管理器"""
    
    def __init__(self, redis_url: str):
        self.redis = await redis.from_url(redis_url, decode_responses=True)
    
    async def set_session_state(self, session_id: str, state: SessionState, ttl: int = 7200):
        """保存会话状态到 Redis，2h 过期"""
        ...
    
    async def get_session_state(self, session_id: str) -> SessionState | None:
        """从 Redis 恢复会话状态"""
        ...
    
    async def append_message(self, session_id: str, message: Message):
        """追加消息到会话的消息列表（Redis List 结构）"""
        ...
    
    async def get_recent_messages(self, session_id: str, count: int = 5) -> list[Message]:
        """获取最近 N 条消息（用于会话恢复）"""
        ...
    
    async def delete_session(self, session_id: str):
        """删除会话状态"""
        ...
```

- [ ] 消息序列化：Pydantic → JSON (model_dump_json) → Redis String
- [ ] Redis 数据结构：
  - `session:{id}:state` → JSON (hash?)
  - `session:{id}:messages` → Redis List (每个元素是 JSON 消息)
  - `session:{id}:pending_requests` → Redis Hash

### 2.3 SQLite 持久化层

- [ ] 文件：`backend/app/utils/sqlite_store.py`

```python
class SQLiteStore:
    """SQLite 历史记录存储"""
    
    async def __aenter__(self):
        self.db = await aiosqlite.connect("minicc_history.db")
        await self._init_tables()
        return self
    
    async def _init_tables(self):
        """建表"""
        await self.db.execute("""
            CREATE TABLE IF NOT EXISTS sessions (
                session_id TEXT PRIMARY KEY,
                created_at TEXT,
                updated_at TEXT,
                metadata TEXT
            )
        """)
        await self.db.execute("""
            CREATE TABLE IF NOT EXISTS messages (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                session_id TEXT,
                role TEXT,
                content TEXT,
                created_at TEXT,
                model TEXT,
                FOREIGN KEY (session_id) REFERENCES sessions(session_id)
            )
        """)
        await self.db.execute("""
            CREATE TABLE IF NOT EXISTS tool_calls (
                id TEXT PRIMARY KEY,
                session_id TEXT,
                name TEXT,
                input TEXT,
                result TEXT,
                is_error INTEGER,
                duration_ms INTEGER,
                created_at TEXT
            )
        """)
        await self.db.execute("""
            CREATE TABLE IF NOT EXISTS approval_logs (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                session_id TEXT,
                tool_name TEXT,
                action TEXT,
                created_at TEXT
            )
        """)
        await self.db.commit()
    
    async def save_session(self, session: SessionState):
        """持久化会话信息"""
        ...
    
    async def save_message(self, session_id: str, message: Message):
        """持久化单条消息"""
        ...
    
    async def get_session_history(self, session_id: str) -> list[Message]:
        """获取完整会话历史"""
        ...
```

### 2.4 SessionManager — 统一会话管理

- [ ] 文件：`backend/app/engine/session.py`

```python
class SessionManager:
    """
    会话状态管理层。
    统一管理热数据（Redis）和冷数据（SQLite）的读写。
    """
    
    def __init__(self, redis_client: RedisClient, sqlite_store: SQLiteStore):
        self.redis = redis_client
        self.sqlite = sqlite_store
    
    async def save_message(self, session_id: str, message: Message):
        """同时写入 Redis（热）和 SQLite（冷）"""
        await self.redis.append_message(session_id, message)
        await self.sqlite.save_message(session_id, message)
    
    async def resume_session(self, session_id: str) -> SessionState | None:
        """
        恢复会话。
        1. 尝试从 Redis 获取完整状态（用户刚断开的会话）
        2. 如果 Redis 没有，从 SQLite 重建基础状态
        3. 加载最近 5 条消息到热缓存
        """
        ...
    
    async def snapshot_session(self, session_id: str, state: SessionState):
        """定期快照会话状态到 Redis"""
        ...
```

### 2.5 TaskManager — 后台任务管理

- [ ] 文件：`backend/app/engine/task_manager.py`

```python
class TaskManager:
    """
    后台任务管理器。
    管理所有正在运行的 Agent 会话，支持断开后继续执行。
    """
    
    def __init__(self):
        self.active_tasks: dict[str, asyncio.Task] = {}  # session_id → Task
        self.session_states: dict[str, SessionState] = {}
    
    async def start_session_task(
        self,
        session_id: str,
        query_engine: QueryEngine,
    ) -> asyncio.Task:
        """
        启动后台会话任务。
        即使 WebSocket 断开，任务继续在后台运行。
        """
        task = asyncio.create_task(
            self._run_session(session_id, query_engine)
        )
        self.active_tasks[session_id] = task
        return task
    
    async def _run_session(self, session_id: str, engine: QueryEngine):
        """后台运行会话任务"""
        try:
            await engine.run()
        except asyncio.CancelledError:
            await self._cleanup_session(session_id)
        finally:
            # 快照最终状态到持久层
            await self._snapshot(session_id)
    
    async def cancel_session(self, session_id: str):
        """取消运行中的会话"""
        if session_id in self.active_tasks:
            self.active_tasks[session_id].cancel()
    
    def is_session_active(self, session_id: str) -> bool:
        """检查会话是否仍在运行"""
        return session_id in self.active_tasks and not self.active_tasks[session_id].done()
```

### 2.6 会话恢复流程

```
用户重连 WebSocket (/ws/{session_id})
  │
  ├── TaskManager.is_session_active(session_id)?
  │   ├── Yes → 复用已有 QueryEngine 实例
  │   │         重放最近 5 条消息到前端
  │   │         恢复审批等待状态
  │   │
  │   └── No  → SessionManager.resume_session(session_id)
  │              重建 QueryEngine
  │              重放最近 5 条消息
  │              继续待办的工具调用
```

### 2.7 会话心跳与过期

- [ ] Redis TTL：2h（无活动自动过期）
- [ ] 前端连接断开时，不清除会话——保留 30 分钟等待重连
- [ ] 30 分钟后无重连 → 终止后台任务（可配置）

### 2.8 迁移与数据清理

- [ ] 定期归档旧会话（SQLite 中超过 30 天的会话可压缩）
- [ ] Redis 自动过期无需手动清理

### 2.9 单元测试

- [ ] 测试 Redis 读写会话状态
- [ ] 测试 SQLite 建表和消息持久化
- [ ] 测试 SessionManager 恢复流程
- [ ] 测试 TaskManager 启停会话
- [ ] 测试断开后消息缓存正确性

## 3. 验收标准

| # | 检查项 | 验证方式 |
|:-|:-|:-|
| 1 | Redis 可读写会话状态 | pytest (需要 mock Redis) |
| 2 | SQLite 可持久化消息和工具调用记录 | pytest |
| 3 | 断开后重新连接，最近 5 条消息正确恢复 | 端到端测试（断开 WebSocket → 重连） |
| 4 | 后台任务在 WebSocket 断开后继续运行 | 启动命令 → 断开前端 → 检查后端日志 |
| 5 | 会话 30 分钟无重连后自动清理 | 集成测试 |
| 6 | 重连时恢复审批等待状态 | 端到端测试 |

## 4. 参考资源

- [redis-py 文档](https://redis-py.readthedocs.io/)
- [aiosqlite 文档](https://aiosqlite.omnilib.dev/)
- [Python TaskGroup](https://docs.python.org/3/library/asyncio-task.html#task-groups)
- Claude Code AppStateStore 设计理念（参考 xuanyuancode 教程）
- 规划文档 §3 Phase 3 任务 3.1-3.2

## 5. 注意事项

- Redis 和 SQLite 都是可选依赖——如果没有配置，Session 退化为纯内存模式
- 消息序列化/反序列化必须可靠——不要破坏 Pydantic 的模型校验
- 会话恢复时不能泄露敏感信息（API Key 等）
- 后台任务必须有超时兜底，防止僵尸任务
- Redis TTL 应可配置，不同场景需求不同
