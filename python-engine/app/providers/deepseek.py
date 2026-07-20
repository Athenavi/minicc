# DeepSeek Provider — OpenAI 兼容 API
from __future__ import annotations

from app.providers.openai import OpenAIProvider


class DeepSeekProvider(OpenAIProvider):
    """DeepSeek 使用 OpenAI 兼容接口，直接继承 OpenAIProvider"""

    name = "deepseek"

    def __init__(self, api_key: str, base_url: str = "https://api.deepseek.com"):
        super().__init__(api_key=api_key, base_url=base_url)
