# Python AI 寮曟搸鍏ュ彛 鈥?鏃犵姸鎬?FastAPI + 杩炴帴姹?+ 鍋ュ悍妫€鏌?+ 渚濊禆娉ㄥ叆
from __future__ import annotations

import asyncio
import logging
import time
import socket
import uuid
import re
from contextlib import asynccontextmanager

import redis.asyncio as aioredis
import uvicorn
from fastapi import FastAPI, Depends, HTTPException, Request
from fastapi.responses import JSONResponse, StreamingResponse

from app.config import settings
from app.core.container import GlobalContainer, get_container
from app.session_store import SessionStore

# 鍏ㄥ眬浼氳瘽娑堟伅缂撳瓨锛坙ifespan 涓帴鍏?Redis 瀹炵幇澶氬疄渚嬪叡浜級
_session_cache = SessionStore(max_sessions=200)
# 娲昏穬 AgentRuntime 娉ㄥ唽琛紙S 瀹夊叏淇锛氬伐鍏风‘璁ょ鐐规寜 session_id 瀹氫綅 runtime锛?
# 鍊间负 (runtime, owner_user_id)锛岀敤浜庣‘璁ゆ椂鏍￠獙鏉ョ數鑰呮槸鍚︿负浼氳瘽 owner锛?
_ACTIVE_RUNTIMES: dict[str, tuple[AgentRuntime, str]] = {}

logger = logging.getLogger(__name__)

# 鍏ㄥ眬寮曠敤锛坙ifespan 涓垵濮嬪寲锛?
_start_time = time.monotonic()
_redis: aioredis.Redis | None = None
_gateway = None  # GatewayRouter
_queue_worker = None  # asyncio.Task
_mcp_client = None  # MCPClient
_key_pool = None  # SmartAPIKeyPool


# 鈹€鈹€ FastAPI 渚濊禆娉ㄥ叆 鈹€鈹€

async def get_redis() -> aioredis.Redis:
    """鑾峰彇 Redis 杩炴帴锛團astAPI Depends锛?""
    if _redis is None:
        raise RuntimeError("Redis not initialized")
    return _redis


async def get_gateway():
    """鑾峰彇 Gateway Router锛團astAPI Depends锛?""
    if _gateway is None:
        raise RuntimeError("Gateway not initialized")
    return _gateway


async def get_key_pool():
    """鑾峰彇 SmartAPIKeyPool锛團astAPI Depends锛?""
    if _key_pool is None:
        raise RuntimeError("Key pool not initialized")
    return _key_pool


# 鈹€鈹€ 鎻掍欢 MCP 杩炴帴姹狅紙鏃犵姸鎬佸弸濂斤細閰嶇疆瀛樼鐩橈紝杩炴帴涓哄彲閲嶅缓缂撳瓨锛?鈹€鈹€

_plugin_pool = None  # MCPClientPool


def get_plugin_pool():
    """鑾峰彇 MCP 鎻掍欢杩炴帴姹狅紙FastAPI Depends / 鍐呴儴璋冪敤锛夈€?""
    if _plugin_pool is None:
        raise RuntimeError("Plugin pool not initialized")
    return _plugin_pool


