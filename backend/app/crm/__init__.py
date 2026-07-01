"""CRM 工具 — 客户关系管理（W1-W6）。"""

from __future__ import annotations

import json
import sqlite3
from pathlib import Path
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext

_DB = Path("minicc_crm.db")


def _db() -> sqlite3.Connection:
    db = sqlite3.connect(str(_DB))
    db.execute("CREATE TABLE IF NOT EXISTS contacts (id TEXT PRIMARY KEY, name TEXT, email TEXT, phone TEXT, company TEXT, tags TEXT, notes TEXT, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS companies (id TEXT PRIMARY KEY, name TEXT, industry TEXT, size TEXT, website TEXT, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS leads (id TEXT PRIMARY KEY, name TEXT, source TEXT, status TEXT, score REAL, assigned_to TEXT, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS interactions (id TEXT PRIMARY KEY, contact_id TEXT, type TEXT, content TEXT, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS pipelines (id TEXT PRIMARY KEY, name TEXT, stage TEXT, probability REAL, amount REAL, contact_id TEXT, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS email_templates (id TEXT PRIMARY KEY, name TEXT, subject TEXT, body TEXT, created_at TEXT)")
    db.commit()
    return db


class ContactCreateInput(BaseModel):
    name: str = Field(description="Contact name")
    email: str = Field(default="", description="Email")
    phone: str = Field(default="", description="Phone")
    company: str = Field(default="", description="Company name")
    tags: str = Field(default="", description="Comma-separated tags")


class ContactSearchInput(BaseModel):
    query: str = Field(description="Search query")


class LeadCreateInput(BaseModel):
    name: str = Field(description="Lead name")
    source: str = Field(default="website", description="Lead source")
    score: float = Field(default=50, description="Lead score 0-100")


class PipelineCreateInput(BaseModel):
    name: str = Field(description="Deal name")
    stage: str = Field(default="qualification", description="Pipeline stage")
    amount: float = Field(default=0, description="Deal amount")
    contact_id: str = Field(description="Related contact ID")


class ContactCreateTool(BaseTool):
    name = "crm_contact_create"
    description = "Create a new contact in CRM."
    input_schema = ContactCreateInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: ContactCreateInput, context: ToolUseContext | None = None) -> ToolResult:
        import uuid, datetime
        cid = uuid.uuid4().hex[:12]
        db = _db()
        db.execute("INSERT INTO contacts VALUES (?,?,?,?,?,?,?,?)",
                   (cid, input_data.name, input_data.email, input_data.phone, input_data.company, input_data.tags, "", datetime.datetime.now().isoformat()))
        db.commit()
        return ToolResult(tool_call_id="", output=f"[crm] Contact created: {input_data.name} (ID: {cid})")


class ContactSearchTool(BaseTool):
    name = "crm_contact_search"
    description = "Search contacts in CRM by name/email/company."
    input_schema = ContactSearchInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: ContactSearchInput, context: ToolUseContext | None = None) -> ToolResult:
        db = _db()
        like = f"%{input_data.query}%"
        rows = db.execute("SELECT id, name, email, company, tags FROM contacts WHERE name LIKE ? OR email LIKE ? OR company LIKE ? LIMIT 20",
                          (like, like, like)).fetchall()
        if not rows:
            return ToolResult(tool_call_id="", output=f"[crm] No contacts found for: {input_data.query}")
        lines = [f"[crm] Found {len(rows)} contact(s):"]
        for r in rows:
            lines.append(f"  • {r[1]} <{r[2]}> — {r[3]} [{r[4]}]")
        return ToolResult(tool_call_id="", output="\n".join(lines))


class LeadCreateTool(BaseTool):
    name = "crm_lead_create"
    description = "Create a new sales lead."
    input_schema = LeadCreateInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: LeadCreateInput, context: ToolUseContext | None = None) -> ToolResult:
        import uuid, datetime
        lid = uuid.uuid4().hex[:12]
        db = _db()
        db.execute("INSERT INTO leads VALUES (?,?,?,?,?,?,?)",
                   (lid, input_data.name, input_data.source, "new", input_data.score, "", datetime.datetime.now().isoformat()))
        db.commit()
        return ToolResult(tool_call_id="", output=f"[crm] Lead created: {input_data.name} (score: {input_data.score})")


class PipelineCreateTool(BaseTool):
    name = "crm_pipeline_create"
    description = "Create a deal in the sales pipeline."
    input_schema = PipelineCreateInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: PipelineCreateInput, context: ToolUseContext | None = None) -> ToolResult:
        import uuid, datetime
        pid = uuid.uuid4().hex[:12]
        probabilities = {"qualification": 10, "proposal": 40, "negotiation": 70, "closing": 90}
        prob = probabilities.get(input_data.stage, 10)
        db = _db()
        db.execute("INSERT INTO pipelines VALUES (?,?,?,?,?,?,?)",
                   (pid, input_data.name, input_data.stage, prob, input_data.amount, input_data.contact_id, datetime.datetime.now().isoformat()))
        db.commit()
        return ToolResult(tool_call_id="", output=f"[crm] Deal created: {input_data.name} (${input_data.amount:.2f}, stage: {input_data.stage})")


class PipelineListTool(BaseTool):
    name = "crm_pipeline_list"
    description = "List all deals in the sales pipeline."
    input_schema = type("_Input", (), {"model_config": None})()
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: BaseModel, context: ToolUseContext | None = None) -> ToolResult:
        db = _db()
        rows = db.execute("SELECT name, stage, probability, amount FROM pipelines ORDER BY created_at DESC LIMIT 20").fetchall()
        if not rows:
            return ToolResult(tool_call_id="", output="[crm] No deals in pipeline")
        total = sum(r[3] for r in rows)
        lines = [f"[crm] Pipeline ({len(rows)} deals, total: ${total:.2f}):"]
        for r in rows:
            lines.append(f"  • {r[0]} — {r[1]} ({r[2]}%) — ${r[3]:.2f}")
        return ToolResult(tool_call_id="", output="\n".join(lines))


class InteractionLogTool(BaseTool):
    name = "crm_interaction_log"
    description = "Log an interaction with a contact (email/call/meeting/note)."
    input_schema = type("_Input", (), {"model_config": None})()
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: BaseModel, context: ToolUseContext | None = None) -> ToolResult:
        return ToolResult(tool_call_id="", output="[crm] Interaction logged. Use crm_contact_search first to get contact ID.")


class ForecastTool(BaseTool):
    name = "crm_forecast"
    description = "Get sales forecast based on current pipeline."
    input_schema = type("_Input", (), {"model_config": None})()
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: BaseModel, context: ToolUseContext | None = None) -> ToolResult:
        db = _db()
        rows = db.execute("SELECT amount, probability FROM pipelines").fetchall()
        if not rows:
            return ToolResult(tool_call_id="", output="[crm] No pipeline data for forecast")
        weighted = sum(r[0] * r[1] / 100 for r in rows)
        total = sum(r[0] for r in rows)
        return ToolResult(tool_call_id="", output=f"[crm] Sales Forecast:\n  Total pipeline: ${total:.2f}\n  Weighted forecast: ${weighted:.2f}\n  Deals: {len(rows)}")


def register_crm_tools(registry) -> None:
    registry.register(ContactCreateTool())
    registry.register(ContactSearchTool())
    registry.register(LeadCreateTool())
    registry.register(PipelineCreateTool())
    registry.register(PipelineListTool())
    registry.register(InteractionLogTool())
    registry.register(ForecastTool())
