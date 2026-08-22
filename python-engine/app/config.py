# Python AI 引擎配置
from pathlib import Path

from pydantic import ConfigDict, model_validator
from pydantic_settings import BaseSettings
from typing import Optional


def _find_env_file() -> str:
    """从引擎代码位置向上探测项目根 .env（引擎可能以任意 cwd 启动）。

    修复：run.py 以 python-engine/ 为 cwd 启动引擎，pydantic 默认读 cwd 的
    .env（不存在）导致 LLM_API_KEY 等全部为空 → provider 未注册。
    这里从 app/config.py 向上找到项目根（含 run.py 的目录）的 .env。
    """
    cur = Path(__file__).resolve().parent  # python-engine/app
    for p in (cur.parent, cur.parent.parent, cur.parent.parent.parent):
        env = p / ".env"
        if env.is_file():
            return str(env)
    return ""


class Settings(BaseSettings):
    """Python AI 引擎配置，支持环境变量覆盖"""

    # ── HTTP Server ──
    http_port: int = 8000
    # 默认仅绑回环地址，避免误暴露到外网（生产需经反向代理/网关）
    http_host: str = "127.0.0.1"

    # ── Redis ──
    redis_url: str = "redis://localhost:6379"
    redis_max_connections: int = 50

    # ── PostgreSQL ──
    # 默认空：强制通过 .env / POSTGRES_DSN 环境变量提供，避免误用开发库
    postgres_dsn: str = ""

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
    # 默认向量数据库: milvus | pgvector
    vector_db_type: str = "milvus"
    pgvector_table: str = "knowledge_chunk_vectors"
    # 本地嵌入模型路径（BGE/Jina 等），为空则跳过本地嵌入、回退到 API
    local_embedding_model: str = ""

    # ── 记忆配置 ──
    short_term_ttl: int = 604800  # 7 天（秒）
    long_term_ttl: int = 0  # 0 = 永不过期
    # L2 档案卡（用户长期记忆）容量与整理参数
    memory_profile_max_items: int = 200      # 每用户条目软上限（超出按 置信度×新近度 淘汰 derived）
    memory_archive_days: int = 180           # 超期未引用且低置信 → 归档
    memory_dedup_threshold: float = 0.95     # cosine 超过该阈值判定近重复 → 整理时合并
    memory_search_min_cosine: float = 0.30   # 语义检索召回下限
    # L3 近期对话摘要（语义检索层）
    memory_summary_top_k: int = 5            # 每回合召回的摘要条数
    memory_summary_retention_days: int = 90  # 超期未命中 → archived
    memory_summary_min_cosine: float = 0.45  # 摘要语义检索召回下限
    memory_recall_token_budget: int = 8000   # L2+L3 总注入预算（tokens）
    memory_consolidate_batch: int = 32       # 巩固攒批大小

    # ── LLM Gateway 缓存 ──
    cache_l1_capacity: int = 2048
    cache_l2_ttl: int = 3600
    semantic_cache_threshold: float = 0.95
    semantic_cache_prefix_dims: int = 64

    # ── 插件/内部端点鉴权 ──
    # 与 Go 网关 LLM_GATEWAY_KEY 一致；插件 reload 等内部端点校验 X-API-Key
    llm_gateway_key: str = ""

    # ── 限流 ──
    rate_limit_rpm: int = 60  # requests per minute per tenant
    rate_limit_rps: int = 10  # requests per second per tenant

    # ── 队列 ──
    queue_worker_concurrency: int = 10

    # ── JWT ──
    # 默认空：生产必须在环境变量或 .env 中显式配置
    jwt_secret: str = ""

    # ── Go 网关内部互信 token ──
    # 与 Go 网关共享，Python 仅在 X-Internal-Token 匹配时才接受
    # 网关透传的 ?tenant_id= / ?user_id= query 身份（防直连绕过）
    internal_token: str = ""

    # ── 可观测性 ──
    log_level: str = "INFO"
    otel_endpoint: str = ""  # e.g. "http://otel-collector:4317"

    # ── 实例标识（K8s 注入）──
    pod_name: str = ""
    instance_id: str = ""

    # extra="ignore"：项目根 .env 混有 Go 网关变量（PORT/CORS_ORIGINS 等），
    # Python 引擎只取自己声明的字段，其余忽略
    model_config = ConfigDict(env_prefix="", case_sensitive=False, extra="ignore", env_file=_find_env_file())

    @model_validator(mode="after")
    def _validate_security_defaults(self):
        """P0 安全 fail-fast：JWT secret 必须显式配置且非弱值。

        历史问题：默认 jwt_secret='dev-secret-change-in-production' 会导致
        任何知道该值的攻击者可伪造任意租户身份的 JWT。Go 端已对弱值黑名单拒绝，
        Python 端必须保持一致校验，否则一旦 Python 端口直连暴露即可被绕过。

        生产模式（APP_ENV=production 或 PYTHON_ENV=production）下额外禁止
        sslmode=disable；开发模式允许，便于本地 docker postgres。
        """
        import os
        is_prod = os.getenv("APP_ENV", "").lower() == "production" or \
                  os.getenv("PYTHON_ENV", "").lower() == "production"

        WEAK_SECRETS = {
            "",
            "dev-secret-change-in-production",
            "changeme",
            "secret",
            "test-secret",
        }
        if not self.jwt_secret or self.jwt_secret in WEAK_SECRETS:
            raise ValueError(
                "JWT_SECRET must be set to a strong value (>=32 chars) "
                "via env or .env; weak/empty defaults are rejected"
            )
        if len(self.jwt_secret) < 32:
            raise ValueError(
                f"JWT_SECRET too short ({len(self.jwt_secret)} chars); "
                "must be at least 32 characters for HMAC-SHA256 security"
            )
        if not self.postgres_dsn:
            raise ValueError(
                "POSTGRES_DSN must be explicitly set via env or .env"
            )
        if is_prod and "sslmode=disable" in self.postgres_dsn:
            raise ValueError(
                "POSTGRES_DSN with sslmode=disable is forbidden in production; "
                "use sslmode=require or verify-full"
            )
        return self

    @model_validator(mode="after")
    def _resolve_llm_fallback(self):
        """LLM_* 配置回退：当 OPENAI_* 为空时使用 LLM_*，且 LLM_MODEL 覆盖 default_model"""
        if self.llm_api_key and not self.openai_api_key:
            self.openai_api_key = self.llm_api_key
            # 仅当用户未显式配置 OPENAI_BASE_URL 时才用 LLM_BASE_URL 回退端点：
            # 用户显式设置的自定义 OpenAI 兼容端点（OPENAI_BASE_URL）不应被
            # LLM_BASE_URL（默认指向 DeepSeek）静默覆盖。
            if "openai_base_url" not in self.model_fields_set:
                self.openai_base_url = self.llm_base_url or self.openai_base_url
        # 仅当 llm_model 被显式设置（非默认值）时才覆盖 default_model，
        # 否则用户通过 DEFAULT_MODEL 环境变量设置的值会被静默忽略。
        if "llm_model" in self.model_fields_set:
            self.default_model = self.llm_model
        return self


settings = Settings()
