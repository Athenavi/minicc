"""Knowledge Base API — CRUD operations with PostgreSQL."""
from __future__ import annotations

import json
import logging
import uuid
from datetime import datetime, timezone
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


async def create_knowledge_base(
    user_id: str,
    name: str,
    description: str = "",
    kb_type: str = "wiki",
    visibility: str = "private",
) -> dict:
    """Create a new knowledge base."""
    if not name or not name.strip():
        raise HTTPException(status_code=400, detail="name must not be empty")
    if kb_type not in ("wiki", "rag"):
        raise HTTPException(status_code=400, detail="type must be 'wiki' or 'rag'")
    if visibility not in ("public", "private"):
        raise HTTPException(status_code=400, detail="visibility must be 'public' or 'private'")

    pool = get_pool()
    kb_id = str(uuid.uuid4())
    now = datetime.now(timezone.utc)
    await pool.execute(
        """INSERT INTO knowledge_bases (id, user_id, name, description, type, visibility, status, created_at, updated_at)
           VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, $7)""",
        kb_id, user_id, name.strip(), description, kb_type, visibility, now,
    )
    return {"id": kb_id, "name": name.strip(), "type": kb_type, "visibility": visibility}


async def get_knowledge_base(kb_id: str, user_id: str) -> dict:
    """Get a single knowledge base by id (own or public)."""
    pool = get_pool()
    row = await pool.fetchrow(
        """SELECT id, name, COALESCE(description, '') as description, type, visibility, status,
                  document_count, total_size_bytes, credits_consumed, config, created_at, updated_at
           FROM knowledge_bases
           WHERE id = $1 AND (user_id = $2 OR visibility = 'public')""",
        kb_id, user_id,
    )
    if row is None:
        raise HTTPException(status_code=404, detail="knowledge base not found")
    return _serialize_kb(dict(row))


async def update_knowledge_base(
    kb_id: str,
    user_id: str,
    name: Optional[str] = None,
    description: Optional[str] = None,
    kb_type: Optional[str] = None,
) -> dict:
    """Update a knowledge base (owner only). Public KBs cannot change type."""
    pool = get_pool()

    # Verify ownership
    existing = await pool.fetchrow(
        "SELECT id, user_id, visibility FROM knowledge_bases WHERE id = $1",
        kb_id,
    )
    if existing is None:
        raise HTTPException(status_code=404, detail="knowledge base not found")
    if existing["user_id"] != user_id:
        raise HTTPException(status_code=403, detail="not authorized to update this knowledge base")

    # Public KBs can't change type
    if kb_type is not None and existing["visibility"] == "public":
        raise HTTPException(status_code=400, detail="cannot change type of a public knowledge base")

    if kb_type is not None and kb_type not in ("wiki", "rag"):
        raise HTTPException(status_code=400, detail="type must be 'wiki' or 'rag'")

    # Dynamic UPDATE — only set provided fields
    updates: list[str] = []
    params: list[Any] = []
    idx = 1
    if name is not None:
        if not name.strip():
            raise HTTPException(status_code=400, detail="name must not be empty")
        updates.append(f"name = ${idx}")
        params.append(name.strip())
        idx += 1
    if description is not None:
        updates.append(f"description = ${idx}")
        params.append(description)
        idx += 1
    if kb_type is not None:
        updates.append(f"type = ${idx}")
        params.append(kb_type)
        idx += 1

    if not updates:
        raise HTTPException(status_code=400, detail="no fields to update")

    updates.append(f"updated_at = ${idx}")
    params.append(datetime.now(timezone.utc))
    idx += 1

    params.append(kb_id)
    query = f"UPDATE knowledge_bases SET {', '.join(updates)} WHERE id = ${idx}"
    await pool.execute(query, *params)

    return {"id": kb_id, "updated": True}


async def delete_knowledge_base(kb_id: str, user_id: str, is_admin: bool = False) -> dict:
    """Delete a knowledge base. Admin can delete any; users can only delete their own."""
    pool = get_pool()

    existing = await pool.fetchrow(
        "SELECT id, user_id FROM knowledge_bases WHERE id = $1",
        kb_id,
    )
    if existing is None:
        raise HTTPException(status_code=404, detail="knowledge base not found")

    if not is_admin and existing["user_id"] != user_id:
        raise HTTPException(status_code=403, detail="not authorized to delete this knowledge base")

    await pool.execute("DELETE FROM knowledge_bases WHERE id = $1", kb_id)
    return {"id": kb_id, "deleted": True}


# ── Core Functions — Documents ──

