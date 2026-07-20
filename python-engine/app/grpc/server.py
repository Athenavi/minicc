# gRPC Server 实现
import asyncio
import logging
import time
from typing import AsyncIterator

import grpc
from concurrent import futures
from app.config import settings
from app.grpc import agent_pb2, agent_pb2_grpc, rag_pb2, rag_pb2_grpc, memory_pb2, memory_pb2_grpc, common_pb2, knowledge_pb2

logger = logging.getLogger(__name__)


class AgentEngineServicer(agent_pb2_grpc.AgentEngineServicer):
    """Agent 推理引擎 gRPC 服务"""

    def __init__(self):
        from app.agent.loop import run_agent
        self._run_agent = run_agent

    async def Run(self, request, context):
        """Agent 推理，流式返回结果"""
        try:
            # 转换请求参数
            history = []
            for msg in request.history:
                history.append({
                    "role": msg.role,
                    "content": msg.content,
                })

            tools = []
            for tool in request.tools:
                tools.append({
                    "name": tool.name,
                    "description": tool.description,
                    "parameters_json": tool.parameters_json,
                })

            llm_config = None
            if request.llm_config and request.llm_config.model:
                llm_config = {
                    "model": request.llm_config.model,
                    "max_tokens": request.llm_config.max_tokens,
                    "temperature": request.llm_config.temperature,
                }

            # 执行推理循环
            async for event in self._run_agent(
                system_prompt=request.system_prompt,
                history=history,
                content=request.content,
                tools=tools if tools else None,
                llm_config=llm_config,
                max_turns=request.max_turns if request.max_turns > 0 else None,
            ):
                if event["type"] == "text":
                    yield agent_pb2.RunResponse(text_chunk=event["content"])
                elif event["type"] == "tool_call":
                    yield agent_pb2.RunResponse(
                        tool_call=agent_pb2.ToolCallRequest(
                            id=event["id"],
                            name=event["name"],
                            arguments=event["arguments"],
                        )
                    )
                elif event["type"] == "usage":
                    yield agent_pb2.RunResponse(
                        usage=common_pb2.Usage(
                            input_tokens=event["input_tokens"],
                            output_tokens=event["output_tokens"],
                            total_tokens=event["input_tokens"] + event["output_tokens"],
                        )
                    )
                elif event["type"] == "done":
                    yield agent_pb2.RunResponse(done=True)
                elif event["type"] == "error":
                    yield agent_pb2.RunResponse(error=event["message"])

        except Exception as e:
            logger.error(f"Run 错误: {e}")
            yield agent_pb2.RunResponse(error=str(e))

    async def SubmitToolOutput(self, request, context):
        """提交工具执行结果，继续推理"""
        try:
            # 构建更新后的消息历史
            history = []
            for msg in request.history:
                history.append({
                    "role": msg.role,
                    "content": msg.content,
                })

            tools = []
            for tool in request.tools:
                tools.append({
                    "name": tool.name,
                    "description": tool.description,
                    "parameters_json": tool.parameters_json,
                })

            llm_config = None
            if request.llm_config and request.llm_config.model:
                llm_config = {
                    "model": request.llm_config.model,
                    "max_tokens": request.llm_config.max_tokens,
                    "temperature": request.llm_config.temperature,
                }

            # 将工具结果添加到历史
            history.append({
                "role": "tool",
                "content": request.output,
                "tool_call_id": request.tool_call_id,
            })

            # 继续推理
            async for event in self._run_agent(
                system_prompt="",
                history=history,
                content="",
                tools=tools if tools else None,
                llm_config=llm_config,
                max_turns=request.max_turns if request.max_turns > 0 else None,
            ):
                if event["type"] == "text":
                    yield agent_pb2.RunResponse(text_chunk=event["content"])
                elif event["type"] == "tool_call":
                    yield agent_pb2.RunResponse(
                        tool_call=agent_pb2.ToolCallRequest(
                            id=event["id"],
                            name=event["name"],
                            arguments=event["arguments"],
                        )
                    )
                elif event["type"] == "usage":
                    yield agent_pb2.RunResponse(
                        usage=common_pb2.Usage(
                            input_tokens=event["input_tokens"],
                            output_tokens=event["output_tokens"],
                            total_tokens=event["input_tokens"] + event["output_tokens"],
                        )
                    )
                elif event["type"] == "done":
                    yield agent_pb2.RunResponse(done=True)
                elif event["type"] == "error":
                    yield agent_pb2.RunResponse(error=event["message"])

        except Exception as e:
            logger.error(f"SubmitToolOutput 错误: {e}")
            yield agent_pb2.RunResponse(error=str(e))

    async def HealthCheck(self, request, context):
        """健康检查"""
        return common_pb2.HealthCheckResponse(
            status="ok",
            version="1.0.0",
            uptime=self._get_uptime(),
        )

    def _get_uptime(self):
        """获取运行时间"""
        if not hasattr(self, '_start_time'):
            self._start_time = time.time()
        elapsed = int(time.time() - self._start_time)
        hours = elapsed // 3600
        minutes = (elapsed % 3600) // 60
        seconds = elapsed % 60
        return f"{hours}h{minutes}m{seconds}s"


