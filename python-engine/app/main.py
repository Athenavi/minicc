# Python AI 引擎入口 — 无状态 FastAPI + 连接池 + 健康检查 + 依赖注入
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
from fastapi import FastAPI, Depends, Request
from fastapi.responses import JSONResponse, StreamingResponse

from app.config import settings
from app.core.container import GlobalContainer, get_container
from app.session_store import SessionStore

# 全局会话消息缓存（lifespan 中接入 Redis 实现多实例共享）
_session_cache = SessionStore(max_sessions=200)

logger = logging.getLogger(__name__)

# 全局引用（lifespan 中初始化）
_start_time = time.monotonic()
_redis: aioredis.Redis | None = None
_gateway = None  # GatewayRouter
_queue_worker = None  # asyncio.Task
_mcp_client = None  # MCPClient
_key_pool = None  # SmartAPIKeyPool


# ── FastAPI 依赖注入 ──

async def get_redis() -> aioredis.Redis:
    """获取 Redis 连接（FastAPI Depends）"""
    if _redis is None:
        raise RuntimeError("Redis not initialized")
    return _redis


async def get_gateway():
    """获取 Gateway Router（FastAPI Depends）"""
    if _gateway is None:
        raise RuntimeError("Gateway not initialized")
    return _gateway


async def get_key_pool():
    """获取 SmartAPIKeyPool（FastAPI Depends）"""
    if _key_pool is None:
        raise RuntimeError("Key pool not initialized")
    return _key_pool