async def upload_document(
    kb_id: str,
    user_id: str,
    name: str,
    file_type: str,
    file_size_bytes: int = 0,
) -> dict:
    """Upload a document into a knowledge base.

    Verifies the KB exists, is not currently building, inserts the document
    row, and atomically updates the KB aggregate stats.
    """
    pool = get_pool()

    # Verify KB exists and is owned by (or visible to) user
    kb_row = await pool.fetchrow(
        "SELECT id, status, user_id FROM knowledge_bases WHERE id = $1",
        kb_id,
    )
    if kb_row is None:
        raise HTTPException(status_code=404, detail="knowledge base not found")
    if kb_row["user_id"] != user_id:
        raise HTTPException(status_code=403, detail="not authorized to upload to this knowledge base")

    # Reject if KB is currently building
    if kb_row["status"] == "building":
        raise HTTPException(status_code=409, detail="knowledge base is currently building, try again later")

    if not name or not name.strip():
        raise HTTPException(status_code=400, detail="document name must not be empty")

    doc_id = str(uuid.uuid4())
    now = datetime.now(timezone.utc)

    # Insert the document
    await pool.execute(
        """INSERT INTO knowledge_documents
               (id, knowledge_base_id, user_id, name, file_type, file_size_bytes, status, created_at)
           VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7)""",
        doc_id, kb_id, user_id, name.strip(), file_type, file_size_bytes, now,
    )

    # Update KB aggregate stats
    await pool.execute(
        """UPDATE knowledge_bases
           SET document_count = document_count + 1,
               total_size_bytes = total_size_bytes + $1,
               updated_at = $2
           WHERE id = $3""",
        file_size_bytes, now, kb_id,
    )

    return {
        "id": doc_id,
        "knowledge_base_id": kb_id,
        "name": name.strip(),
        "file_type": file_type,
        "file_size_bytes": file_size_bytes,
        "status": "pending",
        "created_at": now.isoformat(),
    }


async def list_documents(kb_id: str, user_id: str) -> dict:
    """List all documents in a knowledge base."""
    pool = get_pool()

    # Verify KB exists and is accessible
    kb_row = await pool.fetchrow(
        "SELECT id, user_id, visibility FROM knowledge_bases WHERE id = $1",
        kb_id,
    )
    if kb_row is None:
        raise HTTPException(status_code=404, detail="knowledge base not found")
    if kb_row["user_id"] != user_id and kb_row["visibility"] != "public":
        raise HTTPException(status_code=403, detail="not authorized to view this knowledge base")

    rows = await pool.fetch(
        """SELECT id, knowledge_base_id, name, file_url, file_type, file_size_bytes,
                  chunk_count, status, error_message, metadata, created_at
           FROM knowledge_documents
           WHERE knowledge_base_id = $1
           ORDER BY created_at DESC""",
        kb_id,
    )
    docs = [_serialize_doc(dict(row)) for row in rows]
    return {"documents": docs, "count": len(docs)}


# ── Core Functions — Build & Query ──

async def build_knowledge_base(kb_id: str, user_id: str) -> dict:
    """Build a knowledge base — mark documents as completed and deduct credits."""
    pool = get_pool()

    # Fetch KB
    kb_row = await pool.fetchrow(
        """SELECT id, type, status, document_count, total_size_bytes, user_id
           FROM knowledge_bases WHERE id = $1""",
        kb_id,
    )
    if kb_row is None:
        raise HTTPException(status_code=404, detail="knowledge base not found")
    if kb_row["user_id"] != user_id:
        raise HTTPException(status_code=403, detail="not authorized to build this knowledge base")

    doc_count = kb_row["document_count"] or 0
    if doc_count == 0:
        raise HTTPException(status_code=400, detail="cannot build knowledge base with no documents")

    # Calculate cost
    char_count = kb_row["total_size_bytes"] or 0
    kb_type = kb_row["type"]
    coefficient = 0.5 if kb_type == "wiki" else 2.0
    rate = 0.001
    estimated_cost = min(char_count * coefficient * rate, 499)

    # Check user credits
    user_row = await pool.fetchrow(
        "SELECT credits FROM users WHERE id = $1",
        user_id,
    )
    if user_row is None:
        raise HTTPException(status_code=404, detail="user not found")

    user_credits = user_row["credits"] or 0
    if user_credits < estimated_cost:
        raise HTTPException(status_code=402, detail="insufficient credits")

    # Mark KB as building
    now = datetime.now(timezone.utc)
    await pool.execute(
        "UPDATE knowledge_bases SET status = 'building', updated_at = $1 WHERE id = $2",
        now, kb_id,
    )

    # Transaction: update documents, deduct credits, activate KB
    async with pool.acquire() as conn:
        async with conn.transaction():
            await conn.execute(
                """UPDATE knowledge_documents
                   SET status = 'completed'
                   WHERE knowledge_base_id = $1""",
                kb_id,
            )
            await conn.execute(
                "UPDATE users SET credits = credits - $1 WHERE id = $2",
                estimated_cost, user_id,
            )
            await conn.execute(
                """UPDATE knowledge_bases
                   SET status = 'active', credits_consumed = credits_consumed + $1, updated_at = $2
                   WHERE id = $3""",
                estimated_cost, now, kb_id,
            )

    return {
        "status": "active",
        "estimated_cost": round(estimated_cost, 4),
        "doc_count": doc_count,
        "type": kb_type,
    }


