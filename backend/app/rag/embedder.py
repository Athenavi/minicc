"""向量化与嵌入 — 多 Provider 支持。"""

from __future__ import annotations

import hashlib
from typing import Optional

from pydantic import BaseModel, Field


class EmbeddingResult(BaseModel):
    """嵌入结果。"""
    vector: list[float]
    model: str = ""
    dimensions: int = 0


class BaseEmbedder:
    """嵌入器基类。"""

    async def embed(self, text: str) -> EmbeddingResult:
        raise NotImplementedError

    async def embed_batch(self, texts: list[str]) -> list[EmbeddingResult]:
        return [await self.embed(t) for t in texts]


class OpenAIEmbedder(BaseEmbedder):
    """OpenAI / DeepSeek 兼容嵌入 API。"""

    def __init__(self, api_key: str = "", model: str = "text-embedding-3-small", base_url: str = ""):
        self.api_key = api_key
        self.model = model
        self.base_url = base_url

    async def embed(self, text: str) -> EmbeddingResult:
        import httpx
        url = f"{self.base_url}/embeddings" if self.base_url else "https://api.openai.com/v1/embeddings"
        headers = {"Authorization": f"Bearer {self.api_key}", "Content-Type": "application/json"}
        async with httpx.AsyncClient(timeout=30) as client:
            resp = await client.post(url, json={"model": self.model, "input": text}, headers=headers)
            data = resp.json()
            vec = data["data"][0]["embedding"]
            return EmbeddingResult(vector=vec, model=self.model, dimensions=len(vec))

    async def embed_batch(self, texts: list[str]) -> list[EmbeddingResult]:
        import httpx
        url = f"{self.base_url}/embeddings" if self.base_url else "https://api.openai.com/v1/embeddings"
        headers = {"Authorization": f"Bearer {self.api_key}", "Content-Type": "application/json"}
        async with httpx.AsyncClient(timeout=120) as client:
            resp = await client.post(url, json={"model": self.model, "input": texts}, headers=headers)
            data = resp.json()
            return [EmbeddingResult(vector=d["embedding"], model=self.model, dimensions=len(d["embedding"])) for d in data["data"]]


def compute_hash(text: str) -> str:
    return hashlib.md5(text.encode()).hexdigest()