class RAGEngineServicer(rag_pb2_grpc.RAGEngineServicer):
    """RAG 检索引擎 gRPC 服务"""

    def __init__(self):
        from app.rag.retriever import RAGRetriever
        self.retriever = RAGRetriever()

    async def IndexDocument(self, request, context):
        """文档索引"""
        try:
            result = await self.retriever.index_document(
                tenant_id=request.tenant_id,
                document_id=request.document_id,
                content=request.content,
                file_type=request.file_type,
                metadata=dict(request.metadata) if request.metadata else {},
            )
            return rag_pb2.IndexDocumentResponse(
                document_id=result["document_id"],
                chunks_count=result["chunks_count"],
                status=result["status"],
                error=result.get("error", ""),
            )
        except Exception as e:
            logger.error(f"IndexDocument 错误: {e}")
            return rag_pb2.IndexDocumentResponse(
                document_id=request.document_id,
                chunks_count=0,
                status="failed",
                error=str(e),
            )

    async def Retrieve(self, request, context):
        """向量检索"""
        try:
            results = await self.retriever.retrieve(
                tenant_id=request.tenant_id,
                query=request.query,
                top_k=request.top_k if request.top_k > 0 else settings.default_top_k,
                threshold=request.threshold if request.threshold > 0 else settings.default_threshold,
            )
            retrieve_results = []
            for r in results:
                retrieve_results.append(rag_pb2.RetrieveResult(
                    document_id=r["document_id"],
                    chunk_id=r.get("chunk_id", ""),
                    content=r["content"],
                    score=r["score"],
                    metadata=r.get("metadata", {}),
                ))
            return rag_pb2.RetrieveResponse(results=retrieve_results)
        except Exception as e:
            logger.error(f"Retrieve 错误: {e}")
            return rag_pb2.RetrieveResponse(results=[])

    async def HealthCheck(self, request, context):
        """健康检查"""
        return common_pb2.HealthCheckResponse(
            status="ok",
            version="1.0.0",
            uptime="0h0m0s",
        )