def touch_user(user_id: str) -> None:
    """鏍囪鐢ㄦ埛娲昏穬锛堟湁浼氳瘽/宸ュ叿/Agent 璇锋眰鏃惰皟鐢級锛岄┍鍔?MCP 杞鑼冨洿銆?""
    if _plugin_pool is not None and user_id:
        _plugin_pool._tracker.touch(user_id)  # noqa: SLF001 鈥?姹犲唴涓撶敤鍏ュ彛


@asynccontextmanager
async def lifespan(app: FastAPI):
    """搴旂敤鐢熷懡鍛ㄦ湡锛氬惎鍔ㄥ垵濮嬪寲 + 鍏抽棴娓呯悊"""
    global _redis, _gateway, _queue_worker, _key_pool

    # 鈹€鈹€ 1. 鍙娴嬫€?鈹€鈹€
    from app.observability.logging import configure_logging
    from app.observability.tracing import configure_tracing
    from app.observability.metrics import ENGINE_INFO, INSTANCE_UPTIME

    configure_logging(settings.log_level)
    configure_tracing(service_name="python-engine", otlp_endpoint=settings.otel_endpoint)
    ENGINE_INFO.info({"version": "0.1.260825.01", "instance_id": _get_instance_id()})

    logger.info("=" * 60)
    logger.info("Chiron Python AI Engine v0.1.260825.01 鈥?Enterprise Edition")
    logger.info("=" * 60)

    # 鈹€鈹€ 2. Redis 杩炴帴姹?鈹€鈹€
    _redis = aioredis.from_url(
        settings.redis_url,
        decode_responses=False,
        max_connections=settings.redis_max_connections,
    )
    try:
        await _redis.ping()
        logger.info("Redis connected: %s (pool=%d)", settings.redis_url, settings.redis_max_connections)
        # 灏?SessionStore 鎺ュ叆 Redis锛屽疄鐜板瀹炰緥鍏变韩
        _session_cache._redis = _redis
        logger.info("SessionStore switched to Redis backend")
    except Exception as e:
        # 浜у搧鍐崇瓥(2026-08-22)銆孯edis 蹇呴渶銆佹棤闄嶇骇銆嶅凡淇(2026-09)锛氫笌 Go 缃戝叧涓€鑷达紝
        # Redis 涓嶅彲鐢ㄦ椂闄嶇骇鍚姩鈥斺€擲essionStore 鍥為€€杩涚▼鍐呭唴瀛樻ā寮忥紝
        # 渚濊禆 Redis 鐨勫姛鑳斤紙鍒嗗竷寮忛檺娴?浼氳瘽澶氬疄渚嬪叡浜?闃熷垪锛夎繑鍥?503锛屼笉闃绘柇寮曟搸鍚姩銆?
        logger.warning(
            "Redis unavailable 鈥?degraded mode (session cache in-process, distributed features disabled): %s", e)
        _redis = None
    # 鈹€鈹€ 2.5. PostgreSQL 鈹€鈹€
    if settings.postgres_dsn:
        from app.db import init_pool, ensure_tables
        try:
            await init_pool(settings.postgres_dsn)
            await ensure_tables()
            logger.info("PostgreSQL connected and tables ensured")
        except Exception as e:
            logger.warning("PostgreSQL not available: %s", e)

    # 鈹€鈹€ 3. LLM Gateway 鈹€鈹€
    from app.gateway.provider import LLMProvider
    from app.gateway.router import GatewayRouter
    from app.gateway.cache import SemanticCache
    from app.gateway.budget import TokenBudget
    from app.gateway.ratelimit import TenantRateLimiter

    providers: dict[str, LLMProvider] = {}
    if settings.anthropic_api_key:
        from app.providers.anthropic import AnthropicProvider
        providers["anthropic"] = AnthropicProvider(
            api_key=settings.anthropic_api_key, base_url=settings.anthropic_base_url,
        )
    if settings.openai_api_key or settings.llm_api_key:
        from app.providers.openai import OpenAIProvider
        providers["openai"] = OpenAIProvider(
            api_key=settings.openai_api_key or settings.llm_api_key,
            base_url=settings.openai_base_url or settings.llm_base_url,
        )
    if settings.deepseek_api_key:
        from app.providers.deepseek import DeepSeekProvider
        providers["deepseek"] = DeepSeekProvider(
            api_key=settings.deepseek_api_key, base_url=settings.deepseek_base_url,
        )

    if not providers:
        logger.warning("No LLM providers configured! Set ANTHROPIC_API_KEY / OPENAI_API_KEY / DEEPSEEK_API_KEY or LLM_API_KEY")

    # 鍒涘缓 embedding 鍑芥暟锛堢敤浜庤涔夌紦瀛橈級
    async def _embed_for_cache(text: str) -> list[float]:
        if "openai" in providers:
            resp = await providers["openai"].embed(text, settings.embedding_model)
            return resp.embedding
        return []

    if _redis is not None:
        cache = SemanticCache(
            redis=_redis,
            embed_fn=_embed_for_cache,
            l1_capacity=settings.cache_l1_capacity,
            l2_ttl=settings.cache_l2_ttl,
            semantic_threshold=settings.semantic_cache_threshold,
            semantic_prefix_dims=settings.semantic_cache_prefix_dims,
        )
        budget = TokenBudget(_redis)
    else:
        cache = None
        budget = None

    _gateway = GatewayRouter(
        providers=providers,
        cache=cache,
        budget=budget,
    )
    logger.info("LLM Gateway: %s providers", ", ".join(providers.keys()) or "none")

    # 鈹€鈹€ 3.4. 宸ュ叿/宸ヤ綔娴?gateway 娉ㄥ叆锛堝叚澶т簰閫氾細瀵硅瘽/agent 鍙皟鐢ㄥ伐浣滄祦锛?鈹€鈹€
    from app.tools.graph import bind_gateway as bind_graph_gateway
    from app.tools.pm import bind_gateway as bind_pm_gateway
    from app.workflow.tools import bind_gateway as bind_workflow_gateway
    bind_graph_gateway(_gateway)
    bind_pm_gateway(_gateway)
    bind_workflow_gateway(_gateway)
    # LLM client锛圧AG 妫€绱㈠祵鍏ワ級鍚屾牱鎺ュ叆 gateway
    from app.llm.client import llm_client
    llm_client.bind_gateway(_gateway)
    logger.info("Tool/Workflow gateways bound")

    # 鈹€鈹€ 3.45 璁板繂鏈嶅姟锛圠2 妗ｆ鍗★細璺ㄤ細璇濋暱鏈熻蹇?+ 璇箟妫€绱級鈹€鈹€
    # 渚濊禆 PostgreSQL 杩炴帴姹犱笌宓屽叆閾捐矾锛涗换涓€涓嶅彲鐢ㄥ垯璁板繂鏈嶅姟涓嶅惎鐢紙API 杩斿洖 503 fail-loud锛?
    try:
        from app.db import get_pool
        from app.memory.profile import ProfileStore
        from app.memory.summaries import SummaryStore
        from app.memory.consolidator import Consolidator
        from app.memory.service import MemoryService, bind_service as bind_memory_service
        from app.agent.prompt_engine import bind_memory_service as bind_prompt_memory
        pool = get_pool()
        summary_store = SummaryStore(pool)
        consolidator = Consolidator(
            store=summary_store,
            embedder=llm_client.embed,
        )
        mem_svc = MemoryService(
            store=ProfileStore(pool),
            embedder=llm_client.embed,
            summary_store=summary_store,
            consolidator=consolidator,
        )
        bind_memory_service(mem_svc)
        bind_prompt_memory(mem_svc)
        logger.info("Memory service initialized (L2 profile card + L3 summaries)")
    except Exception as e:
        logger.warning("Memory service not available: %s", e)

    # 鈹€鈹€ 3.6. 鍏ぇ宸ヤ綔鍙拌兘鍔涙敞鍐岋紙浜掗€氬熀纭€锛歍askRouter 渚濊禆鑳藉姏娉ㄥ唽涓績锛?鈹€鈹€
    from app.core.capabilities import preload_default_capabilities
    await preload_default_capabilities()

    # 鈹€鈹€ 3.5. SmartAPIKeyPool 鈹€鈹€
    from app.gateway.smart_key_pool import SmartAPIKeyPool
    _key_pool = SmartAPIKeyPool()
    # 浠?settings 娉ㄥ唽宸叉湁 key
    if settings.openai_api_key:
        await _key_pool.add_key("openai", settings.openai_api_key, "from env")
    if settings.deepseek_api_key:
        await _key_pool.add_key("deepseek", settings.deepseek_api_key, "from env")
    if settings.anthropic_api_key:
        await _key_pool.add_key("anthropic", settings.anthropic_api_key, "from env")
    logger.info("SmartAPIKeyPool initialized with %d providers", len(providers))

    # 鈹€鈹€ 4. 闄愭祦鍣紙middleware 闇€瑕侊級 鈹€鈹€
    if _redis is not None:
        limiter = TenantRateLimiter(
            redis=_redis,
            requests_per_minute=settings.rate_limit_rpm,
            requests_per_second=settings.rate_limit_rps,
        )
    else:
        limiter = None
    app.state.limiter = limiter

    # 鈹€鈹€ 5. MCP Plugin System锛堢敤鎴风骇杩炴帴姹狅細25s 杞娲昏穬鐢ㄦ埛閰嶇疆锛?鈹€鈹€
    global _plugin_pool
    from app.plugins.pool import MCPClientPool
    from app.plugins.store import ActiveTracker, PluginStore
    _plugin_pool = MCPClientPool(store=PluginStore(), tracker=ActiveTracker())
    await _plugin_pool.start()
    logger.info("MCP plugin pool started (poll=%ds)", 25)

    # 鈹€鈹€ 6. 鍚姩 Queue Worker 鈹€鈹€
    if _redis is not None:
        _queue_worker = asyncio.create_task(_run_queue_worker(_redis, _gateway))
        logger.info("Queue worker started (concurrency=%d)", settings.queue_worker_concurrency)
    else:
        logger.info("Queue worker skipped (Redis not available)")

    # 鈹€鈹€ 8. 瀹炰緥娉ㄥ唽 鈹€鈹€
    instance_id = _get_instance_id()
    if _redis is not None:
        await _redis.hset(f"instance:{instance_id}", mapping={
            "started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "pod_name": settings.pod_name or socket.gethostname(),
            "version": "0.1.260825.01",
        })
        await _redis.expire(f"instance:{instance_id}", 60)
        logger.info("Instance registered: %s", instance_id)

    logger.info("=" * 60)
    logger.info("Ready. HTTP port: %d", settings.http_port)
    logger.info("=" * 60)

    yield  # 鈹€鈹€ 搴旂敤杩愯涓?鈹€鈹€

    # 鈹€鈹€ 鍏抽棴 鈹€鈹€
    logger.info("Shutting down...")

    # 娉ㄩ攢瀹炰緥
    if _redis is not None:
        await _redis.delete(f"instance:{instance_id}")

    # 鍋滄闃熷垪 worker
    if _queue_worker:
        _queue_worker.cancel()
        try:
            await _queue_worker
        except asyncio.CancelledError:
            pass

    # 绛夊緟鍚庡彴浠诲姟瀹屾垚锛堜笂涓嬫枃宸╁浐绛夛級
    from app.context.manager import wait_background_tasks
    await wait_background_tasks(timeout=10.0)

    # 鍏抽棴 PostgreSQL
    from app.db import close_pool
    await close_pool()

    # 鍏抽棴 MCP 鎻掍欢姹?
    if _plugin_pool:
        await _plugin_pool.stop()
        _plugin_pool = None

    # 鍏抽棴 Gateway
    if _gateway:
        try:
            await _gateway.close()
        except Exception as e:
            logger.warning("Gateway close error: %s", e)

    # 鍏抽棴 Redis
    if _redis:
        await _redis.close()

    logger.info("Shutdown complete")


def _get_instance_id() -> str:
    if settings.instance_id:
        return settings.instance_id
    if settings.pod_name:
        return settings.pod_name
    return f"{socket.gethostname()}-{uuid.uuid4().hex[:8]}"


# 鈹€鈹€ 闄勪欢鍐呭娉ㄥ叆锛氳嚜鍔ㄤ笅杞芥枃浠跺苟娉ㄥ叆鍒?LLM 涓婁笅鏂囦腑 鈹€鈹€

_MEDIA_URL_RE = re.compile(r'(!?)\[([^\]]+)\]\(([^)]+)\)')


async def _resolve_attachments(content: str) -> str:
    """瑙ｆ瀽鐢ㄦ埛娑堟伅涓殑闄勪欢 Markdown 閾炬帴锛屼笅杞芥枃浠跺唴瀹瑰苟娉ㄥ叆鍒版秷鎭枃鏈腑銆?

    鏀寔锛?
    - Markdown 鍥剧墖 ![](url) 鍜屾櫘閫氶摼鎺?[name](url)
    - 鏂囨湰绫绘枃浠讹紙.txt, .md, .csv, .json, .py 绛夛級锛氳嚜鍔ㄤ笅杞藉苟娉ㄥ叆鍐呭
    - PDF 鏂囦欢锛氭彁鍙栨枃鏈唴瀹?
    - 鍥剧墖鏂囦欢锛氫繚鐣欏師閾炬帴骞舵坊鍔犺鏄?

    澶辫触鏃朵紭闆呴€€鍖栤€斺€斾繚鐣欏師濮嬮摼鎺ワ紝LLM 浠嶅彲閫氳繃 web_fetch 宸ュ叿璁块棶銆?
    """
    if not content:
        return content

    matches = _MEDIA_URL_RE.findall(content)
    if not matches:
        return content

    import httpx
    from app.tools.ssrf import fetch_url_safe

    # S 瀹夊叏淇锛氱鐢ㄨ嚜鍔ㄩ噸瀹氬悜锛屾敼鐢?ssrf.fetch_url_safe 閫愯烦鏍￠獙
    # scheme/绔彛/DNS/IP锛堝惈閲嶅畾鍚戠粫杩囷級鍚庡啀鑾峰彇锛岄槻姝㈤檮浠?URL 鎵撳唴缃?浜戝厓鏁版嵁銆?
    async with httpx.AsyncClient(timeout=15.0, follow_redirects=False) as client:
        for is_image, name, url in matches:
            try:
                resp = await fetch_url_safe(client, url)
                if resp.status_code != 200:
                    continue

                content_type = resp.headers.get("content-type", "") or ""
                file_ext = name.rsplit(".", 1)[-1].lower() if "." in name else ""

                # 鈹€鈹€ 鏂囨湰绫绘枃浠讹細鐩存帴娉ㄥ叆鍐呭 鈹€鈹€
                if (content_type.startswith("text/")
                        or file_ext in ("txt", "md", "csv", "json", "xml", "yaml", "yml",
                                        "py", "js", "ts", "go", "java", "c", "cpp", "h",
                                        "rs", "sh", "bat", "ps1", "sql", "html", "css",
                                        "toml", "ini", "cfg", "conf", "log")):
                    text = resp.text
                    MAX_CHARS = 8000
                    snippet = text[:MAX_CHARS]
                    file_block = (
                        f"\n\n===== 闄勪欢銆寋name}銆嶅唴瀹?({(len(text))} 瀛楃) ====\n"
                        f"{snippet}"
                    )
                    if len(text) > MAX_CHARS:
                        file_block += f"\n... (宸叉埅鏂紝浠呮樉绀哄墠 {MAX_CHARS} 瀛楃)"
                    file_block += "\n===== 闄勪欢缁撴潫 ====="
                    content = content.replace(f"{'!' if is_image else ''}[{name}]({url})", file_block)

                # 鈹€鈹€ PDF锛氬皾璇曟彁鍙栨枃鏈?鈹€鈹€
                elif (content_type == "application/pdf" or file_ext == "pdf"):
                    try:
                        import pymupdf
                        doc = pymupdf.open(stream=resp.content, filetype="pdf")
                        pdf_text = "\n".join(page.get_text() for page in doc)
                        doc.close()
                        MAX_PDF_CHARS = 8000
                        snippet = pdf_text[:MAX_PDF_CHARS]
                        file_block = (
                            f"\n\n===== 闄勪欢銆寋name}銆嶅唴瀹?(PDF, {len(pdf_text)} 瀛楃) ====\n"
                            f"{snippet}"
                        )
                        if len(pdf_text) > MAX_PDF_CHARS:
                            file_block += f"\n... (PDF 杈冮暱锛屽凡鎴柇鍓?{MAX_PDF_CHARS} 瀛楃)"
                        file_block += "\n===== 闄勪欢缁撴潫 ====="
                        content = content.replace(f"{'!' if is_image else ''}[{name}]({url})", file_block)
                    except Exception:
                        # PDF 瑙ｆ瀽澶辫触锛屼繚鐣欏師濮嬮摼鎺?
                        pass

                # 鈹€鈹€ 鍥剧墖锛氫繚鐣?Markdown 鏍煎紡锛屾坊鍔犺鏄?鈹€鈹€
                elif content_type.startswith("image/"):
                    content = content.replace(
                        f"![{name}]({url})",
                        f"![{name}]({url})\n[鍥剧墖闄勪欢锛歿name}]",
                    )

                # 鈹€鈹€ 鍏朵粬浜岃繘鍒舵枃浠讹細灏濊瘯浣滀负鏂囨湰璇诲彇 鈹€鈹€
                else:
                    try:
                        text = resp.text
                        if text and len(text) > 20:
                            MAX_CHARS = 4000
                            snippet = text[:MAX_CHARS]
                            file_block = (
                                f"\n\n===== 闄勪欢銆寋name}銆嶅唴瀹?====\n"
                                f"{snippet}"
                            )
                            if len(text) > MAX_CHARS:
                                file_block += f"\n... (宸叉埅鏂?"
                            file_block += "\n===== 闄勪欢缁撴潫 ====="
                            content = content.replace(f"[{name}]({url})", file_block)
                    except Exception:
                        pass

            except Exception as e:
                logger.warning("瑙ｆ瀽闄勪欢澶辫触: %s 鈥?%s", url, e)
                continue

    return content


def _setup_middleware(app: FastAPI, redis: aioredis.Redis, limiter) -> None:
    """娉ㄥ唽涓棿浠堕摼锛堟敞鎰忥細FastAPI 鍚庢敞鍐岀殑鍏堟墽琛岋級"""
    from app.middleware.error_handler import ErrorHandlerMiddleware
    from app.middleware.metrics import MetricsMiddleware
    from app.middleware.rate_limit import RateLimitMiddleware
    from app.middleware.auth import AuthMiddleware
    from app.middleware.request_context import RequestContextMiddleware

    # 鎵ц椤哄簭: RequestContext 鈫?Auth 鈫?RateLimit 鈫?Metrics 鈫?ErrorHandler 鈫?handler
    app.add_middleware(ErrorHandlerMiddleware)
    app.add_middleware(MetricsMiddleware)
    app.add_middleware(RateLimitMiddleware, limiter=limiter)
    app.add_middleware(
        AuthMiddleware,
        redis_client=redis,
        jwt_secret=settings.jwt_secret,
        internal_token=settings.internal_token,
    )
    app.add_middleware(RequestContextMiddleware)


def _setup_routes(app: FastAPI) -> None:
    """娉ㄥ唽鎵€鏈?HTTP 璺敱"""
    import time as _time

    from app.observability.metrics import QUEUE_DEPTH

    # 鈹€鈹€ 鍋ュ悍妫€鏌?鈹€鈹€

    @app.get("/healthz")
    async def healthz():
        return {"status": "ok"}

    @app.get("/readyz")
    async def readyz():
        """K8s readiness: Redis + 鑷冲皯涓€涓?Provider 鍙敤"""
        if _redis is None:
            return JSONResponse({"status": "not_ready", "reason": "redis not available"}, status_code=503)
        try:
            await _redis.ping()
            return {"status": "ready"}
        except Exception:
            return JSONResponse({"status": "not_ready", "reason": "redis ping failed"}, status_code=503)

    @app.get("/info")
    async def info():
        return {
            "version": "0.1.260825.01",
            "instance_id": _get_instance_id(),
            "uptime_seconds": int(_time.monotonic() - _start_time),
            "gateway": _gateway.stats() if _gateway else None,
        }

    # 鈹€鈹€ Agent 鎺ㄧ悊锛堟ā鍧楃骇璺敱鍑芥暟锛?鈹€鈹€
    app.post("/v1/agent/run")(agent_run)
    app.post("/v1/agent/submit")(agent_submit)
    app.post("/v1/agent/approval")(agent_approval)

    # 鈹€鈹€ 鐭ヨ瘑搴擄紙妯″潡绾ц矾鐢卞嚱鏁帮級 鈹€鈹€
    app.post("/v1/kb/build")(kb_build)
    app.post("/v1/kb/query")(kb_query)

    # 鈹€鈹€ Tools API锛圥hase 1锛?鈹€鈹€
    from app.api import api_router
    app.include_router(api_router)

    # 鈹€鈹€ Admin API Keys锛堟ā鍧楃骇璺敱鍑芥暟锛?鈹€鈹€
    app.get("/v1/admin/api-keys")(admin_list_api_keys)
    app.post("/v1/admin/api-keys")(admin_add_api_key)
    app.put("/v1/admin/api-keys/{key_id}")(admin_update_api_key)
    app.delete("/v1/admin/api-keys/{key_id}")(admin_delete_api_key)


# 鈹€鈹€ 妯″潡绾ц矾鐢卞鐞嗗嚱鏁帮紙FastAPI 闇€鍦ㄦā鍧椾綔鐢ㄥ煙鎵嶈兘姝ｇ‘鎺ㄦ柇 body 绫诲瀷锛?鈹€鈹€


async def agent_run(
    request: Request,
    gateway=Depends(get_gateway),
):
    """娴佸紡 Agent 鎺ㄧ悊 鈥?SSE 杈撳嚭"""
    import json
    from app.agent.loop import run_agent

    body = await request.json()
    llm_config = body.get("llm_config") or {}
    provider_hint = llm_config.get("provider", "")

    async def event_generator():
        try:
            async for event in run_agent(
                gateway=gateway,
                system_prompt=body.get("system_prompt", ""),
                history=body.get("history", []),
                content=body.get("content", ""),
                tools=body.get("tools") or None,
                llm_config=llm_config,
                max_turns=(body.get("max_turns") or 0) if body.get("max_turns", 0) > 0 else None,
                tenant_id=body.get("tenant_id", ""),
                provider_hint=provider_hint,
            ):
                yield f"data: {json.dumps(event, ensure_ascii=False)}\n\n"
        except Exception as e:
            logger.error("Agent run error: %s", e)
            yield f"data: {json.dumps({'type': 'error', 'message': str(e)})}\n\n"

    return StreamingResponse(
        event_generator(),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
    )


async def agent_submit(
    request: Request,
    gateway=Depends(get_gateway),
):
    """Go 缃戝叧浠ｇ悊绔偣 鈥?瀹屾暣 ReAct 寰幆锛孲SE 杈撳嚭"""
    import json
    body = await request.json()
    from app.agent.runtime import AgentRuntime, AgentTask
    import app.tools.core  # noqa: F401 鈥?纭繚鏍稿績宸ュ叿宸叉敞鍐?
    import app.tools.subagent  # noqa: F401 鈥?澶?agent 濮旀淳宸ュ叿
    import app.tools.terminal  # noqa: F401 鈥?鎸佷箙缁堢
    import app.tools.jobs  # noqa: F401 鈥?鍚庡彴浠诲姟
    import app.tools.web  # noqa: F401 鈥?缃戦〉鎼滅储/鎶撳彇
    import app.tools.run_code  # noqa: F401 鈥?PTC 妯″紡
    import app.tools.mode_admin  # noqa: F401 鈥?鍒涢€犳ā寮?
    # 鈹€鈹€ 鍏ぇ宸ヤ綔鍙颁簰鑱斾簰閫氾細娉ㄥ唽鍚勫伐浣滃彴宸ュ叿,浣?CHAT 鐨?LLM 鍙€氳繃 function-calling 璋冪敤 鈹€鈹€
    import app.tools.skill  # noqa: F401 鈥?SKILLS 宸ヤ綔鍙?(skill_list/skill_run/skill_install)
    import app.tools.kb  # noqa: F401 鈥?KNOWLEDGE 宸ヤ綔鍙?(kb_list/kb_search)
    import app.tools.agent  # noqa: F401 鈥?AGENTS 宸ヤ綔鍙?(agent_dispatch 绛?
    import app.workflow.tools  # noqa: F401 鈥?WORKFLOW 宸ヤ綔鍙?(workflow_run/workflow_list)
    import app.tools.memory  # noqa: F401 鈥?闀挎湡璁板繂 (璺ㄥ伐浣滃彴鍏变韩涓婁笅鏂?
    import app.tools.edit_file  # noqa: F401 鈥?鏂囦欢缂栬緫 (鍒涢€犳ā寮忓父鐢?
    import app.tools.browser  # noqa: F401 鈥?娴忚鍣ㄨ嚜鍔ㄥ寲 (PLUGINS/MCP 鎵╁睍)
    # PLUGINS 宸ヤ綔鍙? MCP 宸ュ叿鐢?app.plugins.pool / app.mcp.registry 鍔ㄦ€佹敞鍐?鍚姩鏃跺凡鍔犺浇

    # 鈹€鈹€ 瑙ｆ瀽闄勪欢鏂囦欢鍐呭骞舵敞鍏ュ埌鐢ㄦ埛娑堟伅涓?鈹€鈹€
    raw_content = body.get("content", "")
    resolved_content = await _resolve_attachments(raw_content)
    body["content"] = resolved_content

    # 鈹€鈹€ 韬唤锛氫紭鍏堜俊浠荤綉鍏虫敞鍏ョ殑 X-User-ID锛圫2 瀹夊叏淇锛?鈹€鈹€
    # Python 绔彛浠呭簲缁?Go 缃戝叧鍙揪锛涚洿杩炴椂鑻ョ己澶?header 鎵嶅洖閫€ body锛堜笉淇′换 body 浼€狅級
    gw_user = request.headers.get("x-user-id", "")

    task = AgentTask(
        id=f"submit_{int(time.time())}",
        tenant_id=body.get("tenant_id", ""),
        user_id=gw_user or body.get("user_id", ""),
        session_id=body.get("session_id", ""),
        content=body.get("content", ""),
        history=body.get("history", []),
        max_turns=max(1, min(body.get("max_turns") or settings.max_turns, settings.max_turns)),
    )

    # 鈹€鈹€ 娣卞害鎺ㄧ悊妯″紡锛氳缃?system_prompt 瑕佹眰杈撳嚭鎬濊€冭繃绋?鈹€鈹€
    llm_config = body.get("llm_config", {}) or {}
    if llm_config.get("deep_reasoning"):
        task.system_prompt = (
            "You are Chiron. First output your reasoning process inside "
            "[thinking]...[/thinking] tags, then output your final concise answer.\n"
            "Example: [thinking]I need to analyze...[/thinking]The answer is..."
        )
        # 娣卞害妯″紡闇€瑕佹洿澶х殑杈撳嚭 token 棰勭畻浠ュ绾虫€濊€冭繃绋?
        if "max_tokens" not in llm_config:
            llm_config["max_tokens"] = 8192
        task.llm_config = llm_config
    else:
        task.system_prompt = (
            "You are Chiron. Reply briefly in Chinese. "
            "When the user says 'this code' / '杩欐浠ｇ爜' / '涓婇潰鐨勪唬鐮?, they mean "
            "the code you generated in previous turns of this conversation 鈥?use it "
            "directly, don't ask them to re-paste it. "
            "You can save files with the write_file tool. "
            "When the user says '濯掍綋搴? / 'media library', they mean the "
            "media directory inside your sandbox workspace (create it with "
            "mkdir if needed); you only have access to your own sandbox "
            "workspace 鈥?never use absolute paths or try to access "
            "directories outside it (they are blocked). "
            "Code or text files can be saved there too 鈥?just save the file, "
            "don't refuse because it isn't an image/video/audio."
        )
        task.llm_config = llm_config

    # 鈹€鈹€ 杩愯妯″紡锛堝父瑙?鏋佺畝/PTC/鍒涢€狅級锛氬墠绔笅鎷?鈫?body.mode 鎴?llm_config.mode 鈹€鈹€
    # runtime 鍐?get_mode_config 鍏滃簳鏈煡鍊煎洖閫€ NORMAL
    mode = body.get("mode") or llm_config.get("mode")
    if mode:
        task.llm_config = {**task.llm_config, "mode": mode}

    # 娉ㄥ叆璁板繂鏈嶅姟锛圠2 妗ｆ鍗?+ L3 鎽樿锛夛紝涓嶅彲鐢ㄦ椂 None锛堣涓轰笉鍙橈級
    from app.memory.service import get_service as get_memory_service
    runtime = AgentRuntime(
        gateway=gateway, session_store=_session_cache,
        memory=get_memory_service(),
    )
    session_id = task.session_id
    if session_id:
        _ACTIVE_RUNTIMES[session_id] = (runtime, task.user_id)

    async def event_generator():
        try:
            async for event in runtime.run(task):
                yield f"data: {json.dumps({'type': event.type, 'content': event.content or event.error, 'id': event.tool_call_id, 'name': event.tool_name, 'arguments': event.tool_arguments, 'input_tokens': event.input_tokens, 'output_tokens': event.output_tokens}, ensure_ascii=False)}\n\n"
        except Exception as e:
            logger.error("Agent submit error: %s", e)
            yield f"data: {json.dumps({'type': 'error', 'content': str(e)})}\n\n"
        finally:
            if session_id:
                _ACTIVE_RUNTIMES.pop(session_id, None)

    return StreamingResponse(
        event_generator(),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
    )


async def agent_approval(
    request: Request,
):
    """宸ュ叿纭绔偣锛氳В鍐?agent 寰幆涓瓑寰呯敤鎴风‘璁ょ殑宸ュ叿璋冪敤锛圫 瀹夊叏淇锛夈€?""
    body = await request.json()
    session_id = body.get("session_id", "")
    tool_call_id = body.get("tool_call_id", "")
    approved = bool(body.get("approved", False))
    reason = body.get("reason", "")
    entry = _ACTIVE_RUNTIMES.get(session_id)
    if entry is None:
        return {"ok": False, "error": "no active agent for this session"}

    # S 瀹夊叏淇锛氭牎楠屾潵鐢佃€呮槸鍚︿负浼氳瘽 owner锛岄槻姝粬浜轰唬鎵?鎷掓壒鍗遍櫓宸ュ叿銆?
    # 鍙俊 user_id 鐢?Go 缃戝叧浠庡凡楠岃瘉 JWT claims 鍐欏叆 body(鎴?X-User-ID 澶?锛?
    # 鐩磋繛璺緞鏃犺韬唤鏃朵笉寰楁斁琛屼粬浜恒€?
    runtime, owner_uid = entry
    caller = request.headers.get("x-user-id", "") or body.get("user_id", "")
    if owner_uid and caller and owner_uid != caller:
        logger.warning("approval rejected: caller %s != owner %s (session=%s)",
                       caller, owner_uid, session_id)
        return {"ok": False, "error": "not session owner"}
    resolved = await runtime.submit_approval(tool_call_id, approved, reason)
    return {"ok": resolved}


