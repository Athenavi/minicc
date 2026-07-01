"""向量存储适配器 — Chroma（开发）/ PGVector（生产）。"""

from __future__ import annotations

import json
import sqlite3
import time
from pathlib import Path
from typing import Any, Optional

from app.rag.chunker import Chunk
from app.rag.embedder import BaseEmbedder, compute_hash

_DB_PATH = Path("minicc_vector.db")


def _get_db() -> sqlite3.Connection:
    db = sqlite3.connect(str(_DB_PATH))
    db.execute("""
        CREATE TABLE IF NOT EXISTS vectors (
            id TEXT PRIMARY KEY,
            chunk_hash TEXT,
            content TEXT,
            metadata TEXT,
            created_at REAL
        )
    """)
    db.execute("CREATE INDEX IF NOT EXISTS idx_chunk_hash ON vectors(chunk_hash)")
    db.commit()
    return db


class VectorStore:
    """向量存储 — 基于 SQLite + 余弦相似度（适用于开发环境）。"""

    def __init__(self, embedder: BaseEmbedder) -> None:
        self._embedder = embedder
        self._cache: dict[str, list[float]] = {}

    async def add_chunks(self, chunks: list[Chunk], namespace: str = "default") -> int:
        """添加分块到向量存储。"""
        db = _get_db()
        count = 0
        for chunk in chunks:
            h = compute_hash(chunk.content)
            existing = db.execute("SELECT id FROM vectors WHERE chunk_hash = ?", (h,)).fetchone()
            if existing:
                continue
            result = await self._embedder.embed(chunk.content)
            vec_id = f"{namespace}:{h}"
            db.execute(
                "INSERT INTO vectors (id, chunk_hash, content, metadata, created_at) VALUES (?, ?, ?, ?, ?)",
                (vec_id, h, chunk.content[:10000], json.dumps(chunk.metadata), time.time()),
            )
            self._cache[vec_id] = result.vector
            count += 1
        db.commit()
        return count

    async def search(self, query: str, top_k: int = 5, namespace: str = "") -> list[dict]:
        """搜索相似分块。"""
        query_vec = (await self._embedder.embed(query)).vector
        db = _get_db()

        rows = db.execute(
            "SELECT id, content, metadata FROM vectors WHERE id LIKE ?",
            (f"{namespace}:%" if namespace else "%",),
        ).fetchall()

        scored = []
        for row in rows:
            vec_id, content, meta_json = row
            vec = self._cache.get(vec_id)
            if vec is None:
                continue
            score = self._cosine_sim(query_vec, vec)
            scored.append((score, content, meta_json))

        scored.sort(key=lambda x: -x[0])
        return [
            {"score": round(s, 4), "content": c[:2000], "metadata": json.loads(m) if m else {}}
            for s, c, m in scored[:top_k]
        ]

    def count(self, namespace: str = "") -> int:
        db = _get_db()
        if namespace:
            return db.execute("SELECT COUNT(*) FROM vectors WHERE id LIKE ?", (f"{namespace}:%",)).fetchone()[0]
        return db.execute("SELECT COUNT(*) FROM vectors").fetchone()[0]

    def delete_namespace(self, namespace: str) -> None:
        db = _get_db()
        db.execute("DELETE FROM vectors WHERE id LIKE ?", (f"{namespace}:%",))
        db.commit()

    @staticmethod
    def _cosine_sim(a: list[float], b: list[float]) -> float:
        dot = sum(x * y for x, y in zip(a, b))
        na = sum(x * x for x in a) ** 0.5
        nb = sum(x * x for x in b) ** 0.5
        return dot / (na * nb) if na * nb > 0 else 0.0
