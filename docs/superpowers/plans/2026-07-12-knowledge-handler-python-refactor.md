# Knowledge Handler Refactoring Plan: Go → Python

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move knowledge base business logic from Go (`knowledge_handler.go`) to Python (`python-engine/app/api/knowledge.py`), making Go a pure proxy gateway.

**Architecture:** Go gateway forwards `/v1/kb/*` requests to Python engine via `PythonClient`. Python handles all DB operations using `asyncpg` with proper transaction isolation. This eliminates the `SET LOCAL` without transaction bug and aligns with the project's stated architecture (Go = proxy, Python = data plane).

**Tech Stack:** Python 3.11+, FastAPI, asyncpg, Pydantic v2

## Global Constraints

- Follow existing Python API patterns in `python-engine/app/api/workflows.py`
- Use `get_pool()` from `app.db` for PostgreSQL access
- Use async transactions (`pool.acquire() + tx`) for all multi-statement operations
- Maintain backward-compatible API contract (same JSON shapes)
- Go gateway uses existing `PythonClient.GetJSON/PostJSON` for proxying
- All Python endpoints require auth via `X-User-ID` header (passed by Go gateway from JWT)

---

## File Structure

**Create:**
- `python-engine/app/api/knowledge.py` — All KB CRUD endpoints
- `python-engine/tests/test_knowledge.py` — Unit tests

**Modify:**
- `python-engine/app/api/__init__.py` — Register knowledge router
- `internal/api/gateway_router.go` — Replace Go handlers with Python proxy
- `internal/engine/python_client.go` — (no changes needed, existing proxy methods suffice)

**Delete:**
- `internal/api/knowledge_handler.go` — Remove entire file

---

### Task 1: Create Python Knowledge API Module with List Endpoint

**Files:**
- Create: `python-engine/app/api/knowledge.py`
- Modify: `python-engine/app/api/__init__.py`
- Create: `python-engine/tests/test_knowledge.py`

**Interfaces:**
- Consumes: `app.db.get_pool()` → `asyncpg.Pool`
- Produces: `router = APIRouter()` with `/v1/kb` endpoints

- [ ] **Step 1: Write the failing test**

```python
# python-engine/tests/test_knowledge.py
"""Tests for knowledge base API endpoints."""
import pytest
from unittest.mock import AsyncMock, patch, MagicMock


@pytest.fixture
def mock_pool():
    """Mock asyncpg pool."""
    pool = AsyncMock()
    return pool


@pytest.fixture
def mock_user_id():
    return "test-user-123"


class TestKnowledgeList:
    """Tests for GET /v1/kb endpoint."""

    @pytest.mark.asyncio
    async def test_list_returns_empty_when_no_pool(self):
        with patch("app.api.knowledge.get_pool", side_effect=RuntimeError("no pool")):
            from app.api.knowledge import list_knowledge_bases
            result = await list_knowledge_bases(user_id="u1")
            assert result == {"knowledge_bases": [], "count": 0}

    @pytest.mark.asyncio
    async def test_list_returns_user_and_public_kbs(self, mock_pool):
        mock_rows = [
            {
                "id": "kb-1",
                "name": "My KB",
                "description": "desc",
                "type": "wiki",
                "visibility": "private",
                "status": "active",
                "document_count": 5,
                "total_size_bytes": 1024,
                "credits_consumed": 100,
                "config": None,
                "created_at": "2026-01-01T00:00:00Z",
                "updated_at": "2026-01-01T00:00:00Z",
            },
            {
                "id": "kb-2",
                "name": "Public KB",
                "description": "",
                "type": "rag",
                "visibility": "public",
                "status": "active",
                "document_count": 10,
                "total_size_bytes": 2048,
                "credits_consumed": 200,
                "config": '{"chunk_size": 512}',
                "created_at": "2026-01-02T00:00:00Z",
                "updated_at": "2026-01-02T00:00:00Z",
            },
        ]
        mock_pool.fetch = AsyncMock(return_value=mock_rows)

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import list_knowledge_bases
            result = await list_knowledge_bases(user_id="u1")

        assert result["count"] == 2
        assert result["knowledge_bases"][0]["id"] == "kb-1"
        assert result["knowledge_bases"][1]["id"] == "kb-2"

    @pytest.mark.asyncio
    async def test_list_queries_with_correct_params(self, mock_pool):
        mock_pool.fetch = AsyncMock(return_value=[])

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import list_knowledge_bases
            await list_knowledge_bases(user_id="user-abc")

        call_args = mock_pool.fetch.call_args
        query = call_args[0][0]
        assert "user_id = $1" in query
        assert "visibility = 'public'" in query
        assert call_args[0][1] == "user-abc"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd python-engine && python -m pytest tests/test_knowledge.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'app.api.knowledge'`

