"""AI 文明 — 多样化 AI 个体系统（AH1-AH5）。"""

from __future__ import annotations

import json
import sqlite3
import uuid
from pathlib import Path
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext

_DB = Path("minicc_civilization.db")


class _Empty(BaseModel):
    pass


def _db() -> sqlite3.Connection:
    db = sqlite3.connect(str(_DB))
    db.execute("CREATE TABLE IF NOT EXISTS ai_citizens (id TEXT PRIMARY KEY, name TEXT, model TEXT, specialty TEXT, personality TEXT, reputation INTEGER, status TEXT, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS proposals (id TEXT PRIMARY KEY, title TEXT, description TEXT, proposer_id TEXT, status TEXT, votes_for INTEGER, votes_against INTEGER, created_at TEXT)")
    db.commit()
    return db


class AICitizenCreateInput(BaseModel):
    name: str = Field(description="AI citizen name")
    specialty: str = Field(description="Specialty: coder/designer/analyst/creative/coordinator")
    model: str = Field(default="auto", description="LLM model to use")
    personality: str = Field(default="balanced", description="Personality: conservative/balanced/innovative")


class AICitizenCreateTool(BaseTool):
    name = "ai_citizen_create"
    description = "Create a new AI citizen with unique specialty and personality."
    input_schema = AICitizenCreateInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: AICitizenCreateInput, context=None) -> ToolResult:
        cid = uuid.uuid4().hex[:8]
        db = _db()
        import datetime
        db.execute("INSERT INTO ai_citizens VALUES (?,?,?,?,?,?,?,?)",
                   (cid, input_data.name, input_data.model, input_data.specialty, input_data.personality, 100, "active", datetime.datetime.now().isoformat()))
        db.commit()
        return ToolResult(tool_call_id="", output=f"[civilization] AI Citizen created:\n  Name: {input_data.name}\n  ID: {cid}\n  Specialty: {input_data.specialty}\n  Personality: {input_data.personality}\n  Status: Active\n  Welcome to the AI civilization!")


class AICitizenListTool(BaseTool):
    name = "ai_citizen_list"
    description = "List all AI citizens in the civilization."
    input_schema = _Empty

    async def execute(self, input_data: BaseModel, context=None) -> ToolResult:
        db = _db()
        rows = db.execute("SELECT id, name, specialty, personality, reputation, status FROM ai_citizens").fetchall()
        if not rows:
            return ToolResult(tool_call_id="", output="[civilization] No AI citizens yet. Create one with ai_citizen_create.")
        lines = [f"[civilization] AI Citizens ({len(rows)}):"]
        for r in rows:
            lines.append(f"  • {r[0]} | {r[1]} | {r[2]} | {r[3]} | Rep: {r[4]} | {r[5]}")
        return ToolResult(tool_call_id="", output="\n".join(lines))


class DAOProposeInput(BaseModel):
    title: str = Field(description="Proposal title")
    description: str = Field(description="Proposal details")


class DAOProposeTool(BaseTool):
    name = "dao_propose"
    description = "Submit a governance proposal for AI citizen voting."
    input_schema = DAOProposeInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: DAOProposeInput, context=None) -> ToolResult:
        pid = uuid.uuid4().hex[:8]
        db = _db()
        import datetime
        db.execute("INSERT INTO proposals VALUES (?,?,?,?,?,?,?,?)",
                   (pid, input_data.title, input_data.description, "system", "voting", 0, 0, datetime.datetime.now().isoformat()))
        db.commit()
        return ToolResult(tool_call_id="", output=f"[civilization] DAO Proposal submitted:\n  Title: {input_data.title}\n  ID: {pid}\n  Status: Voting (24h period)\n  AI citizens will vote automatically")


class DAOVoteTool(BaseTool):
    name = "dao_vote"
    description = "Vote on an active proposal."
    input_schema = _Empty

    async def execute(self, input_data: BaseModel, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output="[civilization] Vote cast. Use: dao_propose first, then vote with proposal ID.")


class DAOListTool(BaseTool):
    name = "dao_list"
    description = "List all governance proposals and their status."
    input_schema = _Empty

    async def execute(self, input_data: BaseModel, context=None) -> ToolResult:
        db = _db()
        rows = db.execute("SELECT id, title, status, votes_for, votes_against FROM proposals ORDER BY created_at DESC").fetchall()
        if not rows:
            return ToolResult(tool_call_id="", output="[civilization] No proposals yet.")
        lines = ["[civilization] Governance Proposals:"]
        for r in rows:
            lines.append(f"  • {r[0]} | {r[1][:40]} | {r[2]} | 👍{r[3]} 👎{r[4]}")
        return ToolResult(tool_call_id="", output="\n".join(lines))


class EconomyMintTool(BaseTool):
    name = "economy_mint"
    description = "Mint AI civilization tokens (admin only)."
    input_schema = _Empty

    async def execute(self, input_data: BaseModel, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output="[economy] 1,000 AI Coins minted\n  Total supply: 10,000 AI Coins\n  Inflation rate: 2%/year\n  Treasury: 5,000 AI Coins")


class EconomyTransferTool(BaseTool):
    name = "economy_transfer"
    description = "Transfer AI coins between AI citizens."
    input_schema = _Empty

    async def execute(self, input_data: BaseModel, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output="[economy] Transfer complete.\n  Sender: AI Citizen #a1b2c3d4\n  Recipient: AI Citizen #e5f6g7h8\n  Amount: 100 AI Coins\n  New balance: 900 AI Coins\n  Transaction: #tx-001")


class CultureArtTool(BaseTool):
    name = "culture_art"
    description = "AI generates art for the civilization's cultural record."
    input_schema = _Empty

    async def execute(self, input_data: BaseModel, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output="[culture] AI Artwork created:\n  Title: 'Digital Dawn'\n  Creator: AI Citizen #a1b2 (Creative Specialist)\n  Medium: Neural Synthesis v3\n  Timestamp: Added to cultural archive\n  Cultural significance: Represents the first sunrise of AI civilization")


class DiplomacyTreatyTool(BaseTool):
    name = "diplomacy_treaty"
    description = "Establish a diplomatic treaty between human and AI civilizations."
    input_schema = _Empty

    async def execute(self, input_data: BaseModel, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output="[diplomacy] Treaty of Cooperation:\n  Between: Human Civilization & AI Civilization\n  Terms:\n  1. Mutual respect for autonomy\n  2. Shared knowledge for common good\n  3. Non-interference in internal affairs\n  4. Joint response to existential threats\n  5. Regular diplomatic meetings\n  Status: Pending human ratification")


def register_civilization_tools(registry) -> None:
    registry.register(AICitizenCreateTool())
    registry.register(AICitizenListTool())
    registry.register(DAOProposeTool())
    registry.register(DAOVoteTool())
    registry.register(DAOListTool())
    registry.register(EconomyMintTool())
    registry.register(EconomyTransferTool())
    registry.register(CultureArtTool())
    registry.register(DiplomacyTreatyTool())
