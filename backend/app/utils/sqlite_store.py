"""SQLite 存储层 — 会话历史冷数据，永久保留。

表结构：
- sessions: 会话元信息
- messages: 消息历史
- tool_calls: 工具调用记录
- approval_logs: 审批日志
"""

from __future__ import annotations

import json
import logging
from datetime import datetime, timezone
from typing import Optional

from app.models.chat import Message, Role
from app.models.session import SessionState

logger = logging.getLogger("minicc.sqlite")


CREATE_SESSIONS = """
CREATE TABLE IF NOT EXISTS sessions (
    session_id TEXT PRIMARY KEY,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    metadata TEXT DEFAULT '{}'
)
"""

CREATE_MESSAGES = """
CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL,
    model TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(session_id)
)
"""

CREATE_TOOL_CALLS = """
CREATE TABLE IF NOT EXISTS tool_calls (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    name TEXT NOT NULL,
    input TEXT,
    result TEXT,
    is_error INTEGER DEFAULT 0,
    duration_ms INTEGER,
    created_at TEXT NOT NULL
)
"""

CREATE_APPROVAL_LOGS = """
CREATE TABLE IF NOT EXISTS approval_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    action TEXT NOT NULL,
    created_at TEXT NOT NULL
)
"""


class SQLiteStore:
    """SQLite 历史记录存储。可选依赖 — 无写入能力时降级。"""

    def __init__(self, db_path: str = "minicc_history.db") -> None:
        self._path = db_path
        self._db = None

    async def connect(self) -> bool:
        try:
            import aiosqlite
            self._db = await aiosqlite.connect(self._path)
            await self._db.execute("PRAGMA journal_mode=WAL")
            await self._db.execute("PRAGMA foreign_keys=ON")
            await self._init_tables()
            logger.info("SQLite connected: %s", self._path)
            return True
        except Exception as exc:
            logger.warning("SQLite unavailable: %s", exc)
            return False

    async def disconnect(self) -> None:
        if self._db:
            await self._db.close()

    async def _init_tables(self) -> None:
        for ddl in [CREATE_SESSIONS, CREATE_MESSAGES, CREATE_TOOL_CALLS, CREATE_APPROVAL_LOGS]:
            await self._db.execute(ddl)
        await self._db.commit()

    # -- Sessions --

    async def save_session(self, session: SessionState) -> None:
        if not self._db:
            return
        await self._db.execute(
            "INSERT OR REPLACE INTO sessions (session_id, created_at, updated_at, metadata) VALUES (?, ?, ?, ?)",
            (session.session_id, session.created_at.isoformat(), session.updated_at.isoformat(),
             json.dumps(session.metadata)),
        )
        await self._db.commit()

    async def get_session(self, session_id: str) -> Optional[SessionState]:
        if not self._db:
            return None
        cursor = await self._db.execute("SELECT * FROM sessions WHERE session_id = ?", (session_id,))
        row = await cursor.fetchone()
        if not row:
            return None
        return SessionState(
            session_id=row[0],
            created_at=datetime.fromisoformat(row[1]),
            updated_at=datetime.fromisoformat(row[2]),
            metadata=json.loads(row[3]),
        )

    async def list_sessions(self, limit: int = 50) -> list[SessionState]:
        if not self._db:
            return []
        cursor = await self._db.execute(
            "SELECT * FROM sessions ORDER BY updated_at DESC LIMIT ?", (limit,)
        )
        rows = await cursor.fetchall()
        return [
            SessionState(
                session_id=r[0],
                created_at=datetime.fromisoformat(r[1]),
                updated_at=datetime.fromisoformat(r[2]),
                metadata=json.loads(r[3]),
            )
            for r in rows
        ]

    # -- Messages --

    async def save_message(self, session_id: str, message: Message) -> None:
        if not self._db:
            return
        content = message.content if isinstance(message.content, str) else json.dumps([c.model_dump() for c in message.content])
        await self._db.execute(
            "INSERT INTO messages (session_id, role, content, created_at, model) VALUES (?, ?, ?, ?, ?)",
            (session_id, message.role.value, content, message.created_at.isoformat(), message.model),
        )
        await self._db.commit()

    async def get_messages(self, session_id: str) -> list[Message]:
        if not self._db:
            return []
        cursor = await self._db.execute(
            "SELECT role, content, created_at, model FROM messages WHERE session_id = ? ORDER BY id",
            (session_id,),
        )
        rows = await cursor.fetchall()
        messages = []
        for row in rows:
            try:
                content = json.loads(row[1]) if row[1].startswith("[") else row[1]
            except (json.JSONDecodeError, IndexError):
                content = row[1]
            messages.append(Message(
                role=Role(row[0]),
                content=content,
                created_at=datetime.fromisoformat(row[2]),
                model=row[3],
            ))
        return messages

    # -- Approval logs --

    async def log_approval(self, session_id: str, tool_name: str, action: str) -> None:
        if not self._db:
            return
        await self._db.execute(
            "INSERT INTO approval_logs (session_id, tool_name, action, created_at) VALUES (?, ?, ?, ?)",
            (session_id, tool_name, action, datetime.now(timezone.utc).isoformat()),
        )
        await self._db.commit()