- [ ] **Step 3: Write minimal implementation**

```python
# python-engine/app/api/knowledge.py
"""Knowledge Base API — CRUD operations with PostgreSQL."""
from __future__ import annotations

import json
import logging
from datetime import datetime
from typing import Any, Optional

from fastapi import APIRouter, Depends, HTTPException, Query, Request
from pydantic import BaseModel

from app.db import get_pool

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/v1/kb", tags=["knowledge"])


# ── Models ──

class KnowledgeBaseCreate(BaseModel):
    name: str
    description: str = ""
    type: str = "wiki"  # wiki | rag
    visibility: str = "private"  # public | private


class KnowledgeBaseUpdate(BaseModel):
    name: Optional[str] = None
    description: Optional[str] = None
    type: Optional[str] = None


class DocumentUpload(BaseModel):
    name: str
    file_type: str
    file_size_bytes: int = 0
    content: str = ""  # base64 or plain text


class KBQuery(BaseModel):
    query: str
    top_k: int = 5


# ── Helpers ──

def _serialize_kb(row: dict) -> dict:
    """Serialize a knowledge_bases row to JSON response."""
    config = row.get("config")
    if config and isinstance(config, str):
        try:
            config = json.loads(config)
        except json.JSONDecodeError:
            config = None
    return {
        "id": row["id"],
        "name": row["name"],
        "description": row.get("description", ""),
        "type": row["type"],
        "visibility": row["visibility"],
        "status": row["status"],
        "document_count": row.get("document_count", 0),
        "total_size_bytes": row.get("total_size_bytes", 0),
        "credits_consumed": row.get("credits_consumed", 0),
        "config": config,
        "created_at": row["created_at"].isoformat() if isinstance(row["created_at"], datetime) else str(row["created_at"]),
        "updated_at": row["updated_at"].isoformat() if isinstance(row["updated_at"], datetime) else str(row["updated_at"]),
    }


def _serialize_doc(row: dict) -> dict:
    """Serialize a knowledge_documents row to JSON response."""
    metadata = row.get("metadata")
    if metadata and isinstance(metadata, str):
        try:
            metadata = json.loads(metadata)
        except json.JSONDecodeError:
            metadata = None
    return {
        "id": row["id"],
        "knowledge_base_id": row["knowledge_base_id"],
        "name": row["name"],
        "file_url": row.get("file_url", ""),
        "file_type": row.get("file_type", ""),
        "file_size_bytes": row.get("file_size_bytes", 0),
        "chunk_count": row.get("chunk_count", 0),
        "status": row["status"],
        "error_message": row.get("error_message", ""),
        "metadata": metadata,
        "created_at": row["created_at"].isoformat() if isinstance(row["created_at"], datetime) else str(row["created_at"]),
    }


# ── Core Functions (testable without FastAPI) ──

async def list_knowledge_bases(user_id: str) -> dict:
    """List knowledge bases for a user (private + public)."""
    try:
        pool = get_pool()
    except RuntimeError:
        return {"knowledge_bases": [], "count": 0}

    rows = await pool.fetch(
        """SELECT id, name, COALESCE(description, '') as description, type, visibility, status,
                  document_count, total_size_bytes, credits_consumed, config, created_at, updated_at
           FROM knowledge_bases
           WHERE user_id = $1 OR visibility = 'public'
           ORDER BY created_at DESC""",
        user_id,
    )
    kbs = [_serialize_kb(dict(row)) for row in rows]
    return {"knowledge_bases": kbs, "count": len(kbs)}


# ── Routes ──

@router.get("")
async def list_kb(
    request: Request,
    user_id: str = Query("", alias="user_id"),
):
    """List knowledge bases."""
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await list_knowledge_bases(user_id)
```

- [ ] **Step 4: Register router in `__init__.py`**

Add to `python-engine/app/api/__init__.py`:

```python
from app.api.knowledge import router as knowledge_router
api_router.include_router(knowledge_router)
```

- [ ] **Step 5: Run tests**

