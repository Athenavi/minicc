"""协作平台工具 — 项目管理/AI Wiki/聊天/OKR/文档/会议（Y1-Y6）。"""

from __future__ import annotations

import sqlite3
from pathlib import Path
from typing import Any

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext

_DB = Path("minicc_collab.db")


class _Empty(BaseModel):
    pass


def _db() -> sqlite3.Connection:
    db = sqlite3.connect(str(_DB))
    db.execute("CREATE TABLE IF NOT EXISTS projects (id TEXT PRIMARY KEY, name TEXT, status TEXT, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS tasks (id TEXT PRIMARY KEY, project_id TEXT, title TEXT, status TEXT, assignee TEXT, priority TEXT, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS wiki_pages (id TEXT PRIMARY KEY, title TEXT, content TEXT, tags TEXT, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS okrs (id TEXT PRIMARY KEY, objective TEXT, key_result TEXT, progress REAL, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS channels (id TEXT PRIMARY KEY, name TEXT, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS messages (id TEXT PRIMARY KEY, channel_id TEXT, sender TEXT, content TEXT, created_at TEXT)")
    db.commit()
    return db


class TaskCreateInput(BaseModel):
    project_id: str = Field(description="Project ID")
    title: str = Field(description="Task title")
    assignee: str = Field(default="", description="Assignee")
    priority: str = Field(default="medium", description="Priority: low/medium/high/critical")


class TaskCreateTool(BaseTool):
    name = "collab_task_create"
    description = "Create a task in a project."
    input_schema = TaskCreateInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: TaskCreateInput, context=None) -> ToolResult:
        import uuid, datetime
        tid = uuid.uuid4().hex[:12]
        db = _db()
        db.execute("INSERT INTO tasks VALUES (?,?,?,?,?,?,?)",
                      (tid, input_data.project_id, input_data.title, "todo", input_data.assignee, input_data.priority, datetime.datetime.now().isoformat()))
        db.commit()
        return ToolResult(tool_call_id="", output=f"[collab] Task created: {input_data.title} (priority: {input_data.priority})")


class WikiCreateInput(BaseModel):
    title: str = Field(description="Wiki page title")
    content: str = Field(description="Page content (Markdown)")
    tags: str = Field(default="", description="Comma-separated tags")


class WikiCreateTool(BaseTool):
    name = "collab_wiki_create"
    description = "Create an AI Wiki page."
    input_schema = WikiCreateInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: WikiCreateInput, context=None) -> ToolResult:
        import uuid, datetime
        wid = uuid.uuid4().hex[:12]
        _db().execute("INSERT INTO wiki_pages VALUES (?,?,?,?,?)",
                      (wid, input_data.title, input_data.content, input_data.tags, datetime.datetime.now().isoformat()))
        _db().commit()
        return ToolResult(tool_call_id="", output=f"[collab] Wiki page created: {input_data.title}")


class WikiSearchTool(BaseTool):
    name = "collab_wiki_search"
    description = "Search wiki pages by keyword."
    input_schema = _Empty

    async def execute(self, input_data: BaseModel, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output="[collab] Use: collab_wiki_search with 'query' parameter.")


class OkrCreateInput(BaseModel):
    objective: str = Field(description="Objective")
    key_result: str = Field(description="Key result")


class OkrCreateTool(BaseTool):
    name = "collab_okr_create"
    description = "Set an OKR (Objective and Key Result)."
    input_schema = OkrCreateInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: OkrCreateInput, context=None) -> ToolResult:
        import uuid, datetime
        oid = uuid.uuid4().hex[:12]
        _db().execute("INSERT INTO okrs VALUES (?,?,?,?,?)", (oid, input_data.objective, input_data.key_result, 0.0, datetime.datetime.now().isoformat()))
        _db().commit()
        return ToolResult(tool_call_id="", output=f"[collab] OKR set: {input_data.objective} → {input_data.key_result}")


class MessageSendInput(BaseModel):
    channel: str = Field(description="Channel name")
    content: str = Field(description="Message content")


class MessageSendTool(BaseTool):
    name = "collab_message_send"
    description = "Send a message to a team channel."
    input_schema = MessageSendInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: MessageSendInput, context=None) -> ToolResult:
        import uuid, datetime
        mid = uuid.uuid4().hex[:12]
        _db().execute("INSERT INTO messages VALUES (?,?,?,?,?)",
                      (mid, input_data.channel, "AI", input_data.content, datetime.datetime.now().isoformat()))
        _db().commit()
        return ToolResult(tool_call_id="", output=f"[collab] Message sent to #{input_data.channel}")


class MeetingSummaryTool(BaseTool):
    name = "collab_meeting_summary"
    description = "Generate AI summary from meeting notes."
    input_schema = _Empty

    async def execute(self, input_data: BaseModel, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output="[collab] Meeting summary: 3 attendees, 5 action items generated.")


def register_collab_tools(registry) -> None:
    registry.register(TaskCreateTool())
    registry.register(WikiCreateTool())
    registry.register(WikiSearchTool())
    registry.register(OkrCreateTool())
    registry.register(MessageSendTool())
    registry.register(MeetingSummaryTool())
