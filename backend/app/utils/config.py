"""全局配置加载 — 基于 pydantic-settings。"""

from __future__ import annotations

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """MiniCC 全局配置。所有配置通过环境变量注入，前缀 MINICC_。"""

    model_config = SettingsConfigDict(env_prefix="MINICC_", env_file=".env", extra="ignore")

    # LLM 配置
    llm_provider: str = "anthropic"
    llm_api_key: str = ""
    llm_model: str = "claude-sonnet-4-20250514"

    # 工作区
    workspace_dir: str = "."

    # 存储
    redis_url: str = "redis://localhost:6379/0"

    # 运行限制
    max_tool_rounds: int = 25
    max_tokens: int = 8192

    # 日志
    log_level: str = "INFO"


settings = Settings()