Run: `cd python-engine && python -m pytest tests/test_knowledge.py -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add python-engine/app/api/knowledge.py python-engine/app/api/__init__.py python-engine/tests/test_knowledge.py
git commit -m "feat(knowledge): add Python KB API with list endpoint"
```

---

### Task 2: Add Create, Get, Update, Delete Endpoints

**Files:**
- Modify: `python-engine/app/api/knowledge.py`
- Modify: `python-engine/tests/test_knowledge.py`

**Interfaces:**
- Produces: `create_knowledge_base()`, `get_knowledge_base()`, `update_knowledge_base()`, `delete_knowledge_base()`

- [ ] **Step 1: Write failing tests**

```python
# Add to python-engine/tests/test_knowledge.py

class TestKnowledgeCreate:
    @pytest.mark.asyncio
    async def test_create_kb_returns_id(self, mock_pool):
        mock_pool.fetchrow = AsyncMock(return_value={"id": "new-kb-id"})
        mock_pool.execute = AsyncMock(return_value="INSERT 0 1")

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import create_knowledge_base
            result = await create_knowledge_base(
                user_id="u1", name="Test KB", description="desc", kb_type="wiki", visibility="private"
            )
        assert result["id"] == "new-kb-id"
        assert result["name"] == "Test KB"

    @pytest.mark.asyncio
    async def test_create_rejects_empty_name(self):
        from app.api.knowledge import create_knowledge_base
        with pytest.raises(Exception):  # HTTPException 400
            await create_knowledge_base(user_id="u1", name="", description="", kb_type="wiki", visibility="private")

    @pytest.mark.asyncio
    async def test_create_rejects_invalid_type(self):
        from app.api.knowledge import create_knowledge_base
        with pytest.raises(Exception):
            await create_knowledge_base(user_id="u1", name="Test", description="", kb_type="invalid", visibility="private")


class TestKnowledgeGet:
    @pytest.mark.asyncio
    async def test_get_kb_by_id(self, mock_pool):
        mock_row = {
            "id": "kb-1", "name": "Test", "description": "", "type": "wiki",
            "visibility": "private", "status": "active", "document_count": 0,
            "total_size_bytes": 0, "credits_consumed": 0, "config": None,
            "created_at": datetime(2026, 1, 1), "updated_at": datetime(2026, 1, 1),
        }
        mock_pool.fetchrow = AsyncMock(return_value=mock_row)

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import get_knowledge_base
            result = await get_knowledge_base(kb_id="kb-1", user_id="u1")
        assert result["id"] == "kb-1"

    @pytest.mark.asyncio
    async def test_get_kb_not_found(self, mock_pool):
        mock_pool.fetchrow = AsyncMock(return_value=None)

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import get_knowledge_base
            with pytest.raises(Exception):  # HTTPException 404
                await get_knowledge_base(kb_id="nonexistent", user_id="u1")


class TestKnowledgeUpdate:
    @pytest.mark.asyncio
    async def test_update_kb_name(self, mock_pool):
        mock_pool.execute = AsyncMock(return_value="UPDATE 1")
        mock_pool.fetchrow = AsyncMock(return_value={"type": "wiki", "visibility": "private"})

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import update_knowledge_base
            result = await update_knowledge_base(kb_id="kb-1", user_id="u1", name="New Name")
        assert result["status"] == "updated"


class TestKnowledgeDelete:
    @pytest.mark.asyncio
    async def test_delete_own_kb(self, mock_pool):
        mock_pool.execute = AsyncMock(return_value="DELETE 1")

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import delete_knowledge_base
            result = await delete_knowledge_base(kb_id="kb-1", user_id="u1", is_admin=False)
        assert result["status"] == "deleted"

    @pytest.mark.asyncio
    async def test_delete_not_found(self, mock_pool):
        mock_pool.execute = AsyncMock(return_value="DELETE 0")

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import delete_knowledge_base
            with pytest.raises(Exception):  # HTTPException 404
                await delete_knowledge_base(kb_id="nonexistent", user_id="u1", is_admin=False)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd python-engine && python -m pytest tests/test_knowledge.py::TestKnowledgeCreate -v`
Expected: FAIL

- [ ] **Step 3: Implement Create, Get, Update, Delete**

Add to `python-engine/app/api/knowledge.py`:

