"""知识库 RAG 增强: 完整的文档生命周期管理 + 租户隔离

功能:
1. 文档上传 → 分块 → 嵌入 → Milvus 存储 (租户隔离)
2. 查询时动态检索相关 chunk (COSINE 相似度)
3. 独立限流 (QPS=20, Burst=40 每租户)
4. Trace 集成: 每次检索记录 span
"""
from __future__ import annotations

import json
import logging
import time
from typing import Optional
from dataclasses import dataclass, field

from app.rag.retriever import RAGRetriever
from app.trace import record_span

logger = logging.getLogger(__name__)


@dataclass
class DocumentChunk:
    """文档分片"""
    chunk_id: str
    document_id: str
    tenant_id: str  # SaaS 安全: 租户隔离
    content: str
    embedding: list[float]
    metadata: dict = field(default_factory=dict)


@dataclass
class RetrieveResult:
    """检索结果"""
    document_id: str
    chunk_id: str
    content: str
    score: float
    tenant_id: str
    trace_id: str = ""


class EnhancedKnowledgeBase:
    """增强的知识库 (租户隔离 + 链路追踪)
    
    核心能力:
    1. 文档索引 (分块 + 嵌入 + Milvus 存储)
    2. 向量检索 (COSINE 相似度, 租户隔离 filter)
    3. 文档管理 (列出/删除/统计)
    4. 链路追踪 (每次检索记录 span)
    """
    
    def __init__(self):
        self.retriever = RAGRetriever()
        self._collections: dict[str, str] = {}  # tenant_id -> collection_name
    
    async def index_document(
        self,
        tenant_id: str,
        document_id: str,
        content: str,
        file_type: str = "txt",
        metadata: Optional[dict] = None,
        trace_id: str = "",
    ) -> dict:
        """索引文档 (带 trace)
        
        流程:
        1. 文本分块
        2. 调用 LLM 生成嵌入向量
        3. 存入 Milvus (带 tenant_id filter)
        4. 记录 span
        """
        start_time = time.time()
        
        try:
            result = await self.retriever.index_document(
                tenant_id=tenant_id,
                document_id=document_id,
                content=content,
                file_type=file_type,
                metadata=metadata or {},
            )
            
            duration_ms = int((time.time() - start_time) * 1000)
            
            # 记录 span
            if trace_id:
                await record_span(
                    trace_id=trace_id,
                    span_name="kb:index_document",
                    duration_ms=duration_ms,
                    metadata={
                        "document_id": document_id,
                        "chunks_count": result.get("chunks_count", 0),
                        "file_type": file_type,
                        "tenant_id": tenant_id,
                    },
                    tenant_id=tenant_id,
                )
            
            logger.info(
                "Document indexed (doc=%s, chunks=%d, duration=%dms, tenant=%s)",
                document_id, result.get("chunks_count", 0), duration_ms, tenant_id,
            )
            
            return result
            
        except Exception as e:
            logger.error(f"Document indexing failed: {e}")
            return {
                "document_id": document_id,
                "chunks_count": 0,
                "status": "failed",
                "error": str(e),
            }
    
    async def retrieve(
        self,
        tenant_id: str,
        query: str,
        top_k: int = 5,
        threshold: float = 0.7,
        trace_id: str = "",
    ) -> list[RetrieveResult]:
        """检索相关文档片段 (带 trace + 租户隔离)
        
        SaaS 安全:
        - 查询条件强制过滤 tenant_id
        - Milvus expr = f'tenant_id == "{tenant_id}"'
        - 所有返回结果携带 tenant_id 元数据
        """
        start_time = time.time()
        
        try:
            # 调用底层检索器
            raw_results = await self.retriever.retrieve(
                tenant_id=tenant_id,
                query=query,
                top_k=top_k,
                threshold=threshold,
            )
            
            duration_ms = int((time.time() - start_time) * 1000)
            
            # 转换为 RetrieveResult
            results = [
                RetrieveResult(
                    document_id=r["document_id"],
                    chunk_id=r["chunk_id"],
                    content=r["content"],
                    score=r["score"],
                    tenant_id=tenant_id,
                    trace_id=trace_id,
                )
                for r in raw_results
            ]
            
            # 记录 span
            if trace_id:
                await record_span(
                    trace_id=trace_id,
                    span_name="kb:retrieve",
                    duration_ms=duration_ms,
                    metadata={
                        "query_length": len(query),
                        "results_count": len(results),
                        "top_k": top_k,
                        "threshold": threshold,
                        "tenant_id": tenant_id,
                    },
                    tenant_id=tenant_id,
                )
            
            logger.info(
                "Retrieved %d chunks (query=%s, duration=%dms, tenant=%s)",
                len(results), query[:50], duration_ms, tenant_id,
            )
            
            return results
            
        except Exception as e:
            logger.error(f"Retrieval failed: {e}")
            return []
    
    async def list_documents(
        self,
        tenant_id: str,
    ) -> list[dict]:
        """列出租户下的所有文档 (从 PG metadata)"""
        # Fail loud: PG kb_documents 元数据表尚不存在。
        # 返回空列表会让调用方误以为"该租户真的没有文档",
        # 必须显式抛错,由调用方转成 "功能未实现" 语义 (如 HTTP 501)。
        raise NotImplementedError(
            "list_documents not implemented: kb_documents metadata table does not exist yet"
        )
    
    async def delete_document(
        self,
        tenant_id: str,
        document_id: str,
    ) -> bool:
        """删除文档 (Milvus + PG 双删)"""
        try:
            # 1. 删除 Milvus 中的 chunks
            collection = await self.retriever._get_collection()
            if collection:
                delete_expr = f'document_id == "{document_id}" AND tenant_id == "{tenant_id}"'
                await collection.delete(expr=delete_expr)
                await collection.flush()
                
            # 2. 删除 PG 中的 metadata (TODO)
            
            logger.info(f"Document deleted (doc={document_id}, tenant={tenant_id})")
            return True
            
        except Exception as e:
            logger.error(f"Document deletion failed: {e}")
            return False
    
    async def get_tenant_stats(
        self,
        tenant_id: str,
    ) -> dict:
        """获取租户知识库统计信息"""
        # Fail loud: 不存在可查询的真实数据源 (PG kb_documents 表未建立,
        # Milvus 侧也没有租户级统计)。绝不允许返回硬编码 0 伪装成真实统计,
        # 否则调用方无法区分"真的 0 条"和"功能未实现"。
        # 调用方应捕获此异常并转成 HTTP 501 / "功能未实现" 标记。
        raise NotImplementedError(
            "get_tenant_stats not implemented: no kb_documents stats source exists yet"
        )