async def kb_build(
    request: Request,
    gateway=Depends(get_gateway),
):
    """鏂囨。 RAG 绱㈠紩 鈥?SSE 娴佸紡杩涘害"""
    import json, base64
    from app.rag.builder import RAGBuilder

    body = await request.json()
    content_raw = body.get("content", "")
    try:
        content_bytes = base64.b64decode(content_raw)
    except Exception:
        content_bytes = content_raw.encode("utf-8")

    builder = RAGBuilder(llm_gateway=gateway)

    async def event_generator():
        try:
            async for event in builder.build_document(
                kb_id=body.get("kb_id", ""),
                doc_id=body.get("doc_id", ""),
                content=content_bytes,
                file_type=body.get("file_type", ""),
                filename=body.get("filename", ""),
                tenant_id=body.get("tenant_id", ""),
                vector_db=body.get("vector_db", "milvus"),
            ):
                yield f"data: {json.dumps(event, ensure_ascii=False)}\n\n"
        except Exception as e:
            yield f"data: {json.dumps({'type': 'error', 'message': str(e)})}\n\n"

    return StreamingResponse(
        event_generator(),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache"},
    )


async def kb_query(
    request: Request,
    gateway=Depends(get_gateway),
):
    """鏌ヨ鐭ヨ瘑搴?""
    from app.rag.builder import RAGBuilder

    body = await request.json()
    builder = RAGBuilder(llm_gateway=gateway)
    results = await builder.query(
        kb_id=body.get("kb_id", ""),
        query=body.get("query", ""),
        top_k=body.get("top_k", 5),
        threshold=body.get("threshold", 0.5),
        vector_db=body.get("vector_db", "milvus"),
    )
    return {"success": True, "results": results, "count": len(results)}


# 鈹€鈹€ Admin API Key 绠＄悊锛圫martKeyPool 鐨?HTTP 鎺ュ彛锛?鈹€鈹€


def _require_gateway_internal(request: Request) -> None:
    """S 淇:admin API Key 绠＄悊浠呭厑璁哥粡鐢卞彲淇＄綉鍏?X-Internal-Token)鍒拌揪銆?

    Go 缃戝叧宸插湪缃戝叧渚у畬鎴?admin/owner RBAC 鍚庢墠杞彂(涓?ForwardRequest 娉ㄥ叆鏈?token)銆?
    Python 寮曟搸鐩磋繛涓嶅彲缁曡繃瑙掕壊鏍￠獙,闃叉寮曟搸绔彛鍙揪鏃惰浠绘剰璋冪敤鏂瑰鍒犳敼瀵嗛挜姹犮€?
    """
    import hmac

    provided = request.headers.get("X-Internal-Token", "")
    if not settings.internal_token or not provided or not hmac.compare_digest(
        provided, settings.internal_token
    ):
        raise HTTPException(
            status_code=401, detail="admin requires gateway internal token"
        )


async def admin_list_api_keys(
    request: Request,
    pool=Depends(get_key_pool),
    _gateway=Depends(_require_gateway_internal),
):
    """鑾峰彇鎵€鏈?API Key 鍒楄〃"""
    import json
    keys = pool.get_all_keys()
    stats = pool.get_stats()
    return {"keys": keys, "stats": stats}


async def admin_add_api_key(
    request: Request,
    pool=Depends(get_key_pool),
    _gateway=Depends(_require_gateway_internal),
):
    """娣诲姞 API Key"""
    body = await request.json()
    provider = body.get("provider", "")
    key = body.get("key", "")
    remark = body.get("remark", "")
    if not provider or not key:
        from fastapi.responses import JSONResponse
        return JSONResponse({"error": "provider and key are required"}, status_code=400)
    await pool.add_key(provider, key, remark)
    return {"status": "added", "provider": provider}


async def admin_update_api_key(
    request: Request,
    pool=Depends(get_key_pool),
    _gateway=Depends(_require_gateway_internal),
):
    """鏇存柊 API Key 鐘舵€侊紙鎸夌ǔ瀹?ID 瀹氫綅锛宎ctive/rate_limited/circuit_open锛?""
    from fastapi.responses import JSONResponse

    key_id = request.path_params.get("key_id", "")
    body = await request.json()
    status_val = body.get("status", "")
    if not key_id or not status_val:
        return JSONResponse(
            {"status": "error", "error": "key id and status are required", "id": key_id},
            status_code=400,
        )
    updated = await pool.update_key_status(key_id, status_val)
    if not updated:
        return JSONResponse(
            {"status": "not_found", "error": f"API key not found or invalid status: {status_val}", "id": key_id},
            status_code=404,
        )
    return {"status": "updated", "id": key_id, "key_status": status_val}