```python
import uuid


async def create_knowledge_base(
    user_id: str, name: str, description: str, kb_type: str, visibility: str
) -> dict:
    """Create a new knowledge base."""
    if not name:
        raise HTTPException(status_code=400, detail="name is required")
    if kb_type not in ("wiki", "rag"):
        raise HTTPException(status_code=400, detail="type must be 'wiki' or 'rag'")
    if visibility not in ("public", "private"):
        raise HTTPException(status_code=400, detail="visibility must be 'public' or 'private'")

    pool = get_pool()
    kb_id = str(uuid.uuid4())
    await pool.execute(
        """INSERT INTO knowledge_bases (id, tenant_id, user_id, name, description, type, visibility, created_at, updated_at)
           VALUES ($1, '00000000-0000-0000-0000-000000000001', $2, $3, $4, $5, $6, NOW(), NOW())""",
        kb_id, user_id, name, description, kb_type, visibility,
    )
    return {"id": kb_id, "name": name, "type": kb_type, "visibility": visibility}


async def get_knowledge_base(kb_id: str, user_id: str) -> dict:
    """Get a knowledge base by ID."""
    pool = get_pool()
    row = await pool.fetchrow(
        """SELECT id, name, COALESCE(description, '') as description, type, visibility, status,
                  document_count, total_size_bytes, credits_consumed, config, created_at, updated_at
           FROM knowledge_bases
           WHERE id = $1 AND (user_id = $2 OR visibility = 'public')""",
        kb_id, user_id,
    )
    if not row:
        raise HTTPException(status_code=404, detail="knowledge base not found")
    return _serialize_kb(dict(row))


async def update_knowledge_base(
    kb_id: str, user_id: str, name: Optional[str] = None,
    description: Optional[str] = None, kb_type: Optional[str] = None,
) -> dict:
    """Update a knowledge base."""
    pool = get_pool()

    # Verify ownership
    existing = await pool.fetchrow(
        "SELECT type, visibility FROM knowledge_bases WHERE id = $1 AND user_id = $2",
        kb_id, user_id,
    )
    if not existing:
        raise HTTPException(status_code=404, detail="knowledge base not found or access denied")

    # Build dynamic update
    sets = []
    args = []
    arg_idx = 1

    if name is not None and name != "":
        sets.append(f"name = ${arg_idx}")
        args.append(name)
        arg_idx += 1
    if description is not None and description != "":
        sets.append(f"description = ${arg_idx}")
        args.append(description)
        arg_idx += 1
    if kb_type is not None and kb_type != existing["type"]:
        if existing["visibility"] == "public":
            raise HTTPException(status_code=403, detail="cannot change type of public knowledge base")
        if kb_type not in ("wiki", "rag"):
            raise HTTPException(status_code=400, detail="type must be 'wiki' or 'rag'")
        sets.append(f"type = ${arg_idx}")
        args.append(kb_type)
        arg_idx += 1

    if not sets:
        return {"status": "no changes"}

    sets.append("updated_at = NOW()")
    args.append(kb_id)

    query = f"UPDATE knowledge_bases SET {', '.join(sets)} WHERE id = ${arg_idx}"
    await pool.execute(query, *args)
    return {"status": "updated"}


async def delete_knowledge_base(kb_id: str, user_id: str, is_admin: bool = False) -> dict:
    """Delete a knowledge base."""
    pool = get_pool()
    if is_admin:
        result = await pool.execute(
            "DELETE FROM knowledge_bases WHERE id = $1", kb_id
        )
    else:
        result = await pool.execute(
            "DELETE FROM knowledge_bases WHERE id = $1 AND user_id = $2", kb_id, user_id
        )
    if result == "DELETE 0":
        raise HTTPException(status_code=404, detail="knowledge base not found")
    return {"status": "deleted"}
```

- [ ] **Step 4: Add FastAPI routes**

```python
@router.post("")
async def create_kb(body: KnowledgeBaseCreate, request: Request, user_id: str = Query("", alias="user_id")):
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await create_knowledge_base(user_id, body.name, body.description, body.type, body.visibility)


@router.get("/{kb_id}")
async def get_kb(kb_id: str, user_id: str = Query("", alias="user_id")):
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await get_knowledge_base(kb_id, user_id)


@router.put("/{kb_id}")
async def update_kb(kb_id: str, body: KnowledgeBaseUpdate, user_id: str = Query("", alias="user_id")):
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await update_knowledge_base(kb_id, user_id, body.name, body.description, body.type)


@router.delete("/{kb_id}")
async def delete_kb(kb_id: str, request: Request, user_id: str = Query("", alias="user_id")):
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    is_admin = request.headers.get("X-User-Role", "") in ("admin", "owner")
    return await delete_knowledge_base(kb_id, user_id, is_admin)
```