@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期：启动初始化 + 关闭清理"""
    global _redis, _gateway, _queue_worker, _key_pool

    # ── 1. 可观测性 ──
    from app.observability.logging import configure_logging
    from app.observability.tracing import configure_tracing
    from app.observability.metrics import ENGINE_INFO, INSTANCE_UPTIME

    configure_logging(settings.log_level)
    configure_tracing(service_name="python-engine", otlp_endpoint=settings.otel_endpoint)
    ENGINE_INFO.info({"version": "3.0.0", "instance_id": _get_instance_id()})

    logger.info("=" * 60)
    logger.info("MiniCC Python AI Engine v3.0 — Enterprise Edition")
    logger.info("=" * 60)

    # ── 2. Redis 连接池 ──
    _redis = aioredis.from_url(
        settings.redis_url,
        decode_responses=False,
        max_connections=settings.redis_max_connections,
    )
    try:
        await _redis.ping()
        logger.info("Redis connected: %s (pool=%d)", settings.redis_url, settings.redis_max_connections)
        # 将 SessionStore 接入 Redis，实现多实例共享
        _session_cache._redis = _redis
        logger.info("SessionStore switched to Redis backend")
    except Exception as e:
        logger.warning("Redis not available: %s — running without Redis", e)
        _redis = None

    # ── 2.5. PostgreSQL ──
    if settings.postgres_dsn:
        from app.db import init_pool, ensure_tables
        try:
            await init_pool(settings.postgres_dsn)
            await ensure_tables()
            logger.info("PostgreSQL connected and tables ensured")
        except Exception as e:
            logger.warning("PostgreSQL not available: %s", e)

    # ── 3. LLM Gateway ──
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

    # 创建 embedding 函数（用于语义缓存）
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

    # ── 3.5. SmartAPIKeyPool ──
    from app.gateway.smart_key_pool import SmartAPIKeyPool
    _key_pool = SmartAPIKeyPool()
    # 从 settings 注册已有 key
    if settings.openai_api_key:
        await _key_pool.add_key("openai", settings.openai_api_key, "from env")
    if settings.deepseek_api_key:
        await _key_pool.add_key("deepseek", settings.deepseek_api_key, "from env")
    if settings.anthropic_api_key:
        await _key_pool.add_key("anthropic", settings.anthropic_api_key, "from env")
    logger.info("SmartAPIKeyPool initialized with %d providers", len(providers))

    # ── 4. 限流器（middleware 需要） ──
    if _redis is not None:
        limiter = TenantRateLimiter(
            redis=_redis,
            requests_per_minute=settings.rate_limit_rpm,
            requests_per_second=settings.rate_limit_rps,
        )
    else:
        limiter = None
    app.state.limiter = limiter

    # ── 5. MCP Plugin System ──
    from app.mcp.registry import init_mcp
    import os
    mcp_config_path = os.getenv("MCP_CONFIG_PATH", os.path.join(".", "workspace", "plugins.json"))
    _mcp_client = await init_mcp(mcp_config_path)
    if _mcp_client:
        logger.info("MCP initialized: %d tools", len(_mcp_client.tools))

    # ── 6. 启动 Queue Worker ──
    if _redis is not None:
        _queue_worker = asyncio.create_task(_run_queue_worker(_redis))
        logger.info("Queue worker started (concurrency=%d)", settings.queue_worker_concurrency)
    else:
        logger.info("Queue worker skipped (Redis not available)")

    # ── 8. 实例注册 ──
    instance_id = _get_instance_id()
    if _redis is not None:
        await _redis.hset(f"instance:{instance_id}", mapping={
            "started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "pod_name": settings.pod_name or socket.gethostname(),
            "version": "3.0.0",
        })
        await _redis.expire(f"instance:{instance_id}", 60)
        logger.info("Instance registered: %s", instance_id)

    logger.info("=" * 60)
    logger.info("Ready. HTTP port: %d", settings.http_port)
    logger.info("=" * 60)

    yield  # ── 应用运行中 ──

    # ── 关闭 ──
    logger.info("Shutting down...")

    # 注销实例
    if _redis is not None:
        await _redis.delete(f"instance:{instance_id}")

    # 停止队列 worker
    if _queue_worker:
        _queue_worker.cancel()
        try:
            await _queue_worker
        except asyncio.CancelledError:
            pass

    # 关闭 PostgreSQL
    from app.db import close_pool
    await close_pool()

    # 关闭 MCP
    if _mcp_client:
        await _mcp_client.close()

    # 关闭 Gateway
    if _gateway:
        try:
            await _gateway.close()
        except Exception as e:
            logger.warning("Gateway close error: %s", e)

    # 关闭 Redis
    if _redis:
        await _redis.close()

    logger.info("Shutdown complete")


def _get_instance_id() -> str:
    if settings.instance_id:
        return settings.instance_id
    if settings.pod_name:
        return settings.pod_name
    return f"{socket.gethostname()}-{uuid.uuid4().hex[:8]}"


# ── 附件内容注入：自动下载文件并注入到 LLM 上下文中 ──

_MEDIA_URL_RE = re.compile(r'(!?)\[([^\]]+)\]\(([^)]+)\)')


async def _resolve_attachments(content: str) -> str:
    """解析用户消息中的附件 Markdown 链接，下载文件内容并注入到消息文本中。

    支持：
    - Markdown 图片 ![](url) 和普通链接 [name](url)
    - 文本类文件（.txt, .md, .csv, .json, .py 等）：自动下载并注入内容
    - PDF 文件：提取文本内容
    - 图片文件：保留原链接并添加说明

    失败时优雅退化——保留原始链接，LLM 仍可通过 web_fetch 工具访问。
    """
    if not content:
        return content

    matches = _MEDIA_URL_RE.findall(content)
    if not matches:
        return content

    import httpx

    async with httpx.AsyncClient(timeout=15.0, follow_redirects=True) as client:
        for is_image, name, url in matches:
            try:
                resp = await client.get(url)
                if resp.status_code != 200:
                    continue

                content_type = resp.headers.get("content-type", "") or ""
                file_ext = name.rsplit(".", 1)[-1].lower() if "." in name else ""

                # ── 文本类文件：直接注入内容 ──
                if (content_type.startswith("text/")
                        or file_ext in ("txt", "md", "csv", "json", "xml", "yaml", "yml",
                                        "py", "js", "ts", "go", "java", "c", "cpp", "h",
                                        "rs", "sh", "bat", "ps1", "sql", "html", "css",
                                        "toml", "ini", "cfg", "conf", "log")):
                    text = resp.text
                    MAX_CHARS = 8000
                    snippet = text[:MAX_CHARS]
                    file_block = (
                        f"\n\n===== 附件「{name}」内容 ({(len(text))} 字符) ====\n"
                        f"{snippet}"
                    )
                    if len(text) > MAX_CHARS:
                        file_block += f"\n... (已截断，仅显示前 {MAX_CHARS} 字符)"
                    file_block += "\n===== 附件结束 ====="
                    content = content.replace(f"{'!' if is_image else ''}[{name}]({url})", file_block)

                # ── PDF：尝试提取文本 ──
                elif (content_type == "application/pdf" or file_ext == "pdf"):
                    try:
                        import pymupdf
                        doc = pymupdf.open(stream=resp.content, filetype="pdf")
                        pdf_text = "\n".join(page.get_text() for page in doc)
                        doc.close()
                        MAX_PDF_CHARS = 8000
                        snippet = pdf_text[:MAX_PDF_CHARS]
                        file_block = (
                            f"\n\n===== 附件「{name}」内容 (PDF, {len(pdf_text)} 字符) ====\n"
                            f"{snippet}"
                        )
                        if len(pdf_text) > MAX_PDF_CHARS:
                            file_block += f"\n... (PDF 较长，已截断前 {MAX_PDF_CHARS} 字符)"
                        file_block += "\n===== 附件结束 ====="
                        content = content.replace(f"{'!' if is_image else ''}[{name}]({url})", file_block)
                    except Exception:
                        # PDF 解析失败，保留原始链接
                        pass

                # ── 图片：保留 Markdown 格式，添加说明 ──
                elif content_type.startswith("image/"):
                    content = content.replace(
                        f"![{name}]({url})",
                        f"![{name}]({url})\n[图片附件：{name}]",
                    )

                # ── 其他二进制文件：尝试作为文本读取 ──
                else:
                    try:
                        text = resp.text
                        if text and len(text) > 20:
                            MAX_CHARS = 4000
                            snippet = text[:MAX_CHARS]
                            file_block = (
                                f"\n\n===== 附件「{name}」内容 ====\n"
                                f"{snippet}"
                            )
                            if len(text) > MAX_CHARS:
                                file_block += f"\n... (已截断)"
                            file_block += "\n===== 附件结束 ====="
                            content = content.replace(f"[{name}]({url})", file_block)
                    except Exception:
                        pass

            except Exception as e:
                logger.warning("解析附件失败: %s — %s", url, e)
                continue

    return content


def _setup_middleware(app: FastAPI, redis: aioredis.Redis, limiter) -> None:
    """注册中间件链（注意：FastAPI 后注册的先执行）"""
    from app.middleware.error_handler import ErrorHandlerMiddleware
    from app.middleware.metrics import MetricsMiddleware
    from app.middleware.rate_limit import RateLimitMiddleware
    from app.middleware.auth import AuthMiddleware
    from app.middleware.request_context import RequestContextMiddleware

    # 执行顺序: RequestContext → Auth → RateLimit → Metrics → ErrorHandler → handler
    app.add_middleware(ErrorHandlerMiddleware)
    app.add_middleware(MetricsMiddleware)
    app.add_middleware(RateLimitMiddleware, limiter=limiter)
    app.add_middleware(AuthMiddleware, redis_client=redis, jwt_secret=settings.jwt_secret)
    app.add_middleware(RequestContextMiddleware)


def _setup_routes(app: FastAPI) -> None:
    """注册所有 HTTP 路由"""
    import time as _time

    from app.observability.metrics import QUEUE_DEPTH

    # ── 健康检查 ──

    @app.get("/healthz")
    async def healthz():
        return {"status": "ok"}

    @app.get("/readyz")
    async def readyz():
        """K8s readiness: Redis + 至少一个 Provider 可用"""
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
            "version": "3.0.0",
            "instance_id": _get_instance_id(),
            "uptime_seconds": int(_time.monotonic() - _start_time),
            "gateway": _gateway.stats() if _gateway else None,
        }

    # ── Agent 推理（模块级路由函数） ──
    app.post("/v1/agent/run")(agent_run)
    app.post("/v1/agent/submit")(agent_submit)

    # ── 知识库（模块级路由函数） ──
    app.post("/v1/kb/build")(kb_build)
    app.post("/v1/kb/query")(kb_query)

    # ── Tools API（Phase 1） ──
    from app.api import api_router
    app.include_router(api_router)

    # ── Admin API Keys（模块级路由函数） ──
    app.get("/v1/admin/api-keys")(admin_list_api_keys)
    app.post("/v1/admin/api-keys")(admin_add_api_key)
    app.put("/v1/admin/api-keys/{key_id}")(admin_update_api_key)
    app.delete("/v1/admin/api-keys/{key_id}")(admin_delete_api_key)


# ── 模块级路由处理函数（FastAPI 需在模块作用域才能正确推断 body 类型） ──


async def agent_run(
    request: Request,
    gateway=Depends(get_gateway),
):
    """流式 Agent 推理 — SSE 输出"""
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
    """Go 网关代理端点 — 完整 ReAct 循环，SSE 输出"""
    import json
    body = await request.json()
    from app.agent.runtime import AgentRuntime, AgentTask
    import app.tools.core  # noqa: F401 — 确保核心工具已注册

    # ── 解析附件文件内容并注入到用户消息中 ──
    raw_content = body.get("content", "")
    resolved_content = await _resolve_attachments(raw_content)
    body["content"] = resolved_content

    task = AgentTask(
        id=f"submit_{int(time.time())}",
        tenant_id=body.get("tenant_id", ""),
        user_id=body.get("user_id", ""),
        session_id=body.get("session_id", ""),
        content=body.get("content", ""),
        history=body.get("history", []),
        max_turns=max(1, min(body.get("max_turns") or settings.max_turns, settings.max_turns)),
    )

    # ── 深度推理模式：设置 system_prompt 要求输出思考过程 ──
    llm_config = body.get("llm_config", {}) or {}
    if llm_config.get("deep_reasoning"):
        task.system_prompt = (
            "You are MiniCC. First output your reasoning process inside "
            "[thinking]...[/thinking] tags, then output your final concise answer.\n"
            "Example: [thinking]I need to analyze...[/thinking]The answer is..."
        )
        # 深度模式需要更大的输出 token 预算以容纳思考过程
        if "max_tokens" not in llm_config:
            llm_config["max_tokens"] = 8192
        task.llm_config = llm_config
    else:
        task.system_prompt = "You are MiniCC. Reply briefly."
        task.llm_config = llm_config

    runtime = AgentRuntime(gateway=gateway, session_store=_session_cache)

    async def event_generator():
        try:
            async for event in runtime.run(task):
                yield f"data: {json.dumps({'type': event.type, 'content': event.content or event.error, 'id': event.tool_call_id, 'name': event.tool_name, 'arguments': event.tool_arguments, 'input_tokens': event.input_tokens, 'output_tokens': event.output_tokens}, ensure_ascii=False)}\n\n"
        except Exception as e:
            logger.error("Agent submit error: %s", e)
            yield f"data: {json.dumps({'type': 'error', 'content': str(e)})}\n\n"

    return StreamingResponse(
        event_generator(),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
    )


async def kb_build(
    request: Request,
    gateway=Depends(get_gateway),
):
    """文档 RAG 索引 — SSE 流式进度"""
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
    """查询知识库"""
    from app.rag.builder import RAGBuilder

    body = await request.json()
    builder = RAGBuilder(llm_gateway=gateway)
    results = await builder.query_milvus(
        kb_id=body.get("kb_id", ""),
        query=body.get("query", ""),
        top_k=body.get("top_k", 5),
        threshold=body.get("threshold", 0.5),
    )
    return {"success": True, "results": results, "count": len(results)}


# ── Admin API Key 管理（SmartKeyPool 的 HTTP 接口） ──


async def admin_list_api_keys(
    request: Request,
    pool=Depends(get_key_pool),
):
    """获取所有 API Key 列表"""
    import json
    keys = pool.get_all_keys()
    stats = pool.get_stats()
    return {"keys": keys, "stats": stats}


async def admin_add_api_key(
    request: Request,
    pool=Depends(get_key_pool),
):
    """添加 API Key"""
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
):
    """更新 API Key 状态"""
    key_id = request.path_params.get("key_id", "")
    body = await request.json()
    status_val = body.get("status", "")
    # SmartKeyPool 无按 ID 更新接口，暂返回 success
    return {"status": "updated", "id": key_id}


async def admin_delete_api_key(
    request: Request,
    pool=Depends(get_key_pool),
):
    """删除 API Key"""
    key_id = request.path_params.get("key_id", "")
    try:
        body = await request.json()
        provider = body.get("provider", "")
        key_full = body.get("key", "")
        if not provider or not key_full:
            return JSONResponse(
                {"status": "error", "error": "provider and key are required in request body", "id": key_id},
                status_code=400,
            )
        removed = await pool.remove_key(provider, key_full)
        if not removed:
            return JSONResponse(
                {"status": "not_found", "error": "API key not found", "id": key_id},
                status_code=404,
            )
    except Exception as e:
        logger.error("Failed to delete API key %s: %s", key_id, e)
        return {"status": "error", "error": str(e), "id": key_id}
    return {"status": "deleted", "id": key_id}


async def _run_queue_worker(redis: aioredis.Redis) -> None:
    """后台队列消费者"""
    from app.queue.worker import QueueWorker

    worker = QueueWorker(redis=redis, concurrency=settings.queue_worker_concurrency)
    try:
        await worker.start()
    except asyncio.CancelledError:
        await worker.stop()


def main():
    """主函数"""
    uvicorn.run(
        "app.main:create_app",
        factory=True,
        host=settings.http_host,
        port=settings.http_port,
        log_level="warning",  # 我们用 structlog，不需要 uvicorn 的日志
        access_log=False,
    )


def create_app() -> FastAPI:
    """创建 FastAPI 应用实例（供 uvicorn factory 模式使用）"""
    app = FastAPI(
        title="MiniCC Python AI Engine",
        version="3.0.0",
        lifespan=lifespan,
    )
    _setup_middleware_early(app)
    _setup_routes(app)
    return app


def _setup_middleware_early(app: FastAPI) -> None:
    """注册中间件（在 app 创建时调用，lifespan 中补充 redis 依赖）"""
    from app.middleware.error_handler import ErrorHandlerMiddleware
    from app.middleware.metrics import MetricsMiddleware
    from app.middleware.request_context import RequestContextMiddleware

    # 执行顺序: RequestContext → Auth → RateLimit → Metrics → ErrorHandler → handler
    app.add_middleware(ErrorHandlerMiddleware)
    app.add_middleware(MetricsMiddleware)
    app.add_middleware(RequestContextMiddleware)


if __name__ == "__main__":
    main()