async def admin_delete_api_key(
    request: Request,
    pool=Depends(get_key_pool),
    _gateway=Depends(_require_gateway_internal),
):
    """鍒犻櫎 API Key锛堟寜璺緞 ID锛涘吋瀹硅姹備綋 provider+key 瀹氫綅锛?""
    key_id = request.path_params.get("key_id", "")
    try:
        try:
            body = await request.json()
        except Exception:
            body = {}
        provider = body.get("provider", "")
        key_full = body.get("key", "")
        if key_id:
            removed = await pool.remove_key_by_id(key_id)
        elif provider and key_full:
            removed = await pool.remove_key(provider, key_full)
        else:
            return JSONResponse(
                {"status": "error", "error": "key id (path) or provider+key (body) required", "id": key_id},
                status_code=400,
            )
        if not removed:
            return JSONResponse(
                {"status": "not_found", "error": "API key not found", "id": key_id},
                status_code=404,
            )
    except Exception as e:
        logger.error("Failed to delete API key %s: %s", key_id, e)
        return {"status": "error", "error": str(e), "id": key_id}
    return {"status": "deleted", "id": key_id}


async def _run_queue_worker(redis: aioredis.Redis, gateway=None) -> None:
    """鍚庡彴闃熷垪娑堣垂鑰?""
    from app.queue.worker import QueueWorker

    worker = QueueWorker(redis=redis, concurrency=settings.queue_worker_concurrency, gateway=gateway)
    try:
        await worker.start()
    except asyncio.CancelledError:
        await worker.stop()


