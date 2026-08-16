# RAG 检索测试
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.rag.retriever import RAGRetriever


class TestRAGRetriever:
    """测试 RAG 检索器"""

    def test_split_text_basic(self):
        """测试基本文本分块"""
        retriever = RAGRetriever()
        text = "Hello World " * 100  # 1200 chars
        chunks = retriever._split_text(text, chunk_size=500, chunk_overlap=100)
        assert len(chunks) > 1
        for chunk in chunks:
            assert len(chunk) <= 500

    def test_split_text_short(self):
        """测试短文本分块"""
        retriever = RAGRetriever()
        text = "Hello World"
        chunks = retriever._split_text(text, chunk_size=500, chunk_overlap=100)
        assert len(chunks) == 1
        assert chunks[0] == "Hello World"

    def test_split_text_empty(self):
        """测试空文本分块"""
        retriever = RAGRetriever()
        chunks = retriever._split_text("", chunk_size=500, chunk_overlap=100)
        assert len(chunks) == 0

    def test_split_text_overlap(self):
        """测试分块重叠"""
        retriever = RAGRetriever()
        text = "A" * 1000
        chunks = retriever._split_text(text, chunk_size=500, chunk_overlap=200)
        assert len(chunks) == 3
        # 验证重叠
        assert chunks[0][-200:] == chunks[1][:200]

    @pytest.mark.asyncio
    async def test_index_document_milvus_unavailable(self):
        """测试 Milvus 不可用时的文档索引"""
        retriever = RAGRetriever()
        # Milvus 未连接，应返回降级结果
        result = await retriever.index_document(
            tenant_id="test",
            document_id="doc1",
            content="Hello World",
        )
        assert result["status"] == "indexed"  # 跳过但标记为 indexed
        assert result["chunks_count"] > 0

    @pytest.mark.asyncio
    async def test_retrieve_milvus_unavailable(self):
        """测试 Milvus 不可用时的检索"""
        retriever = RAGRetriever()
        results = await retriever.retrieve(
            tenant_id="test",
            query="Hello",
        )
        assert results == []


class TestTextChunking:
    """测试文本分块算法"""

    def test_markdown_splitting(self):
        """测试 Markdown 文本分块"""
        retriever = RAGRetriever()
        md_text = """# Title

## Section 1

Content 1

## Section 2

Content 2"""
        chunks = retriever._split_text(md_text, chunk_size=50, chunk_overlap=10)
        assert len(chunks) > 1

    def test_code_splitting(self):
        """测试代码分块"""
        retriever = RAGRetriever()
        code = """def hello():
    print("Hello")

def world():
    print("World")"""
        chunks = retriever._split_text(code, chunk_size=40, chunk_overlap=10)
        assert len(chunks) > 1


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
