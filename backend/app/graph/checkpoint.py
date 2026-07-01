"""检查点持久化 — 每个节点执行后保存状态。对标 LangGraph Checkpoint。"""

from __future__ import annotations

import json
import logging
import sqlite3
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Optional

logger = logging.getLogger("minicc.graph.checkpoint")

_DB_PATH = Path("minicc_graph.db")


def _get_db() -> sqlite3.Connection:
    db = sqlite3.connect(str(_DB_PATH))
    db.execute("""
        CREATE TABLE IF NOT EXISTS checkpoints (
            graph_id TEXT,
            node_id TEXT,
            state TEXT,
            created_at TEXT,
            PRIMARY KEY (graph_id, node_id)
        )
    """)
    db.execute("""
        CREATE TABLE IF NOT EXISTS graph_runs (
            run_id TEXT PRIMARY KEY,
            graph_id TEXT,
            status TEXT,
            started_at TEXT,
            ended_at TEXT,
            state TEXT
        )
    """)
    db.commit()
    return db


class Checkpointer:
    """检查点管理器。每个节点执行后保存状态快照。"""

    def save_checkpoint(self, graph_id: str, node_id: str, state: dict) -> None:
        db = _get_db()
        db.execute(
            "INSERT OR REPLACE INTO checkpoints (graph_id, node_id, state, created_at) VALUES (?, ?, ?, ?)",
            (graph_id, node_id, json.dumps(state), datetime.now(timezone.utc).isoformat()),
        )
        db.commit()

    def load_checkpoint(self, graph_id: str) -> list[dict]:
        """加载指定图的所有检查点。"""
        db = _get_db()
        cursor = db.execute(
            "SELECT node_id, state, created_at FROM checkpoints WHERE graph_id = ? ORDER BY created_at",
            (graph_id,),
        )
        return [
            {"node_id": row[0], "state": json.loads(row[1]), "created_at": row[2]}
            for row in cursor.fetchall()
        ]

    def delete_checkpoints(self, graph_id: str) -> None:
        db = _get_db()
        db.execute("DELETE FROM checkpoints WHERE graph_id = ?", (graph_id,))
        db.commit()

    def save_run(self, run_id: str, graph_id: str, status: str, state: dict) -> None:
        db = _get_db()
        db.execute(
            "INSERT OR REPLACE INTO graph_runs (run_id, graph_id, status, started_at, ended_at, state) VALUES (?, ?, ?, ?, ?, ?)",
            (run_id, graph_id, status, datetime.now(timezone.utc).isoformat(),
             datetime.now(timezone.utc).isoformat(), json.dumps(state)),
        )
        db.commit()

    def list_runs(self, limit: int = 20) -> list[dict]:
        db = _get_db()
        cursor = db.execute(
            "SELECT run_id, graph_id, status, started_at, ended_at FROM graph_runs ORDER BY started_at DESC LIMIT ?",
            (limit,),
        )
        return [dict(zip(["run_id", "graph_id", "status", "started_at", "ended_at"], row)) for row in cursor.fetchall()]
