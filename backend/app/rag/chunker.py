"""文档分块策略 — Recursive / Token / Semantic。对标 Dify 分块。"""

from __future__ import annotations

import re
from typing import Optional

from pydantic import BaseModel, Field

from app.rag.loader import Document


class Chunk(BaseModel):
    """一个分块。"""
    content: str
    index: int = 0
    metadata: dict = Field(default_factory=dict)


class BaseChunker:
    """分块器基类。"""
    def chunk(self, doc: Document) -> list[Chunk]:
        raise NotImplementedError


class RecursiveChunker(BaseChunker):
    """递归分块 — 按段落 → 句子 → 固定长度降级。"""

    def __init__(self, chunk_size: int = 1000, chunk_overlap: int = 200):
        self.chunk_size = chunk_size
        self.chunk_overlap = chunk_overlap

    def chunk(self, doc: Document) -> list[Chunk]:
        text = doc.content
        if len(text) <= self.chunk_size:
            return [Chunk(content=text, index=0, metadata=doc.metadata)]

        chunks = []
        # Try paragraph split first
        paragraphs = re.split(r"\n\s*\n", text)
        current = ""
        idx = 0

        for para in paragraphs:
            if len(current) + len(para) < self.chunk_size:
                current += ("\n\n" if current else "") + para
            else:
                if current:
                    chunks.append(Chunk(content=current, index=idx, metadata=doc.metadata))
                    idx += 1
                    # Keep overlap
                    current = current[-self.chunk_overlap:] + "\n\n" + para if self.chunk_overlap else para
                else:
                    # Single paragraph exceeds chunk_size — split by sentence
                    for sentence in re.split(r"(?<=[.!?])\s+", para):
                        if len(current) + len(sentence) < self.chunk_size:
                            current += (" " if current else "") + sentence
                        else:
                            if current:
                                chunks.append(Chunk(content=current, index=idx, metadata=doc.metadata))
                                idx += 1
                            current = sentence

        if current:
            chunks.append(Chunk(content=current, index=idx, metadata=doc.metadata))

        return chunks


class TokenChunker(BaseChunker):
    """基于 token 的分块（4 字符 ≈ 1 token）。"""

    def __init__(self, chunk_tokens: int = 500, overlap_tokens: int = 100):
        self.chunk_tokens = chunk_tokens
        self.overlap_tokens = overlap_tokens

    def chunk(self, doc: Document) -> list[Chunk]:
        text = doc.content
        char_limit = self.chunk_tokens * 4
        overlap_chars = self.overlap_tokens * 4

        chunks = []
        start = 0
        idx = 0
        while start < len(text):
            end = min(start + char_limit, len(text))
            chunks.append(Chunk(content=text[start:end], index=idx, metadata=doc.metadata))
            idx += 1
            start = end - overlap_chars if end < len(text) else len(text)

        return chunks
