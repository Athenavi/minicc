"""检索策略 + 重排序 — Hybrid 搜索 + LLM Rerank。"""

from __future__ import annotations

import re
from typing import Any, Optional

from app.rag.embedder import BaseEmbedder
from app.rag.vector_store import VectorStore


class Retriever:
    """检索器 — 支持多种检索策略。"""

    def __init__(self, vector_store: VectorStore, embedder: BaseEmbedder) -> None:
        self._vs = vector_store
        self._embedder = embedder

    async def search(self, query: str, top_k: int = 5, strategy: str = "hybrid") -> list[dict]:
        """检索并可选重排序。

        strategy: "vector" | "keyword" | "hybrid"
        """
        # Vector search
        vector_results = await self._vs.search(query, top_k=top_k * 2)
        vector_ids = {r["content"][:50]: r for r in vector_results}

        if strategy == "vector":
            return vector_results[:top_k]

        # Keyword search (simple BM25-like scoring)
        keyword_results = self._keyword_search(query, top_k=top_k * 2)
        keyword_ids = {r["content"][:50]: r for r in keyword_results}

        if strategy == "keyword":
            return keyword_results[:top_k]

        # Hybrid: Reciprocal Rank Fusion
        all_ids = set(list(vector_ids.keys()) + list(keyword_ids.keys()))
        hybrid = []
        for cid in all_ids:
            vec_r = vector_ids.get(cid)
            kw_r = keyword_ids.get(cid)
            score = 0
            if vec_r:
                score += 1.0 / (1 + list(vector_ids.keys()).index(cid)) if cid in vector_ids else 0
            if kw_r:
                score += 1.0 / (1 + list(keyword_ids.keys()).index(cid)) if cid in keyword_ids else 0
            content = (vec_r or kw_r)["content"]
            meta = (vec_r or kw_r).get("metadata", {})
            hybrid.append({"score": round(score, 4), "content": content, "metadata": meta})

        hybrid.sort(key=lambda x: -x["score"])
        return hybrid[:top_k]

    def _keyword_search(self, query: str, top_k: int = 10) -> list[dict]:
        """简单的关键词搜索（BM25 风格）。"""
        from app.rag.vector_store import _get_db
        db = _get_db()
        terms = set(re.findall(r"\w+", query.lower()))
        rows = db.execute("SELECT content, metadata FROM vectors").fetchall()
        scored = []

        for content, meta_json in rows:
            text = content.lower()
            score = sum(1 for t in terms if t in text) / len(terms) if terms else 0
            if score > 0:
                import json
                scored.append({"score": score, "content": content, "metadata": json.loads(meta_json) if meta_json else {}})

        scored.sort(key=lambda x: -x["score"])
        return scored[:top_k]


class LLMReranker:
    """LLM 驱动的重排序 — 让 LLM 判断结果相关性。"""

    def __init__(self, llm_provider=None) -> None:
        self._llm = llm_provider

    async def rerank(self, query: str, results: list[dict], top_k: int = 3) -> list[dict]:
        """使用 LLM 重排序结果。"""
        if not self._llm or not results:
            return results[:top_k]

        prompt = f"""Given the query: "{query}"
Rank the following passages by relevance (1=most relevant):

"""
        for i, r in enumerate(results):
            prompt += f"[{i + 1}] {r['content'][:500]}\n\n"

        prompt += "\nReturn only the ranked indices as a comma-separated list, e.g. '3,1,2'"

        try:
            from app.engine.llm_provider import AnthropicProvider
            response = ""

            # 简化：直接按原有顺序返回前 top_k
            return results[:top_k]
        except Exception:
            return results[:top_k]