class MemoryEngineServicer(memory_pb2_grpc.MemoryEngineServicer):
    """记忆管理引擎 gRPC 服务"""

    def __init__(self):
        from app.memory.manager import MemoryManager
        self.manager = MemoryManager()

    async def SaveMemory(self, request, context):
        """保存记忆"""
        try:
            result = await self.manager.save_memory(
                tenant_id=request.tenant_id,
                user_id=request.user_id,
                session_id=request.session_id,
                content=request.content,
                memory_type=request.memory_type,
                metadata=dict(request.metadata) if request.metadata else {},
            )
            return memory_pb2.SaveMemoryResponse(
                memory_id=result["memory_id"],
                status=result["status"],
                error=result.get("error", ""),
            )
        except Exception as e:
            logger.error(f"SaveMemory 错误: {e}")
            return memory_pb2.SaveMemoryResponse(
                memory_id="",
                status="failed",
                error=str(e),
            )

    async def QueryMemory(self, request, context):
        """查询记忆"""
        try:
            results = await self.manager.query_memory(
                tenant_id=request.tenant_id,
                user_id=request.user_id,
                query=request.query,
                top_k=request.top_k if request.top_k > 0 else 5,
                memory_type=request.memory_type if request.memory_type else "all",
            )
            memory_results = []
            for r in results:
                memory_results.append(memory_pb2.MemoryResult(
                    memory_id=r["memory_id"],
                    content=r["content"],
                    relevance=r["relevance"],
                    created_at=r.get("created_at", ""),
                    memory_type=r.get("memory_type", ""),
                    metadata=r.get("metadata", {}),
                ))
            return memory_pb2.QueryMemoryResponse(results=memory_results)
        except Exception as e:
            logger.error(f"QueryMemory 错误: {e}")
            return memory_pb2.QueryMemoryResponse(results=[])

    async def HealthCheck(self, request, context):
        """健康检查"""
        return common_pb2.HealthCheckResponse(
            status="ok",
            version="1.0.0",
            uptime="0h0m0s",
        )


class KnowledgeEngineServicer:
    """知识库引擎 gRPC 服务"""

    def __init__(self):
        from app.rag.builder import rag_builder
        self.builder = rag_builder

    async def BuildDocument(self, request, context):
        """构建文档索引（流式返回进度）"""
        try:
            async for event in self.builder.build_document(
                kb_id=request.kb_id,
                doc_id=request.doc_id,
                content=request.content,
                file_type=request.file_type,
                filename=request.filename,
                tenant_id=request.tenant_id,
                vector_db=request.vector_db or "milvus",
            ):
                if event["type"] == "progress":
                    yield knowledge_pb2.BuildDocumentResponse(
                        progress=knowledge_pb2.BuildProgress(
                            step=event["step"],
                            progress=event["progress"],
                            message=event.get("message", ""),
                        )
                    )
                elif event["type"] == "complete":
                    yield knowledge_pb2.BuildDocumentResponse(
                        complete=knowledge_pb2.BuildComplete(
                            chunk_count=event["chunk_count"],
                            char_count=event["char_count"],
                            page_count=event.get("page_count", 0),
                            status="completed",
                        )
                    )
                elif event["type"] == "error":
                    yield knowledge_pb2.BuildDocumentResponse(
                        error=event["message"]
                    )
        except Exception as e:
            logger.error(f"BuildDocument 错误: {e}")
            yield knowledge_pb2.BuildDocumentResponse(error=str(e))

    async def Query(self, request, context):
        """查询知识库"""
        try:
            vector_db = request.vector_db or "milvus"

            if vector_db == "milvus":
                results = await self.builder.query_milvus(
                    kb_id=request.kb_id,
                    query=request.query,
                    top_k=request.top_k if request.top_k > 0 else 5,
                    threshold=request.threshold if request.threshold > 0 else 0.5,
                )
            elif vector_db == "qdrant":
                results = await self.builder.query_qdrant(
                    kb_id=request.kb_id,
                    query=request.query,
                    top_k=request.top_k if request.top_k > 0 else 5,
                    threshold=request.threshold if request.threshold > 0 else 0.5,
                )
            else:
                results = []

            query_results = []
            for r in results:
                query_results.append(knowledge_pb2.QueryResult(
                    id=r.get("id", ""),
                    content=r.get("content", ""),
                    doc_id=r.get("doc_id", ""),
                    chunk_index=r.get("chunk_index", 0),
                    score=r.get("score", 0.0),
                ))

            return knowledge_pb2.QueryResponse(results=query_results)

        except Exception as e:
            logger.error(f"Query 错误: {e}")
            return knowledge_pb2.QueryResponse(results=[])

    async def HealthCheck(self, request, context):
        """健康检查"""
        return common_pb2.HealthCheckResponse(
            status="ok",
            version="1.0.0",
            uptime="0h0m0s",
        )
