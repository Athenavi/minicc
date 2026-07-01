"""多租户隔离 + 工作区管理。对标 Dify 工作区。"""

from __future__ import annotations

import json
import sqlite3
import uuid
from pathlib import Path
from typing import Optional

from pydantic import BaseModel, Field

_DB_PATH = Path("minicc_platform.db")


def _get_db() -> sqlite3.Connection:
    db = sqlite3.connect(str(_DB_PATH))
    db.execute("""
        CREATE TABLE IF NOT EXISTS workspaces (
            id TEXT PRIMARY KEY,
            name TEXT,
            owner_id TEXT,
            settings TEXT,
            created_at TEXT
        )
    """)
    db.execute("""
        CREATE TABLE IF NOT EXISTS workspace_members (
            workspace_id TEXT,
            user_id TEXT,
            role TEXT,
            PRIMARY KEY (workspace_id, user_id)
        )
    """)
    db.execute("""
        CREATE TABLE IF NOT EXISTS audit_log (
            id TEXT PRIMARY KEY,
            workspace_id TEXT,
            user_id TEXT,
            action TEXT,
            details TEXT,
            created_at TEXT
        )
    """)
    db.commit()
    return db


class WorkspaceManager:
    """工作区管理 — 多租户隔离。"""

    def create_workspace(self, name: str, owner_id: str) -> str:
        db = _get_db()
        wid = uuid.uuid4().hex[:12]
        db.execute(
            "INSERT INTO workspaces (id, name, owner_id, settings, created_at) VALUES (?, ?, ?, ?, datetime('now'))",
            (wid, name, owner_id, "{}"),
        )
        db.execute("INSERT INTO workspace_members (workspace_id, user_id, role) VALUES (?, ?, 'owner')", (wid, owner_id))
        db.commit()
        return wid

    def list_workspaces(self, user_id: str) -> list[dict]:
        db = _get_db()
        cursor = db.execute("""
            SELECT w.id, w.name, wm.role FROM workspaces w
            JOIN workspace_members wm ON w.id = wm.workspace_id
            WHERE wm.user_id = ?
        """, (user_id,))
        return [dict(zip(["id", "name", "role"], row)) for row in cursor.fetchall()]

    def add_member(self, workspace_id: str, user_id: str, role: str = "member") -> None:
        db = _get_db()
        db.execute("INSERT OR REPLACE INTO workspace_members VALUES (?, ?, ?)", (workspace_id, user_id, role))
        db.commit()

    def log_action(self, workspace_id: str, user_id: str, action: str, details: dict | None = None) -> None:
        db = _get_db()
        db.execute(
            "INSERT INTO audit_log (id, workspace_id, user_id, action, details, created_at) VALUES (?, ?, ?, ?, ?, datetime('now'))",
            (uuid.uuid4().hex[:12], workspace_id, user_id, action, json.dumps(details or {})),
        )
        db.commit()

    def get_recent_logs(self, workspace_id: str, limit: int = 50) -> list[dict]:
        db = _get_db()
        cursor = db.execute(
            "SELECT id, user_id, action, details, created_at FROM audit_log WHERE workspace_id = ? ORDER BY created_at DESC LIMIT ?",
            (workspace_id, limit),
        )
        return [dict(zip(["id", "user_id", "action", "details", "created_at"], row)) for row in cursor.fetchall()]


workspace_manager = WorkspaceManager()