- [ ] **Step 5: Run tests**

Run: `cd python-engine && python -m pytest tests/test_knowledge.py -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add python-engine/app/api/knowledge.py python-engine/tests/test_knowledge.py
git commit -m "feat(knowledge): add Create, Get, Update, Delete endpoints"
```

---

### Task 3: Add Document CRUD Endpoints

**Files:**
- Modify: `python-engine/app/api/knowledge.py`
- Modify: `python-engine/tests/test_knowledge.py`

**Interfaces:**
- Produces: `upload_document()`, `list_documents()`

- [ ] **Step 1: Write failing tests**

```python
class TestDocumentUpload:
    @pytest.mark.asyncio
    async def test_upload_document(self, mock_pool):
        mock_pool.fetchrow = AsyncMock(side_effect=[
            {"status": "active"},  # KB lookup
            {"id": "doc-1"},       # INSERT RETURNING
        ])
        mock_pool.execute = AsyncMock(return_value="UPDATE 1")

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import upload_document
            result = await upload_document(
                kb_id="kb-1", user_id="u1", name="test.pdf",
                file_type="pdf", file_size_bytes=1024
            )
        assert result["id"] == "doc-1"
        assert result["name"] == "test.pdf"

    @pytest.mark.asyncio
    async def test_upload_rejects_building_kb(self, mock_pool):
        mock_pool.fetchrow = AsyncMock(return_value={"status": "building"})

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import upload_document
            with pytest.raises(Exception):
                await upload_document(
                    kb_id="kb-1", user_id="u1", name="test.pdf",
                    file_type="pdf", file_size_bytes=1024
                )


class TestDocumentList:
    @pytest.mark.asyncio
    async def test_list_documents(self, mock_pool):
        mock_pool.fetch = AsyncMock(return_value=[
            {
                "id": "doc-1", "knowledge_base_id": "kb-1", "name": "test.pdf",
                "file_url": "", "file_type": "pdf", "file_size_bytes": 1024,
                "chunk_count": 5, "status": "completed", "error_message": "",
                "metadata": None, "created_at": datetime(2026, 1, 1),
            }
        ])

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import list_documents
            result = await list_documents(kb_id="kb-1", user_id="u1")
        assert result["count"] == 1
        assert result["documents"][0]["id"] == "doc-1"
```

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement Document CRUD**

```python
async def upload_document(
    kb_id: str, user_id: str, name: str, file_type: str, file_size_bytes: int
) -> dict:
    """Upload a document to a knowledge base."""
    pool = get_pool()

    # Verify KB exists and is not building
    kb = await pool.fetchrow(
        "SELECT status FROM knowledge_bases WHERE id = $1 AND user_id = $2",
        kb_id, user_id,
    )
    if not kb:
        raise HTTPException(status_code=404, detail="knowledge base not found or access denied")
    if kb["status"] == "building":
        raise HTTPException(status_code=400, detail="knowledge base is currently building")

    doc_id = str(uuid.uuid4())
    await pool.execute(
        """INSERT INTO knowledge_documents (id, knowledge_base_id, tenant_id, user_id, name, file_type, file_size_bytes, status, created_at, updated_at)
           VALUES ($1, $2, '00000000-0000-0000-0000-000000000001', $3, $4, $5, $6, 'pending', NOW(), NOW())""",
        doc_id, kb_id, user_id, name, file_type, file_size_bytes,
    )

    # Update KB stats
    await pool.execute(
        """UPDATE knowledge_bases SET
           document_count = document_count + 1,
           total_size_bytes = total_size_bytes + $1,
           updated_at = NOW()
           WHERE id = $2""",
        file_size_bytes, kb_id,
    )

    return {"id": doc_id, "name": name, "file_type": file_type, "size": str(file_size_bytes)}


async def list_documents(kb_id: str, user_id: str) -> dict:
    """List documents in a knowledge base."""
    pool = get_pool()
    rows = await pool.fetch(
        """SELECT id, knowledge_base_id, name, COALESCE(file_url, '') as file_url,
                  COALESCE(file_type, '') as file_type, file_size_bytes, chunk_count,
                  status, COALESCE(error_message, '') as error_message, metadata, created_at
           FROM knowledge_documents
           WHERE knowledge_base_id = $1
           ORDER BY created_at DESC""",
        kb_id,
    )
    docs = [_serialize_doc(dict(row)) for row in rows]
    return {"documents": docs, "count": len(docs)}
```