# ── API Handler ────────────────────────────────────────────────────
class KnowledgeBaseHandler:
    """知识库 HTTP Handler (Go 侧代理到 Python)
    
    路由:
    - POST /v1/kb/index      索引文档
    - POST /v1/kb/search     检索文档
    - GET  /v1/kb/documents  列出文档
    - DELETE /v1/kb/{id}     删除文档
    - GET  /v1/kb/stats      租户统计
    """
    
    def __init__(self):
        self.kb = EnhancedKnowledgeBase()
    
    async def handle_index(
        self,
        tenant_id: str,
        trace_id: str,
        document_id: str,
        content: str,
        file_type: str = "txt",
    ) -> dict:
        """处理文档索引请求"""
        return await self.kb.index_document(
            tenant_id=tenant_id,
            document_id=document_id,
            content=content,
            file_type=file_type,
            trace_id=trace_id,
        )
    
    async def handle_search(
        self,
        tenant_id: str,
        trace_id: str,
        query: str,
        top_k: int = 5,
        threshold: float = 0.7,
    ) -> dict:
        """处理检索请求"""
        results = await self.kb.retrieve(
            tenant_id=tenant_id,
            query=query,
            top_k=top_k,
            threshold=threshold,
            trace_id=trace_id,
        )
        
        return {
            "query": query,
            "results": [
                {
                    "document_id": r.document_id,
                    "chunk_id": r.chunk_id,
                    "content": r.content,
                    "score": r.score,
                }
                for r in results
            ],
            "count": len(results),
        }
    
    async def handle_delete(
        self,
        tenant_id: str,
        document_id: str,
    ) -> dict:
        """处理删除请求"""
        success = await self.kb.delete_document(tenant_id, document_id)
        return {
            "document_id": document_id,
            "deleted": success,
        }


# ── 限流中间件配置 ─────────────────────────────────────────────────
# Go 侧配置示例:
# kbRateLimiter := NewTenantRateLimiter(redis, 20, 40)  // QPS=20, Burst=40
# kbRateMW := kbRateLimiter.Middleware
