"""LLM client 兼容桩（供 RAG 等模块导入）。

当前项目已改用 GatewayRouter / LLMProvider 抽象，本模块仅保留最小接口，
避免旧代码 import 报错；生产环境请替换为真实实现。
"""
from __future__ import annotations

from dataclasses import dataclass
from typing import List

from app.config import settings


@dataclass
class _StubLLMClient:
    async def embed(self, text: str) -> List[float]:
        # 返回零向量，仅用于占位；真实实现应调用 embedding 模型。
        return [0.0] * settings.embedding_dim


llm_client = _StubLLMClient()
