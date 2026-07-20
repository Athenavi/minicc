"""PostgreSQL connection pool for graph persistence."""
from __future__ import annotations

import logging
from typing import Any, Optional

import asyncpg

logger = logging.getLogger(__name__)

_pool: Optional[asyncpg.Pool] = None


async def init_pool(dsn: str) -> asyncpg.Pool:
    """Initialize the global connection pool."""
    global _pool
    _pool = await asyncpg.create_pool(dsn, min_size=5, max_size=20)
    logger.info("PostgreSQL connected (pool=5-20)")
    return _pool


async def close_pool():
    """Close the global connection pool."""
    global _pool
    if _pool:
        await _pool.close()
        _pool = None


def get_pool() -> asyncpg.Pool:
    """Get the global connection pool."""
    if _pool is None:
        raise RuntimeError("PostgreSQL pool not initialized")
    return _pool


async def ensure_tables():
    """Create all required tables if they don't exist. Each table is attempted independently."""
    pool = get_pool()
    tables = [
        # ── workflow_graphs ──
        ("""CREATE TABLE IF NOT EXISTS workflow_graphs (
            id VARCHAR(32) PRIMARY KEY,
            name VARCHAR(255) NOT NULL,
            user_id VARCHAR(64) DEFAULT '',
            graph_json JSONB NOT NULL DEFAULT '{}',
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )""", "workflow_graphs"),
        ("""CREATE INDEX IF NOT EXISTS idx_workflow_graphs_user ON workflow_graphs(user_id)""", "idx_workflow_graphs_user"),
        ("""CREATE INDEX IF NOT EXISTS idx_workflow_graphs_updated ON workflow_graphs(updated_at)""", "idx_workflow_graphs_updated"),

        # ── knowledge_bases ──
        ("""CREATE TABLE IF NOT EXISTS knowledge_bases (
            id VARCHAR(32) PRIMARY KEY,
            user_id VARCHAR(64) DEFAULT '',
            name VARCHAR(255) NOT NULL,
            description TEXT DEFAULT '',
            type VARCHAR(32) DEFAULT 'wiki',
            visibility VARCHAR(32) DEFAULT 'private',
            status VARCHAR(32) DEFAULT 'draft',
            doc_count INTEGER DEFAULT 0,
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )""", "knowledge_bases"),
        ("""CREATE INDEX IF NOT EXISTS idx_knowledge_bases_user ON knowledge_bases(user_id)""", "idx_knowledge_bases_user"),

        # ── knowledge_documents ──
        ("""CREATE TABLE IF NOT EXISTS knowledge_documents (
            id VARCHAR(32) PRIMARY KEY,
            kb_id VARCHAR(32) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
            user_id VARCHAR(64) DEFAULT '',
            name VARCHAR(255) NOT NULL,
            file_type VARCHAR(32) DEFAULT '',
            file_size_bytes INTEGER DEFAULT 0,
            status VARCHAR(32) DEFAULT 'pending',
            chunk_count INTEGER DEFAULT 0,
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )""", "knowledge_documents"),
        ("""CREATE INDEX IF NOT EXISTS idx_knowledge_documents_kb ON knowledge_documents(kb_id)""", "idx_knowledge_documents_kb"),
    ]

    for sql, name in tables:
        try:
            await pool.execute(sql)
        except Exception as e:
            logger.warning("Table/index creation failed for %s: %s (continuing)", name, e)

    # ── 迁移：兼容已有表的不同 schema ──
    migrations = [
        # workflow_graphs: user_id 可能被旧版本创建为 UUID 类型
        "ALTER TABLE workflow_graphs ALTER COLUMN user_id TYPE VARCHAR(64) USING user_id::VARCHAR",
        # knowledge_bases: 兼容旧版本可能缺少的列
        "ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS doc_count INTEGER DEFAULT 0",
        # knowledge_documents: 兼容旧版本可能缺少的列
        "ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS chunk_count INTEGER DEFAULT 0",
    ]
    for sql in migrations:
        try:
            await pool.execute(sql)
        except Exception as e:
            logger.debug("Migration skipped (table may not exist yet): %s", e)

    logger.info("All tables ensured")