- [ ] **Step 4: Add routes**

```python
@router.post("/{kb_id}/documents")
async def upload_doc(kb_id: str, body: DocumentUpload, user_id: str = Query("", alias="user_id")):
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await upload_document(kb_id, user_id, body.name, body.file_type, body.file_size_bytes)


@router.get("/{kb_id}/documents")
async def list_docs(kb_id: str, user_id: str = Query("", alias="user_id")):
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await list_documents(kb_id, user_id)
```

- [ ] **Step 5: Run tests and commit**

```bash
cd python-engine && python -m pytest tests/test_knowledge.py -v
git add python-engine/app/api/knowledge.py python-engine/tests/test_knowledge.py
git commit -m "feat(knowledge): add document upload and list endpoints"
```

---

### Task 4: Add Build and Query Endpoints

**Files:**
- Modify: `python-engine/app/api/knowledge.py`
- Modify: `python-engine/tests/test_knowledge.py`

**Interfaces:**
- Produces: `build_knowledge_base()`, `query_knowledge_base()`

- [ ] **Step 1: Write failing tests**

```python
class TestKnowledgeBuild:
    @pytest.mark.asyncio
    async def test_build_calculates_cost(self, mock_pool):
        mock_pool.fetchrow = AsyncMock(return_value={
            "type": "wiki", "document_count": 5, "total_size_bytes": 10000
        })
        mock_pool.execute = AsyncMock(return_value="UPDATE 1")

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import build_knowledge_base
            result = await build_knowledge_base(kb_id="kb-1", user_id="u1")
        assert result["status"] == "building"
        assert result["estimated_cost"] > 0

    @pytest.mark.asyncio
    async def test_build_rejects_empty_docs(self, mock_pool):
        mock_pool.fetchrow = AsyncMock(return_value={
            "type": "wiki", "document_count": 0, "total_size_bytes": 0
        })

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import build_knowledge_base
            with pytest.raises(Exception):
                await build_knowledge_base(kb_id="kb-1", user_id="u1")


class TestKnowledgeQuery:
    @pytest.mark.asyncio
    async def test_query_wiki(self, mock_pool):
        mock_pool.fetchrow = AsyncMock(return_value={"type": "wiki"})
        mock_pool.fetch = AsyncMock(return_value=[
            {"id": "chunk-1", "content": "test content", "chunk_index": 0,
             "doc_name": "test.pdf", "rank": 0.95}
        ])

        with patch("app.api.knowledge.get_pool", return_value=mock_pool):
            from app.api.knowledge import query_knowledge_base
            result = await query_knowledge_base(kb_id="kb-1", user_id="u1", query="test", top_k=5)
        assert result["type"] == "wiki"
        assert len(result["results"]) == 1
```

- [ ] **Step 2: Implement Build and Query**

