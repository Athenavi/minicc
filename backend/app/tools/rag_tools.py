"""RAG 检索工具 — 文档加载/分块/嵌入/搜索。"""

from __future__ import annotations

from typing import Optional

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext
from app.rag.loader import CompositeLoader
from app.rag.chunker import RecursiveChunker
from app.rag.vector_store import VectorStore


class _Empty(BaseModel):
    pass


class RagLoadInput(BaseModel):
    source: str = Field(description="File path, directory path, or URL to load")
    chunk: bool = Field(default=True, description="Whether to chunk the loaded document")


class RagSearchInput(BaseModel):
    query: str = Field(description="Search query")
    top_k: int = Field(default=5, description="Number of results")


class RagLoadTool(BaseTool):
    name = "rag_load"
    description = "Load documents from files, directories, or URLs into the RAG knowledge base."
    input_schema = RagLoadInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: RagLoadInput, context: ToolUseContext | None = None) -> ToolResult:
        loader = CompositeLoader()
        docs = await loader.load(input_data.source)
        if input_data.chunk:
            chunker = RecursiveChunker()
            for doc in docs:
                chunker.chunk(doc)
        return ToolResult(tool_call_id="", output=f"[rag] Loaded {len(docs)} document(s) from {input_data.source}")


class RagSearchTool(BaseTool):
    name = "rag_search"
    description = "Search the RAG knowledge base for relevant documents."
    input_schema = RagSearchInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: RagSearchInput, context: ToolUseContext | None = None) -> ToolResult:
        from app.rag.embedder import OpenAIEmbedder
        embedder = OpenAIEmbedder()
        vs = VectorStore(embedder)
        results = await vs.search(input_data.query, top_k=input_data.top_k)
        if not results:
            return ToolResult(tool_call_id="", output="[rag] No results found. Load documents first with rag_load.")
        lines = [f"[rag] Found {len(results)} result(s):"]
        for r in results:
            lines.append(f"  • Score: {r['score']} | {r['content'][:100]}...")
        return ToolResult(tool_call_id="", output="\n".join(lines))


def register_rag_tools(registry) -> None:
    registry.register(RagLoadTool())
    registry.register(RagSearchTool())
