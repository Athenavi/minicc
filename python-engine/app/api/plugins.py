"""Plugins API — MCP 插件池状态与重载（内部端点，共享密钥校验）。

- GET  /v1/plugins/status：当前连接池状态（活跃用户/共享连接/已加载用户）
- POST /v1/plugins/reload：立即按当前配置 reconcile（无需等待 25s 轮询）

鉴权：请求头 X-API-Key 必须等于 LLM_GATEWAY_KEY（与 Go 网关共享）。
未配置 LLM_GATEWAY_KEY 时端点不可用（返回 503），强制生产配置。
"""
from __future__ import annotations

from typing import Any

from fastapi import APIRouter, HTTPException, Request

from app.config import settings

router = APIRouter(tags=["plugins"])


def _verify_gateway_key(request: Request) -> None:
    if not settings.llm_gateway_key:
        raise HTTPException(status_code=503, detail="LLM_GATEWAY_KEY not configured")
    provided = request.headers.get("X-API-Key", "")
    if provided != settings.llm_gateway_key:
        raise HTTPException(status_code=401, detail="invalid gateway key")


@router.get("/v1/plugins/status")
async def plugin_status(request: Request) -> dict[str, Any]:
    _verify_gateway_key(request)
    from app.main import get_plugin_pool

    pool = get_plugin_pool()
    return {"ok": True, **pool.status()}


@router.post("/v1/plugins/reload")
async def plugin_reload(request: Request) -> dict[str, Any]:
    _verify_gateway_key(request)
    from app.main import get_plugin_pool

    pool = get_plugin_pool()
    await pool.reconcile()
    return {"ok": True, **pool.status()}
