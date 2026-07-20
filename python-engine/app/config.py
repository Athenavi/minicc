# Python AI 引擎配置
from pydantic import ConfigDict, model_validator
from pydantic_settings import BaseSettings
from typing import Optional


class Settings(BaseSettings):
    """Python AI 引擎配置，支持环境变量覆盖"""

    # ── HTTP Server ──
    http_port: int = 8000
    http_host: str = "0.0.0.0"

    # ── Redis ──
    redis_url: str = "redis://localhost:6379"
    redis_max_connections: int = 50

    # ── PostgreSQL ──
    postgres_dsn: str = "postgres://postgres:123456@localhost:5432/minicc0710?sslmode=disable"

    # ── Milvus ──
    milvus_address: str = "localhost:19530"
    milvus_collection: str = "knowledge_base"

    # ── LLM Provider API Keys ──
    anthropic_api_key: str = ""
    anthropic_base_url: str = ""
    openai_api_key: str = ""
    openai_base_url: str = ""
    deepseek_api_key: str = ""
    deepseek_base_url: str = "https://api.deepseek.com"

    # ── 统一 LLM 配置（与 Go Gateway 共用变量名）──
    llm_provider: str = "openai"
    llm_api_key: str = ""
    llm_base_url: str = "https://api.deepseek.com"
    llm_model: str = "deepseek-v4-flash"

    # ── Agent 配置 ──
    max_turns: int = 10
    default_model: str = "claude-sonnet-4-20250514"
    default_max_tokens: int = 4096
    default_temperature: float = 0.1

    # ── RAG 配置 ──
    embedding_model: str = "text-embedding-3-small"
    embedding_dim: int = 1536  # 嵌入维度，可配置
    chunk_size: int = 1000
    chunk_overlap: int = 200
    default_top_k: int = 5
    default_threshold: float = 0.7

    # ── 记忆配置 ──
    short_term_ttl: int = 604800  # 7 天（秒）
    long_term_ttl: int = 0  # 0 = 永不过期

    # ── LLM Gateway 缓存 ──
    cache_l1_capacity: int = 2048
    cache_l2_ttl: int = 3600
    semantic_cache_threshold: float = 0.95
    semantic_cache_prefix_dims: int = 64

    # ── 限流 ──
    rate_limit_rpm: int = 60  # requests per minute per tenant
    rate_limit_rps: int = 10  # requests per second per tenant

    # ── 队列 ──
    queue_worker_concurrency: int = 10

    # ── JWT ──
    jwt_secret: str = "dev-secret-change-in-production"

    # ── 可观测性 ──
    log_level: str = "INFO"
    otel_endpoint: str = ""  # e.g. "http://otel-collector:4317"

    # ── 实例标识（K8s 注入）──
    pod_name: str = ""
    instance_id: str = ""

    model_config = ConfigDict(env_prefix="", case_sensitive=False)

    @model_validator(mode="after")
    def _resolve_llm_fallback(self):
        """LLM_* 配置回退：当 OPENAI_* 为空时使用 LLM_*，且 LLM_MODEL 覆盖 default_model"""
        if self.llm_api_key and not self.openai_api_key:
            self.openai_api_key = self.llm_api_key
            self.openai_base_url = self.llm_base_url or self.openai_base_url
        # 仅当 llm_model 被显式设置（非默认值）时才覆盖 default_model，
        # 否则用户通过 DEFAULT_MODEL 环境变量设置的值会被静默忽略。
        if "llm_model" in self.model_fields_set:
            self.default_model = self.llm_model
        return self


settings = Settings()
