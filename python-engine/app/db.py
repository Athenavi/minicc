"""PostgreSQL connection pool for graph persistence."""
from __future__ import annotations

import logging
from typing import Any, Optional

import asyncpg

from app.config import settings

logger = logging.getLogger(__name__)

_pool: Optional[asyncpg.Pool] = None


async def init_pool(dsn: str) -> asyncpg.Pool:
    """Initialize the global connection pool."""
    global _pool
    _pool = await asyncpg.create_pool(dsn, min_size=settings.db_pool_min_size, max_size=settings.db_pool_max_size)
    logger.info("PostgreSQL connected (pool=%d-%d)", settings.db_pool_min_size, settings.db_pool_max_size)
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
    if _pool.is_closed():
        raise RuntimeError("PostgreSQL pool was closed")
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
            document_count INTEGER DEFAULT 0,
            total_size_bytes BIGINT DEFAULT 0,
            credits_consumed INTEGER DEFAULT 0,
            config JSONB DEFAULT '{}',
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )""", "knowledge_bases"),
        ("""CREATE INDEX IF NOT EXISTS idx_knowledge_bases_user ON knowledge_bases(user_id)""", "idx_knowledge_bases_user"),

        # ── knowledge_documents ──
        ("""CREATE TABLE IF NOT EXISTS knowledge_documents (
            id VARCHAR(32) PRIMARY KEY,
            knowledge_base_id VARCHAR(32) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
            user_id VARCHAR(64) DEFAULT '',
            name VARCHAR(255) NOT NULL,
            file_type VARCHAR(32) DEFAULT '',
            file_size_bytes INTEGER DEFAULT 0,
            file_url VARCHAR(1024),
            status VARCHAR(32) DEFAULT 'pending',
            chunk_count INTEGER DEFAULT 0,
            content BYTEA,
            error_message TEXT,
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )""", "knowledge_documents"),
        ("""CREATE INDEX IF NOT EXISTS idx_knowledge_documents_kb ON knowledge_documents(knowledge_base_id)""", "idx_knowledge_documents_kb"),

        # ── knowledge_chunk_vectors（pgvector 向量存储）──
        ("CREATE EXTENSION IF NOT EXISTS vector", "vector_extension"),
        # 表名与维度跟随配置（settings.pgvector_table / settings.embedding_dim），
        # 需与迁移文件 migrations/20260810000001_create_pgvector_tables.sql 保持一致
        (f"""CREATE TABLE IF NOT EXISTS {settings.pgvector_table} (
            id VARCHAR(64) PRIMARY KEY,
            knowledge_base_id VARCHAR(32) NOT NULL,
            document_id VARCHAR(32) NOT NULL,
            tenant_id VARCHAR(32) NOT NULL,
            chunk_index INT NOT NULL,
            content TEXT NOT NULL,
            embedding vector({settings.embedding_dim}) NOT NULL,
            created_at TIMESTAMPTZ DEFAULT NOW()
        )""", "knowledge_chunk_vectors"),
        (f"""CREATE INDEX IF NOT EXISTS idx_kchunk_vectors_kb ON {settings.pgvector_table}(knowledge_base_id)""", "idx_kchunk_vectors_kb"),
        (f"""CREATE INDEX IF NOT EXISTS idx_kchunk_vectors_doc ON {settings.pgvector_table}(document_id)""", "idx_kchunk_vectors_doc"),
        (f"""CREATE INDEX IF NOT EXISTS idx_kchunk_vectors_tenant ON {settings.pgvector_table}(tenant_id)""", "idx_kchunk_vectors_tenant"),
        (f"""CREATE INDEX IF NOT EXISTS idx_kchunk_vectors_embedding
            ON {settings.pgvector_table} USING hnsw (embedding vector_cosine_ops)""", "idx_kchunk_vectors_embedding"),

        # ── unified_sessions / unified_messages（统一任务会话持久化，六大工作台统一入口）──
        ("""CREATE TABLE IF NOT EXISTS unified_sessions (
            id TEXT PRIMARY KEY,
            tenant_id TEXT NOT NULL,
            user_id TEXT NOT NULL,
            title TEXT DEFAULT '',
            mode TEXT DEFAULT 'auto',
            shared_context JSONB DEFAULT '{}',
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW()
        )""", "unified_sessions"),
        ("""CREATE INDEX IF NOT EXISTS idx_unified_sessions_lookup
            ON unified_sessions(tenant_id, user_id, updated_at DESC)""", "idx_unified_sessions_lookup"),
        ("""CREATE TABLE IF NOT EXISTS unified_messages (
            id BIGSERIAL PRIMARY KEY,
            session_id TEXT NOT NULL REFERENCES unified_sessions(id) ON DELETE CASCADE,
            role TEXT NOT NULL,
            content TEXT NOT NULL,
            metadata JSONB DEFAULT '{}',
            error TEXT DEFAULT '',
            created_at TIMESTAMPTZ DEFAULT NOW()
        )""", "unified_messages"),
        ("""CREATE INDEX IF NOT EXISTS idx_unified_messages_session
            ON unified_messages(session_id, created_at)""", "idx_unified_messages_session"),
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
        # knowledge_bases: 兼容旧版本可能缺少的列（列名以迁移文件/代码为准）
        "ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS document_count INTEGER DEFAULT 0",
        "ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS total_size_bytes BIGINT DEFAULT 0",
        "ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS credits_consumed INTEGER DEFAULT 0",
        "ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS config JSONB DEFAULT '{}'",
        # knowledge_documents: 兼容旧版本可能缺少的列（旧建表误用 kb_id，此处补正确列名并回填）
        "ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS knowledge_base_id VARCHAR(32)",
        "ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS file_url VARCHAR(1024)",
        "ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS chunk_count INTEGER DEFAULT 0",
        # 回填：旧 schema 用 kb_id 列，升级后迁移到正确列名
        "UPDATE knowledge_documents SET knowledge_base_id = kb_id WHERE knowledge_base_id IS NULL AND kb_id IS NOT NULL",
        # knowledge_documents: 文档内容（异步 RAG 构建时 worker 读取）
        "ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS content BYTEA",
        "ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS error_message TEXT",
        # knowledge_documents: 租户隔离列 + 索引
        "ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(32) NOT NULL DEFAULT 'default'",
        # ── 三方登录（与 migrations/20260821000001_oauth_login.up.sql 双轨同步）──
        # ent_oidc_providers: OAuth2 协议 + 预设模板扩展列
        "ALTER TABLE ent_oidc_providers ADD COLUMN IF NOT EXISTS protocol VARCHAR(16) NOT NULL DEFAULT 'oidc'",
        "ALTER TABLE ent_oidc_providers ADD COLUMN IF NOT EXISTS provider_type VARCHAR(32) NOT NULL DEFAULT 'custom'",
        "ALTER TABLE ent_oidc_providers ADD COLUMN IF NOT EXISTS display_name VARCHAR(64)",
        "ALTER TABLE ent_oidc_providers ADD COLUMN IF NOT EXISTS icon VARCHAR(64)",
        "ALTER TABLE ent_oidc_providers ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 100",
        "ALTER TABLE ent_oidc_providers ADD COLUMN IF NOT EXISTS auth_url VARCHAR(512)",
        "ALTER TABLE ent_oidc_providers ADD COLUMN IF NOT EXISTS token_url VARCHAR(512)",
        "ALTER TABLE ent_oidc_providers ADD COLUMN IF NOT EXISTS userinfo_url VARCHAR(512)",
        "ALTER TABLE ent_oidc_providers ADD COLUMN IF NOT EXISTS extra JSONB NOT NULL DEFAULT '{}'",
        # users: 手机号 + 密码可用性标识（SSO 建号用户 password_set=false）
        "ALTER TABLE users ADD COLUMN IF NOT EXISTS phone VARCHAR(32)",
        "ALTER TABLE users ADD COLUMN IF NOT EXISTS password_set BOOLEAN NOT NULL DEFAULT TRUE",
        # 人机验证配置表（幂等建表，主迁移在 Go 侧 SQL 文件）
        """CREATE TABLE IF NOT EXISTS ent_captcha_config (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            tenant_id UUID NOT NULL REFERENCES tenants(id),
            provider VARCHAR(32) NOT NULL DEFAULT 'turnstile',
            site_key VARCHAR(256) NOT NULL DEFAULT '',
            secret_enc TEXT NOT NULL DEFAULT '',
            verify_url VARCHAR(512),
            enabled BOOLEAN NOT NULL DEFAULT FALSE,
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW(),
            UNIQUE(tenant_id)
        )""",
        # ── 用户记忆档案卡（L2 层，与 migrations/20260823000006_user_memory_profile 双轨同步）──
        """CREATE TABLE IF NOT EXISTS user_memory_profile (
            tenant_id   VARCHAR(64)  NOT NULL,
            user_id     VARCHAR(64)  NOT NULL,
            slot        VARCHAR(32)  NOT NULL,
            item_key    VARCHAR(128) NOT NULL,
            item_value  JSONB        NOT NULL,
            confidence  SMALLINT     NOT NULL DEFAULT 50,
            source      VARCHAR(16)  NOT NULL DEFAULT 'derived',
            version     INTEGER      NOT NULL DEFAULT 1,
            confirmed_at TIMESTAMPTZ,
            last_referenced_at TIMESTAMPTZ,
            created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
            updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
            PRIMARY KEY (tenant_id, user_id, slot, item_key)
        )""",
        "CREATE INDEX IF NOT EXISTS idx_ump_reference ON user_memory_profile(tenant_id, user_id, last_referenced_at)",
        "CREATE INDEX IF NOT EXISTS idx_ump_conflict ON user_memory_profile(tenant_id, user_id, source) WHERE source = 'user_confirmed'",
        # ── 用户长期记忆条目（L2 档案卡，与 migrations/20260821000003 双轨同步）──
        """CREATE TABLE IF NOT EXISTS user_memory_entries (
            id VARCHAR(64) PRIMARY KEY,
            tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
            user_id VARCHAR(64) NOT NULL,
            slot VARCHAR(32) NOT NULL,
            item_key VARCHAR(128) NOT NULL,
            item_value TEXT NOT NULL,
            confidence SMALLINT NOT NULL DEFAULT 50,
            source VARCHAR(16) NOT NULL DEFAULT 'derived',
            embedding JSONB,
            access_count INTEGER NOT NULL DEFAULT 0,
            last_accessed_at TIMESTAMPTZ,
            status VARCHAR(16) NOT NULL DEFAULT 'active',
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            UNIQUE (tenant_id, user_id, slot, item_key)
        )""",
        "CREATE INDEX IF NOT EXISTS idx_ume_lookup ON user_memory_entries(tenant_id, user_id, status)",
        "CREATE INDEX IF NOT EXISTS idx_ume_access ON user_memory_entries(tenant_id, user_id, last_accessed_at)",
        # ── L3 近期对话摘要（与 migrations/20260821000004 双轨同步）──
        """CREATE TABLE IF NOT EXISTS memory_summaries (
            id              VARCHAR(64) PRIMARY KEY,
            tenant_id       VARCHAR(64) NOT NULL,
            user_id         VARCHAR(64) NOT NULL,
            session_id      VARCHAR(64) NOT NULL,
            content         TEXT NOT NULL,
            topics          JSONB NOT NULL DEFAULT '[]',
            entities        JSONB NOT NULL DEFAULT '{}',
            turn_start      INTEGER NOT NULL DEFAULT 0,
            turn_end        INTEGER NOT NULL DEFAULT 0,
            content_hash    VARCHAR(80) NOT NULL,
            access_count    INTEGER NOT NULL DEFAULT 0,
            last_accessed_at TIMESTAMPTZ,
            status          VARCHAR(16) NOT NULL DEFAULT 'active',
            created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )""",
        "CREATE INDEX IF NOT EXISTS idx_ms_lookup ON memory_summaries(tenant_id, user_id, status, created_at DESC)",
        "CREATE UNIQUE INDEX IF NOT EXISTS uq_ms_hash ON memory_summaries(tenant_id, user_id, content_hash)",
    ]
    for sql in migrations:
        try:
            await pool.execute(sql)
        except Exception as e:
            logger.debug("Migration skipped (table may not exist yet): %s", e)

    logger.info("All tables ensured")
