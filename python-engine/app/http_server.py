"""FastAPI HTTP server — wraps agent loop for Go backend HTTP calls."""
import json
import logging
from typing import AsyncIterator, Optional

from fastapi import FastAPI, UploadFile, File, Form
from fastapi.responses import StreamingResponse
from pydantic import BaseModel

from app.rag.parser import document_parser, text_chunker

logger = logging.getLogger(__name__)

app = FastAPI(title="MiniCC Python AI Engine")


class RunRequest(BaseModel):
    session_id: str = ""
    user_id: str = ""
    content: str = ""
    system_prompt: str = ""
    history: list[dict] = []
    tools: list[dict] = []
    llm_config: Optional[dict] = None
    max_turns: int = 10


@app.get("/health")
async def health():
    return {"status": "ok", "engine": "python"}


@app.post("/v1/agent/run")
async def agent_run(req: RunRequest):
    """Stream agent inference via SSE."""
    async def event_generator() -> AsyncIterator[str]:
        try:
            from app.agent.loop import run_agent
            from app.gateway.router import GatewayRouter
            gateway = GatewayRouter()
            async for event in run_agent(
                gateway=gateway,
                system_prompt=req.system_prompt,
                history=req.history,
                content=req.content,
                tools=req.tools if req.tools else None,
                llm_config=req.llm_config,
                max_turns=req.max_turns if req.max_turns > 0 else None,
            ):
                data = json.dumps(event, ensure_ascii=False)
                yield f"data: {data}\n\n"
        except Exception as e:
            logger.error(f"Agent run error: {e}")
            yield f"data: {json.dumps({'type': 'error', 'message': str(e)})}\n\n"

    return StreamingResponse(
        event_generator(),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
    )


# ── 知识库 API ──

class ParseRequest(BaseModel):
    file_type: str
    filename: str = ""


@app.post("/v1/kb/parse")
async def parse_document(
    file: UploadFile = File(...),
    file_type: str = Form(...),
):
    """解析文档，返回文本内容和字符数"""
    try:
        content = await file.read()
        result = document_parser.parse(content, file_type, file.filename or "")
        return {
            "success": True,
            "text": result["text"][:50000],  # 限制返回长度
            "char_count": result["char_count"],
            "page_count": result["page_count"],
            "metadata": result["metadata"],
            "error": result["error"],
        }
    except Exception as e:
        logger.error(f"文档解析失败: {e}")
        return {"success": False, "error": str(e)}


class ChunkRequest(BaseModel):
    text: str
    chunk_size: int = 1000
    chunk_overlap: int = 200


@app.post("/v1/kb/chunk")
async def chunk_text(req: ChunkRequest):
    """将文本分块"""
    try:
        chunks = text_chunker.chunk(req.text, req.chunk_size, req.chunk_overlap)
        return {
            "success": True,
            "chunks": chunks,
            "chunk_count": len(chunks),
        }
    except Exception as e:
        logger.error(f"分块失败: {e}")
        return {"success": False, "error": str(e)}


class BuildRequest(BaseModel):
    kb_id: str
    doc_id: str
    content: str  # base64 encoded
    file_type: str
    filename: str = ""
    tenant_id: str = ""
    vector_db: str = "milvus"


@app.post("/v1/kb/build")
async def build_document(req: BuildRequest):
    """构建文档索引（SSE 流式返回进度）"""
    import base64

    try:
        content_bytes = base64.b64decode(req.content)
    except Exception:
        content_bytes = req.content.encode("utf-8")

    async def event_generator() -> AsyncIterator[str]:
        try:
            from app.rag.builder import RAGBuilder
            builder = RAGBuilder(llm_gateway=None)
            async for event in builder.build_document(
                kb_id=req.kb_id,
                doc_id=req.doc_id,
                content=content_bytes,
                file_type=req.file_type,
                filename=req.filename,
                tenant_id=req.tenant_id,
                vector_db=req.vector_db,
            ):
                data = json.dumps(event, ensure_ascii=False)
                yield f"data: {data}\n\n"
        except Exception as e:
            logger.error(f"构建失败: {e}")
            yield f"data: {json.dumps({'type': 'error', 'message': str(e)})}\n\n"

    return StreamingResponse(
        event_generator(),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
    )


class QueryRequest(BaseModel):
    kb_id: str
    query: str
    top_k: int = 5
    threshold: float = 0.5
    vector_db: str = "milvus"


@app.post("/v1/kb/query")
async def query_knowledge_base(req: QueryRequest):
    """查询知识库"""
    try:
        if req.vector_db == "milvus":
            from app.rag.builder import RAGBuilder
            _rag_builder = RAGBuilder(llm_gateway=None)
            results = await _rag_builder.query_milvus(
                kb_id=req.kb_id,
                query=req.query,
                top_k=req.top_k,
                threshold=req.threshold,
            )
        elif req.vector_db == "qdrant":
            from app.rag.builder import RAGBuilder
            _rag_builder = RAGBuilder(llm_gateway=None)
            results = await _rag_builder.query_qdrant(
                kb_id=req.kb_id,
                query=req.query,
                top_k=req.top_k,
                threshold=req.threshold,
            )
        else:
            results = []

        return {
            "success": True,
            "results": results,
            "count": len(results),
        }
    except Exception as e:
        logger.error(f"查询失败: {e}")
        return {"success": False, "error": str(e)}