def main():
    """涓诲嚱鏁?""
    uvicorn.run(
        "app.main:create_app",
        factory=True,
        host=settings.http_host,
        port=settings.http_port,
        log_level="warning",  # 鎴戜滑鐢?structlog锛屼笉闇€瑕?uvicorn 鐨勬棩蹇?
        access_log=False,
    )


def create_app() -> FastAPI:
    """鍒涘缓 FastAPI 搴旂敤瀹炰緥锛堜緵 uvicorn factory 妯″紡浣跨敤锛?""
    app = FastAPI(
        title="Chiron Python AI Engine",
        version="0.1.260825.01",
        lifespan=lifespan,
    )
    _setup_middleware_early(app)
    _setup_routes(app)
    return app


def _setup_middleware_early(app: FastAPI) -> None:
    """娉ㄥ唽涓棿浠讹紙鍦?app 鍒涘缓鏃惰皟鐢紝lifespan 涓ˉ鍏?redis 渚濊禆锛?""
    from app.middleware.error_handler import ErrorHandlerMiddleware
    from app.middleware.metrics import MetricsMiddleware
    from app.middleware.request_context import RequestContextMiddleware
    from app.middleware.privacy_middleware import PrivacyModeMiddleware

    # 鎵ц椤哄簭: PrivacyMode 鈫?RequestContext 鈫?Auth 鈫?RateLimit 鈫?Metrics 鈫?ErrorHandler 鈫?handler
    app.add_middleware(ErrorHandlerMiddleware)
    app.add_middleware(MetricsMiddleware)
    app.add_middleware(RequestContextMiddleware)
    app.add_middleware(PrivacyModeMiddleware)


if __name__ == "__main__":
    main()