async def query_knowledge_base(
    kb_id: str, user_id: str, query: str, top_k: int = 5,
) -> dict:
    """Query a knowledge base using full-text search (wiki) or RAG."""
    pool = get_pool()

    # Fetch KB and verify access
    kb_row = await pool.fetchrow(
        """SELECT id, type, user_id, visibility
           FROM knowledge_bases WHERE id = $1""",
        kb_id,
    )
    if kb_row is None:
        raise HTTPException(status_code=404, detail="knowledge base not found")
    if kb_row["user_id"] != user_id and kb_row["visibility"] != "public":
        raise HTTPException(status_code=403, detail="not authorized to query this knowledge base")

    kb_type = kb_row["type"]

    if kb_type == "wiki":
        rows = await pool.fetch(
            """SELECT id, knowledge_base_id, name, file_type, status,
                      ts_rank(to_tsvector('chinese', COALESCE(content, '')),
                              plainto_tsquery('chinese', $1)) AS rank
               FROM knowledge_documents
               WHERE knowledge_base_id = $2
                 AND status = 'completed'
                 AND to_tsvector('chinese', COALESCE(content, ''))
                     @@ plainto_tsquery('chinese', $1)
               ORDER BY rank DESC
               LIMIT $3""",
            query, kb_id, top_k,
        )
        results = [
            {
                "id": row["id"],
                "knowledge_base_id": row["knowledge_base_id"],
                "name": row["name"],
                "file_type": row["file_type"],
                "rank": float(row["rank"]),
            }
            for row in rows
        ]
    else:
        results = [{"message": "RAG query not yet implemented"}]

    return {"type": kb_type, "results": results}


async def admin_list_knowledge_bases() -> dict:
    """List all public knowledge bases (admin/owner only)."""
    try:
        pool = get_pool()
    except RuntimeError:
        return {"knowledge_bases": [], "count": 0}

    rows = await pool.fetch(
        """SELECT id, name, COALESCE(description, '') as description, type, visibility, status,
                  document_count, total_size_bytes, credits_consumed, config, created_at, updated_at
           FROM knowledge_bases
           WHERE visibility = 'public'
           ORDER BY created_at DESC""",
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


@router.post("")
async def create_kb(
    request: Request,
    body: KnowledgeBaseCreate,
    user_id: str = Query("", alias="user_id"),
):
    """Create a knowledge base."""
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await create_knowledge_base(
        user_id=user_id,
        name=body.name,
        description=body.description,
        kb_type=body.type,
        visibility=body.visibility,
    )


@router.get("/admin/list")
async def admin_list_kb(request: Request):
    """Admin endpoint: list all public knowledge bases."""
    role = request.headers.get("X-User-Role", "")
    if role not in ("admin", "owner"):
        raise HTTPException(status_code=403, detail="admin or owner role required")
    return await admin_list_knowledge_bases()


@router.get("/{kb_id}")
async def get_kb(
    kb_id: str,
    request: Request,
    user_id: str = Query("", alias="user_id"),
):
    """Get a knowledge base by id."""
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await get_knowledge_base(kb_id=kb_id, user_id=user_id)


@router.put("/{kb_id}")
async def update_kb(
    kb_id: str,
    request: Request,
    body: KnowledgeBaseUpdate,
    user_id: str = Query("", alias="user_id"),
):
    """Update a knowledge base."""
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await update_knowledge_base(
        kb_id=kb_id,
        user_id=user_id,
        name=body.name,
        description=body.description,
        kb_type=body.type,
    )


@router.delete("/{kb_id}")
async def delete_kb(
    kb_id: str,
    request: Request,
    user_id: str = Query("", alias="user_id"),
    is_admin: bool = Query(False, alias="is_admin"),
):
    """Delete a knowledge base."""
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await delete_knowledge_base(kb_id=kb_id, user_id=user_id, is_admin=is_admin)


@router.post("/{kb_id}/documents")
async def upload_doc(
    kb_id: str,
    request: Request,
    body: DocumentUpload,
    user_id: str = Query("", alias="user_id"),
):
    """Upload a document to a knowledge base."""
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await upload_document(
        kb_id=kb_id,
        user_id=user_id,
        name=body.name,
        file_type=body.file_type,
        file_size_bytes=body.file_size_bytes,
    )


@router.get("/{kb_id}/documents")
async def list_docs(
    kb_id: str,
    request: Request,
    user_id: str = Query("", alias="user_id"),
):
    """List documents in a knowledge base."""
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await list_documents(kb_id=kb_id, user_id=user_id)


@router.post("/{kb_id}/build")
async def build_kb(
    kb_id: str,
    request: Request,
    user_id: str = Query("", alias="user_id"),
):
    """Build a knowledge base."""
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await build_knowledge_base(kb_id=kb_id, user_id=user_id)


@router.post("/{kb_id}/query")
async def query_kb(
    kb_id: str,
    request: Request,
    body: KBQuery,
    user_id: str = Query("", alias="user_id"),
):
    """Query a knowledge base."""
    if not user_id:
        raise HTTPException(status_code=401, detail="user_id required")
    return await query_knowledge_base(
        kb_id=kb_id,
        user_id=user_id,
        query=body.query,
        top_k=body.top_k,
    )
