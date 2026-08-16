# db.py ensure_tables 测试 — pgvector DDL 跟随配置
from unittest.mock import AsyncMock, patch

import pytest

from app.config import settings


async def test_ensure_tables_creates_pgvector_ddl_from_config():
    """knowledge_chunk_vectors 的表名与维度必须跟随 settings，否则存取目标不一致"""
    from app.db import ensure_tables

    pool = AsyncMock()
    with patch("app.db.get_pool", return_value=pool):
        await ensure_tables()

    sqls = [c.args[0] for c in pool.execute.await_args_list]

    # pgvector 扩展
    assert any("CREATE EXTENSION IF NOT EXISTS vector" in s for s in sqls)
    # 表名跟随 settings.pgvector_table
    assert any(f"CREATE TABLE IF NOT EXISTS {settings.pgvector_table}" in s for s in sqls)
    # 维度跟随 settings.embedding_dim（与 builder 存储侧校验一致）
    assert any(f"embedding vector({settings.embedding_dim})" in s for s in sqls)
    # HNSW 余弦索引
    assert any("vector_cosine_ops" in s for s in sqls)
