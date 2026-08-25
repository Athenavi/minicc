# Python AI 寮曟搸閰嶇疆
from pathlib import Path

from pydantic import ConfigDict, model_validator
from pydantic_settings import BaseSettings
from typing import Optional


def _find_env_file() -> str:
    """浠庡紩鎿庝唬鐮佷綅缃悜涓婃帰娴嬮」鐩牴 .env锛堝紩鎿庡彲鑳戒互浠绘剰 cwd 鍚姩锛夈€?

    淇锛歳un.py 浠?python-engine/ 涓?cwd 鍚姩寮曟搸锛宲ydantic 榛樿璇?cwd 鐨?
    .env锛堜笉瀛樺湪锛夊鑷?LLM_API_KEY 绛夊叏閮ㄤ负绌?鈫?provider 鏈敞鍐屻€?
    杩欓噷浠?app/config.py 鍚戜笂鎵惧埌椤圭洰鏍癸紙鍚?run.py 鐨勭洰褰曪級鐨?.env銆?
    """
    cur = Path(__file__).resolve().parent  # python-engine/app
    for p in (cur.parent, cur.parent.parent, cur.parent.parent.parent):
        env = p / ".env"
        if env.is_file():
            return str(env)
    return ""


class Settings(BaseSettings):
    """Python AI 寮曟搸閰嶇疆锛屾敮鎸佺幆澧冨彉閲忚鐩?""

    # 鈹€鈹€ HTTP Server 鈹€鈹€
    http_port: int = 8000
    # 榛樿浠呯粦鍥炵幆鍦板潃锛岄伩鍏嶈鏆撮湶鍒板缃戯紙鐢熶骇闇€缁忓弽鍚戜唬鐞?缃戝叧锛?
    http_host: str = "127.0.0.1"

    # 鈹€鈹€ Redis 鈹€鈹€
    redis_url: str = "redis://localhost:6379"
    redis_max_connections: int = 50

    # 鈹€鈹€ PostgreSQL 鈹€鈹€
    # 榛樿绌猴細寮哄埗閫氳繃 .env / POSTGRES_DSN 鐜鍙橀噺鎻愪緵锛岄伩鍏嶈鐢ㄥ紑鍙戝簱
    postgres_dsn: str = ""
    # 杩炴帴姹犲ぇ灏忥紙姘村钩鎵╁睍鏃舵敞鎰忔€昏繛鎺ユ暟涓嶈秴杩囨暟鎹簱涓婇檺锛?
    db_pool_min_size: int = 5
    db_pool_max_size: int = 20

    # 鈹€鈹€ Milvus 鈹€鈹€
    milvus_address: str = "localhost:19530"
    milvus_collection: str = "knowledge_base"

    # 鈹€鈹€ LLM Provider API Keys 鈹€鈹€
    anthropic_api_key: str = ""
    anthropic_base_url: str = ""
    openai_api_key: str = ""
    openai_base_url: str = ""
    deepseek_api_key: str = ""
    deepseek_base_url: str = "https://api.deepseek.com"

    # 鈹€鈹€ 缁熶竴 LLM 閰嶇疆锛堜笌 Go Gateway 鍏辩敤鍙橀噺鍚嶏級鈹€鈹€
    llm_provider: str = "openai"
    llm_api_key: str = ""
    llm_base_url: str = "https://api.deepseek.com"
    llm_model: str = "deepseek-v4-flash"

    # 鈹€鈹€ Agent 閰嶇疆 鈹€鈹€
    max_turns: int = 10
    default_model: str = "claude-sonnet-4-20250514"
    default_max_tokens: int = 4096
    default_temperature: float = 0.1

    # 鈹€鈹€ RAG 閰嶇疆 鈹€鈹€
    embedding_model: str = "text-embedding-3-small"
    embedding_dim: int = 1536  # 宓屽叆缁村害锛屽彲閰嶇疆
    chunk_size: int = 1000
    chunk_overlap: int = 200
    default_top_k: int = 5
    default_threshold: float = 0.7
    # 榛樿鍚戦噺鏁版嵁搴? milvus | pgvector
    vector_db_type: str = "milvus"
    pgvector_table: str = "knowledge_chunk_vectors"
    # 鏈湴宓屽叆妯″瀷璺緞锛圔GE/Jina 绛夛級锛屼负绌哄垯璺宠繃鏈湴宓屽叆銆佸洖閫€鍒?API
    local_embedding_model: str = ""

    # 鈹€鈹€ 璁板繂閰嶇疆 鈹€鈹€
    short_term_ttl: int = 604800  # 7 澶╋紙绉掞級
    long_term_ttl: int = 0  # 0 = 姘镐笉杩囨湡
    # L2 妗ｆ鍗★紙鐢ㄦ埛闀挎湡璁板繂锛夊閲忎笌鏁寸悊鍙傛暟
    memory_profile_max_items: int = 200      # 姣忕敤鎴锋潯鐩蒋涓婇檺锛堣秴鍑烘寜 缃俊搴γ楁柊杩戝害 娣樻卑 derived锛?
    memory_archive_days: int = 180           # 瓒呮湡鏈紩鐢ㄤ笖浣庣疆淇?鈫?褰掓。
    memory_dedup_threshold: float = 0.95     # cosine 瓒呰繃璇ラ槇鍊煎垽瀹氳繎閲嶅 鈫?鏁寸悊鏃跺悎骞?
    memory_search_min_cosine: float = 0.30   # 璇箟妫€绱㈠彫鍥炰笅闄?
    # L3 杩戞湡瀵硅瘽鎽樿锛堣涔夋绱㈠眰锛?
    memory_summary_top_k: int = 5            # 姣忓洖鍚堝彫鍥炵殑鎽樿鏉℃暟
    memory_summary_retention_days: int = 90  # 瓒呮湡鏈懡涓?鈫?archived
    memory_summary_min_cosine: float = 0.45  # 鎽樿璇箟妫€绱㈠彫鍥炰笅闄?
    memory_recall_token_budget: int = 8000   # L2+L3 鎬绘敞鍏ラ绠楋紙tokens锛?
    memory_consolidate_batch: int = 32       # 宸╁浐鏀掓壒澶у皬

    # 鈹€鈹€ LLM Gateway 缂撳瓨 鈹€鈹€
    cache_l1_capacity: int = 2048
    cache_l2_ttl: int = 3600
    semantic_cache_threshold: float = 0.95
    semantic_cache_prefix_dims: int = 64

    # 鈹€鈹€ 鎻掍欢/鍐呴儴绔偣閴存潈 鈹€鈹€
    # 涓?Go 缃戝叧 LLM_GATEWAY_KEY 涓€鑷达紱鎻掍欢 reload 绛夊唴閮ㄧ鐐规牎楠?X-API-Key
    llm_gateway_key: str = ""

    # 鈹€鈹€ 闄愭祦 鈹€鈹€
    rate_limit_rpm: int = 60  # requests per minute per tenant
    rate_limit_rps: int = 10  # requests per second per tenant

    # 鈹€鈹€ 闃熷垪 鈹€鈹€
    queue_worker_concurrency: int = 10

    # 鈹€鈹€ JWT 鈹€鈹€
    # 榛樿绌猴細鏈樉寮忛厤缃椂鑻ュ瓨鍦?APP_SECRET锛屽垯鐢卞叾娲剧敓锛堜笌 Go 缃戝叧 deriveSubsecret 涓€鑷达級锛?
    # 淇濊瘉寮曟搸绛惧彂鐨?token 涓?Go 缃戝叧鍏变韩鍚屼竴瀵嗛挜
    jwt_secret: str = ""

    # 鈹€鈹€ 閮ㄧ讲绾т富瀵嗛挜锛堜笌 Go 缃戝叧鍏变韩锛?env 鐨?APP_SECRET 缁?extra="ignore" 涔嬪闇€鏄惧紡澹版槑鎵嶈兘璇诲彇锛夆攢鈹€
    app_secret: str = ""

    # 鈹€鈹€ Go 缃戝叧鍐呴儴浜掍俊 token 鈹€鈹€
    # 涓?Go 缃戝叧鍏变韩锛孭ython 浠呭湪 X-Internal-Token 鍖归厤鏃舵墠鎺ュ彈
    # 缃戝叧閫忎紶鐨??tenant_id= / ?user_id= query 韬唤锛堥槻鐩磋繛缁曡繃锛?
    internal_token: str = ""

    # 鈹€鈹€ Go 缃戝叧鍐呴儴閰嶇疆涓嬪彂绔偣锛堝紩鎿庡惎鍔ㄦ椂鎷夊彇鍚庡彴銆岀郴缁熻缃€嶄腑鐨?python 鍒嗙被閰嶇疆锛夆攢鈹€
    gateway_internal_url: str = "http://127.0.0.1:8080"

    # 鈹€鈹€ 鍙娴嬫€?鈹€鈹€
    log_level: str = "INFO"
    otel_endpoint: str = ""  # e.g. "http://otel-collector:4317"

    # 鈹€鈹€ 瀹炰緥鏍囪瘑锛圞8s 娉ㄥ叆锛夆攢鈹€
    pod_name: str = ""
    instance_id: str = ""

    # extra="ignore"锛氶」鐩牴 .env 娣锋湁 Go 缃戝叧鍙橀噺锛圥ORT/CORS_ORIGINS 绛夛級锛?
    # Python 寮曟搸鍙彇鑷繁澹版槑鐨勫瓧娈碉紝鍏朵綑蹇界暐
    model_config = ConfigDict(env_prefix="", case_sensitive=False, extra="ignore", env_file=_find_env_file())

    @model_validator(mode="after")
    def _validate_security_defaults(self):
        """P0 瀹夊叏 fail-fast锛欽WT secret 蹇呴』鏄惧紡閰嶇疆涓旈潪寮卞€笺€?

        鍘嗗彶闂锛氶粯璁?jwt_secret='dev-secret-change-in-production' 浼氬鑷?
        浠讳綍鐭ラ亾璇ュ€肩殑鏀诲嚮鑰呭彲浼€犱换鎰忕鎴疯韩浠界殑 JWT銆侴o 绔凡瀵瑰急鍊奸粦鍚嶅崟鎷掔粷锛?
        Python 绔繀椤讳繚鎸佷竴鑷存牎楠岋紝鍚﹀垯涓€鏃?Python 绔彛鐩磋繛鏆撮湶鍗冲彲琚粫杩囥€?

        鐢熶骇妯″紡锛圓PP_ENV=production 鎴?PYTHON_ENV=production锛変笅棰濆绂佹
        sslmode=disable锛涘紑鍙戞ā寮忓厑璁革紝渚夸簬鏈湴 docker postgres銆?
        """
        import base64
        import hashlib
        import hmac
        import os
        is_prod = os.getenv("APP_ENV", "").lower() == "production" or \
                  os.getenv("PYTHON_ENV", "").lower() == "production"

        # JWT_SECRET 鏈樉寮忛厤缃椂锛岀敱 APP_SECRET 娲剧敓锛堜笌 Go 缃戝叧 deriveSubsecret 瀹屽叏涓€鑷达細
        # HMAC-SHA256(APP_SECRET, "chiron-jwt") 鈫?base64url 鏃?padding锛夈€?
        # 杩欐牱銆屼粎閰嶇疆 APP_SECRET銆嶇殑閮ㄧ讲妯″瀷涓嬪紩鎿庝篃鑳藉惎鍔紝涓斾笌缃戝叧鍏变韩绛惧悕瀵嗛挜銆?
        if not self.jwt_secret and self.app_secret:
            self.jwt_secret = base64.urlsafe_b64encode(
                hmac.new(self.app_secret.encode("utf-8"), b"chiron-jwt", hashlib.sha256).digest()
            ).decode("ascii").rstrip("=")

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
        # PostgreSQL锛氬紑鍙?瀹夎妯″紡鍏佽鏈厤缃紙寮曟搸闄嶇骇鍚姩锛孭G 鐩稿叧鍔熻兘涓嶅彲鐢紝
        # main.py 浠呭湪 postgres_dsn 闈炵┖鏃跺垵濮嬪寲杩炴帴姹狅級锛涚敓浜фā寮忎粛寮哄埗銆?
        if not self.postgres_dsn and is_prod:
            raise ValueError(
                "POSTGRES_DSN must be explicitly set via env or .env (production)"
            )
        if is_prod and "sslmode=disable" in self.postgres_dsn:
            raise ValueError(
                "POSTGRES_DSN with sslmode=disable is forbidden in production; "
                "use sslmode=require or verify-full"
            )
        return self

    @model_validator(mode="after")
    def _resolve_llm_fallback(self):
        """LLM_* 閰嶇疆鍥為€€锛氬綋 OPENAI_* 涓虹┖鏃朵娇鐢?LLM_*锛屼笖 LLM_MODEL 瑕嗙洊 default_model"""
        if self.llm_api_key and not self.openai_api_key:
            self.openai_api_key = self.llm_api_key
            # 浠呭綋鐢ㄦ埛鏈樉寮忛厤缃?OPENAI_BASE_URL 鏃舵墠鐢?LLM_BASE_URL 鍥為€€绔偣锛?
            # 鐢ㄦ埛鏄惧紡璁剧疆鐨勮嚜瀹氫箟 OpenAI 鍏煎绔偣锛圤PENAI_BASE_URL锛変笉搴旇
            # LLM_BASE_URL锛堥粯璁ゆ寚鍚?DeepSeek锛夐潤榛樿鐩栥€?
            if "openai_base_url" not in self.model_fields_set:
                self.openai_base_url = self.llm_base_url or self.openai_base_url
        # 浠呭綋 llm_model 琚樉寮忚缃紙闈為粯璁ゅ€硷級鏃舵墠瑕嗙洊 default_model锛?
        # 鍚﹀垯鐢ㄦ埛閫氳繃 DEFAULT_MODEL 鐜鍙橀噺璁剧疆鐨勫€间細琚潤榛樺拷鐣ャ€?
        if "llm_model" in self.model_fields_set:
            self.default_model = self.llm_model
        return self


settings = Settings()


def _load_gateway_config() -> dict:
    """浠?Go 缃戝叧鍐呴儴绔偣鎷夊彇鍚庡彴銆岀郴缁熻缃€嶇殑 python 鍒嗙被閰嶇疆銆?

    杩斿洖涓?Settings 瀛楁鍚嶄竴鑷寸殑鎵佸钩 dict锛堟晱鎰熼敭宸茬敱缃戝叧鐢?APP_SECRET 瑙ｅ瘑锛夈€?
    缃戝叧涓嶅彲杈炬垨鏈厤缃?internal_token 鏃惰繑鍥炵┖锛坒ail-open锛屼娇鐢?env 榛樿鍊硷紝涓嶉樆鏂惎鍔級銆?
    """
    import json
    import logging
    import urllib.request

    if not settings.internal_token:
        return {}
    url = f"{settings.gateway_internal_url.rstrip('/')}/v1/internal/engine-config"
    try:
        req = urllib.request.Request(url, headers={"X-Internal-Token": settings.internal_token})
        with urllib.request.urlopen(req, timeout=1) as resp:
            payload = json.loads(resp.read().decode())
        data = payload.get("data", payload) if isinstance(payload, dict) else {}
        return {k: v for k, v in data.items() if v is not None}
    except Exception as exc:  # noqa: BLE001 - 閰嶇疆涓嬪彂澶辫触涓嶉樆鏂紩鎿庡惎鍔?
        logging.getLogger(__name__).warning("load engine config from gateway failed: %s", exc)
        return {}


# 鍚堝苟缃戝叧涓嬪彂鐨勯厤缃細浠呮帴鍙?Settings 宸插０鏄庣殑瀛楁锛孌B/env 涔嬪涓嶅紩鍏ヤ换鎰忛敭銆?
_db_overrides = _load_gateway_config()
_allowed = set(Settings.model_fields.keys())
_merged = {k: v for k, v in _db_overrides.items() if k in _allowed}
if _merged:
    settings = settings.model_copy(update=_merged)
    import logging as _log
    _log.getLogger(__name__).info("applied %d engine settings from gateway", len(_merged))

