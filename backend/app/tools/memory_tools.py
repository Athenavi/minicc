"""并行编辑冲突解决 + 长期项目记忆。"""

from __future__ import annotations

import json
import sqlite3
from pathlib import Path
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext

_MEMORY_DB = Path.cwd() / ".minicc" / "project_memory.db"


def _get_db() -> sqlite3.Connection:
    db = sqlite3.connect(str(_MEMORY_DB))
    db.execute("""
        CREATE TABLE IF NOT EXISTS memories (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            key TEXT UNIQUE,
            content TEXT,
            category TEXT,
            created_at TEXT DEFAULT (datetime('now')),
            updated_at TEXT DEFAULT (datetime('now'))
        )
    """)
    db.execute("""
        CREATE TABLE IF NOT EXISTS conflicts (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            file_path TEXT,
            agent_a TEXT,
            agent_b TEXT,
            version_a TEXT,
            version_b TEXT,
            resolved TEXT DEFAULT 'pending',
            resolution TEXT,
            created_at TEXT DEFAULT (datetime('now'))
        )
    """)
    db.commit()
    return db


class MemorySetInput(BaseModel):
    key: str = Field(description="Memory key/name")
    content: str = Field(description="Memory content")
    category: str = Field(default="general", description="Memory category")


class MemoryGetInput(BaseModel):
    key: str = Field(description="Memory key to retrieve")


class MemorySearchInput(BaseModel):
    query: str = Field(description="Search query")
    category: Optional[str] = Field(default=None, description="Category filter")


class MemorySetTool(BaseTool):
    """长期记忆 — 写入。"""
    name = "memory_set"
    description = "Store a fact in long-term project memory. Survives across sessions."
    input_schema = MemorySetInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: MemorySetInput, context: ToolUseContext | None = None) -> ToolResult:
        db = _get_db()
        db.execute(
            "INSERT OR REPLACE INTO memories (key, content, category, updated_at) VALUES (?, ?, ?, datetime('now'))",
            (input_data.key, input_data.content, input_data.category),
        )
        db.commit()
        return ToolResult(tool_call_id="", output=f"[memory] Stored: {input_data.key} ({input_data.category})")


class MemoryGetTool(BaseTool):
    """长期记忆 — 读取。"""
    name = "memory_get"
    description = "Retrieve a specific fact from long-term project memory."
    input_schema = MemoryGetInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: MemoryGetInput, context: ToolUseContext | None = None) -> ToolResult:
        db = _get_db()
        row = db.execute("SELECT content, category, updated_at FROM memories WHERE key = ?", (input_data.key,)).fetchone()
        if row:
            return ToolResult(tool_call_id="", output=f"[memory] {input_data.key} ({row[1]}):\n{row[0][:2000]}")
        return ToolResult(tool_call_id="", output=f"[memory] Not found: {input_data.key}", is_error=True)


class MemorySearchTool(BaseTool):
    """长期记忆 — 搜索。"""
    name = "memory_search"
    description = "Search long-term project memory by keyword."
    input_schema = MemorySearchInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: MemorySearchInput, context: ToolUseContext | None = None) -> ToolResult:
        db = _get_db()
        like = f"%{input_data.query}%"
        if input_data.category:
            rows = db.execute(
                "SELECT key, content, category FROM memories WHERE (key LIKE ? OR content LIKE ?) AND category = ? LIMIT 10",
                (like, like, input_data.category),
            ).fetchall()
        else:
            rows = db.execute(
                "SELECT key, content, category FROM memories WHERE key LIKE ? OR content LIKE ? LIMIT 10",
                (like, like),
            ).fetchall()

        if not rows:
            return ToolResult(tool_call_id="", output=f"[memory] No results for: {input_data.query}")

        lines = [f"[memory] Found {len(rows)} result(s):"]
        for r in rows:
            preview = r[1][:100].replace("\n", " ")
            lines.append(f"  • {r[0]} ({r[2]}): {preview}")
        return ToolResult(tool_call_id="", output="\n".join(lines))


class ResolveConflictInput(BaseModel):
    file_path: str = Field(description="File with conflict")
    resolution: str = Field(description="Resolution content")
    agent_ids: list[str] = Field(description="Conflicting agent IDs")


class ConflictResolveTool(BaseTool):
    """冲突解决。"""
    name = "conflict_resolve"
    description = "Resolve a parallel edit conflict between multiple agents."
    input_schema = ResolveConflictInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: ResolveConflictInput, context: ToolUseContext | None = None) -> ToolResult:
        db = _get_db()
        db.execute(
            "INSERT INTO conflicts (file_path, agent_a, agent_b, version_a, version_b, resolved, resolution) VALUES (?, ?, ?, ?, ?, 'resolved', ?)",
            (input_data.file_path,
             input_data.agent_ids[0] if len(input_data.agent_ids) > 0 else "",
             input_data.agent_ids[1] if len(input_data.agent_ids) > 1 else "",
             "", "",
             input_data.resolution[:500]),
        )
        db.commit()

        # Write resolution to file
        safe_path = Path.cwd() / input_data.file_path
        safe_path.parent.mkdir(parents=True, exist_ok=True)
        safe_path.write_text(input_data.resolution, encoding="utf-8")

        return ToolResult(
            tool_call_id="",
            output=f"[conflict] Resolved conflict in {input_data.file_path} ({len(input_data.resolution)} chars written)",
        )


def register_memory_tools(registry) -> None:
    registry.register(MemorySetTool())
    registry.register(MemoryGetTool())
    registry.register(MemorySearchTool())
    registry.register(ConflictResolveTool())