```python
async def build_knowledge_base(kb_id: str, user_id: str) -> dict:
    """Build a knowledge base (index documents)."""
    pool = get_pool()

    kb = await pool.fetchrow(
        "SELECT type, document_count, total_size_bytes FROM knowledge_bases WHERE id = $1 AND user_id = $2",
        kb_id, user_id,
    )
    if not kb:
        raise HTTPException(status_code=404, detail="knowledge base not found")
    if kb["document_count"] == 0:
        raise HTTPException(status_code=400, detail="no documents to build")

    # Calculate cost
    char_count = int(kb["total_size_bytes"])
    coefficient = 0.5 if kb["type"] == "wiki" else 2.0
    rate = 0.001
    estimated_cost = min(int(char_count * coefficient * rate), 499)

    # Check balance
    user = await pool.fetchrow("SELECT COALESCE(credits, 0) as credits FROM users WHERE id = $1", user_id)
    if not user or user["credits"] < estimated_cost:
        raise HTTPException(status_code=402, detail=f"insufficient credits: need {estimated_cost}, have {user['credits'] if user else 0}")

    # Mark as building
    await pool.execute("UPDATE knowledge_bases SET status = 'building', updated_at = NOW() WHERE id = $1", kb_id)

    # Async build (simplified)
    async with pool.acquire() as conn:
        async with conn.transaction():
            await conn.execute(
                "UPDATE knowledge_documents SET status = 'completed', chunk_count = 1, updated_at = NOW() WHERE knowledge_base_id = $1 AND status = 'pending'",
                kb_id,
            )
            await conn.execute(
                "UPDATE users SET credits = credits - $1 WHERE id = $2",
                estimated_cost, user_id,
            )
            await conn.execute(
                "UPDATE knowledge_bases SET status = 'active', credits_consumed = credits_consumed + $1, updated_at = NOW() WHERE id = $2",
                estimated_cost, kb_id,
            )

    return {"status": "building", "estimated_cost": estimated_cost, "doc_count": kb["document_count"], "type": kb["type"]}


async def query_knowledge_base(kb_id: str, user_id: str, query: str, top_k: int = 5) -> dict:
    """Query a knowledge base."""
    pool = get_pool()

    kb = await pool.fetchrow(
        "SELECT type FROM knowledge_bases WHERE id = $1 AND (user_id = $2 OR visibility = 'public')",
        kb_id, user_id,
    )
    if not kb:
        raise HTTPException(status_code=404, detail="knowledge base not found")

    if kb["type"] == "wiki":
        rows = await pool.fetch(
            """SELECT kc.id, kc.content, kc.chunk_index, kd.name as doc_name,
                      ts_rank(kc.search_vector, plainto_tsquery('chinese', $1)) as rank
               FROM knowledge_chunks kc
               JOIN knowledge_documents kd ON kc.document_id = kd.id
               WHERE kc.knowledge_base_id = $2
                 AND kc.search_vector @@ plainto_tsquery('chinese', $1)
               ORDER BY rank DESC
               LIMIT $3""",
            query, kb_id, top_k,
        )
        results = [
            {"id": row["id"], "content": row["content"], "chunk_index": row["chunk_index"],
             "doc_name": row["doc_name"], "score": row["rank"]}
            for row in rows
        ]
        return {"type": "wiki", "results": results}
    else:
        # RAG mode — would need vector DB integration
        return {"type": "rag", "results": [], "message": "RAG query requires vector DB integration"}
```

- [ ] **Step 3: Add routes and commit**

```python
@router.post("/{kb_id}/build")
async def build_kb(kb_id: str, user_id: str = Query("", alias="user_id")):
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await build_knowledge_base(kb_id, user_id)


@router.post("/{kb_id}/query")
async def query_kb(kb_id: str, body: KBQuery, user_id: str = Query("", alias="user_id")):
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await query_knowledge_base(kb_id, user_id, body.query, body.top_k)
```

```bash
cd python-engine && python -m pytest tests/test_knowledge.py -v
git add python-engine/app/api/knowledge.py python-engine/tests/test_knowledge.py
git commit -m "feat(knowledge): add build and query endpoints"
```

---

### Task 5: Add Admin List Endpoint

**Files:**
- Modify: `python-engine/app/api/knowledge.py`

- [ ] **Step 1: Implement AdminList**

```python
async def admin_list_knowledge_bases() -> dict:
    """List all public knowledge bases (admin only)."""
    pool = get_pool()
    rows = await pool.fetch(
        """SELECT id, name, COALESCE(description, '') as description, type, visibility, status,
                  document_count, total_size_bytes, credits_consumed, config, created_at, updated_at
           FROM knowledge_bases
           WHERE visibility = 'public'
           ORDER BY created_at DESC""",
    )
    kbs = [_serialize_kb(dict(row)) for row in rows]
    return {"knowledge_bases": kbs, "count": len(kbs)}


@router.get("/admin/list")
async def admin_list_kb(request: Request):
    """Admin: list all public knowledge bases."""
    role = request.headers.get("X-User-Role", "")
    if role not in ("admin", "owner"):
        raise HTTPException(status_code=403, detail="admin access required")
    return await admin_list_knowledge_bases()
```

- [ ] **Step 2: Commit**

```bash
git add python-engine/app/api/knowledge.py
git commit -m "feat(knowledge): add admin list endpoint"
```

---

### Task 6: Update Go Gateway to Proxy to Python

**Files:**
- Modify: `internal/api/gateway_router.go`

- [ ] **Step 1: Replace Go KB routes with Python proxy**

In `gateway_router.go`, replace the knowledge base routes section:

```go
// Knowledge Base (auth + rate limit, proxies to Python)
kbProxy := func(method, path string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if pythonClient == nil || !pythonClient.IsConnected() {
            InternalError(w, "python engine not available")
            return
        }
        var resp interface{}
        var err error
        switch method {
        case "GET":
            err = pythonClient.GetJSON(r.Context(), path, &resp)
        case "POST":
            var body map[string]interface{}
            if err2 := DecodeJSON(w, r, &body); err2 != nil {
                return
            }
            err = pythonClient.PostJSON(r.Context(), path, body, &resp)
        case "PUT":
            var body map[string]interface{}
            if err2 := DecodeJSON(w, r, &body); err2 != nil {
                return
            }
            err = pythonClient.PostJSON(r.Context(), path, body, &resp)
        case "DELETE":
            err = pythonClient.DeleteJSON(r.Context(), path, &resp)
        }
        if err != nil {
            slog.Error("kb proxy error", "path", path, "error", err)
            InternalError(w, "python engine error")
            return
        }
        OK(w, resp)
    }
}

mux.Handle("GET /v1/kb", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    kbProxy("GET", "/v1/kb?user_id="+userIDFromClaims(getAuthClaims(r, authenticator)))(w, r)
}))))
mux.Handle("POST /v1/kb", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    kbProxy("POST", "/v1/kb?user_id="+userIDFromClaims(getAuthClaims(r, authenticator)))(w, r)
}))))
mux.Handle("GET /v1/kb/{id}", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    kbProxy("GET", "/v1/kb/"+r.PathValue("id")+"?user_id="+userIDFromClaims(getAuthClaims(r, authenticator)))(w, r)
}))))
mux.Handle("PUT /v1/kb/{id}", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    kbProxy("PUT", "/v1/kb/"+r.PathValue("id")+"?user_id="+userIDFromClaims(getAuthClaims(r, authenticator)))(w, r)
}))))
mux.Handle("DELETE /v1/kb/{id}", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    kbProxy("DELETE", "/v1/kb/"+r.PathValue("id")+"?user_id="+userIDFromClaims(getAuthClaims(r, authenticator)))(w, r)
}))))
mux.Handle("POST /v1/kb/{id}/documents", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    kbProxy("POST", "/v1/kb/"+r.PathValue("id")+"/documents?user_id="+userIDFromClaims(getAuthClaims(r, authenticator)))(w, r)
}))))
mux.Handle("GET /v1/kb/{id}/documents", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    kbProxy("GET", "/v1/kb/"+r.PathValue("id")+"/documents?user_id="+userIDFromClaims(getAuthClaims(r, authenticator)))(w, r)
}))))
mux.Handle("POST /v1/kb/{id}/build", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    kbProxy("POST", "/v1/kb/"+r.PathValue("id")+"/build?user_id="+userIDFromClaims(getAuthClaims(r, authenticator)))(w, r)
}))))
mux.Handle("POST /v1/kb/{id}/query", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    kbProxy("POST", "/v1/kb/"+r.PathValue("id")+"/query?user_id="+userIDFromClaims(getAuthClaims(r, authenticator)))(w, r)
}))))
mux.Handle("GET /v1/admin/kb", authMW(adminPermMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    kbProxy("GET", "/v1/kb/admin/list")(w, r)
}))))
```

- [ ] **Step 2: Remove Go KB handler references**

Remove `kbHandler := NewKnowledgeHandler(authenticator)` and all references to `kbHandler` methods.

- [ ] **Step 3: Run Go tests**

```bash
cd minicc && go test ./internal/api/ -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/api/gateway_router.go
git commit -m "refactor(gateway): proxy knowledge base to Python engine"
```

---

### Task 7: Delete Go Knowledge Handler

**Files:**
- Delete: `internal/api/knowledge_handler.go`

- [ ] **Step 1: Remove the file**

```bash
rm internal/api/knowledge_handler.go
```

- [ ] **Step 2: Run all tests**

```bash
cd minicc && go test ./...
cd python-engine && python -m pytest
```

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "refactor: remove Go knowledge handler (moved to Python)"
```

---

## Verification Checklist

After all tasks:

1. `go test ./...` — All Go tests pass
2. `python -m pytest` — All Python tests pass
3. Manual test: `curl http://localhost:8080/v1/kb` returns proxied response from Python
4. No `SET LOCAL` without transaction in codebase
5. All KB operations use proper async transactions in Python
