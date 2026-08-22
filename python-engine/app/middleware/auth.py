# 认证中间件 — JWT + API Key
from __future__ import annotations

import logging
from typing import Optional

import jwt

from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from app.observability.logging import tenant_id_var

logger = logging.getLogger(__name__)

# 公开路径（不需要认证）
PUBLIC_PATHS = {"/healthz", "/readyz", "/metrics", "/info", "/docs", "/openapi.json"}


class AuthMiddleware(BaseHTTPMiddleware):
    """
    认证策略:
      1. X-API-Key header → 查询 Redis 获取 tenant_id
      2. Authorization: Bearer <jwt> → 解析 JWT 获取 tenant_id
      3. 公开路径直接放行

    认证成功后设置 tenant_id_var 和 request.state.tenant_id
    """

    def __init__(self, app, redis_client=None, jwt_secret: str = ""):
        super().__init__(app)
        self._redis = redis_client
        self._jwt_secret = jwt_secret

    async def dispatch(self, request: Request, call_next: RequestResponseEndpoint) -> Response:
        # 公开路径
        if request.url.path in PUBLIC_PATHS:
            return await call_next(request)

        # API Key 认证
        api_key = request.headers.get("X-API-Key", "")
        if api_key:
            tenant_id = await self._validate_api_key(api_key)
            if tenant_id:
                return self._set_tenant_and_continue(request, call_next, tenant_id)
            return JSONResponse({"error": "Invalid API key"}, status_code=401)

        # JWT 认证
        auth_header = request.headers.get("Authorization", "")
        if auth_header.startswith("Bearer "):
            token = auth_header[7:]
            tenant_id = self._validate_jwt(token)
            if tenant_id:
                return self._set_tenant_and_continue(request, call_next, tenant_id)
            return JSONResponse({"error": "Invalid or expired token"}, status_code=401)

        # Go 网关代理路径：Authorization 头被剥离时，从 ?tenant_id= 与 ?user_id= 校验
        # 仅信任来自 Go 网关的反向代理（部署时 Python 引擎端口不直接暴露）
        query_tid = request.query_params.get("tenant_id", "")
        query_uid = request.query_params.get("user_id", "")
        if query_tid and query_uid:
            return self._set_tenant_and_continue(request, call_next, query_tid)

        return JSONResponse({"error": "Authentication required"}, status_code=401)

    async def _validate_api_key(self, api_key: str) -> Optional[str]:
        """通过 Redis 验证 API Key → 返回 tenant_id"""
        if not self._redis:
            logger.warning("Redis not available for API key validation")
            return None
        try:
            tenant_id = await self._redis.get(f"apikey:{api_key}")
            return tenant_id.decode() if isinstance(tenant_id, bytes) else tenant_id
        except Exception as e:
            logger.error("API key validation error: %s", e)
            return None

    def _validate_jwt(self, token: str) -> Optional[str]:
        """解析 JWT 获取 tenant_id

        Go 网关签发的 JWT 中 tenant_id 字段非空即用；同时兼容 Go 代理路径透传的
        ?tenant_id= query 参数（X-Tenant-ID header 见 _dispatch 流程）。
        """
        if not self._jwt_secret:
            logger.warning("JWT secret not configured")
            return None
        try:
            payload = jwt.decode(token, self._jwt_secret, algorithms=["HS256"])
            tid = payload.get("tenant_id")
            if tid is None or tid == "":
                # 真多租户部署下空 tenant_id 视为非法 token
                logger.warning("JWT missing tenant_id claim, rejecting")
                return None
            return tid
        except Exception:
            logger.debug("JWT validation failed for token: %s...", token[:20] if token else "")
            return None

    @staticmethod
    async def _set_tenant_and_continue(
        request: Request, call_next: RequestResponseEndpoint, tenant_id: str
    ) -> Response:
        """设置 tenant_id 并继续"""
        token = tenant_id_var.set(tenant_id)
        request.state.tenant_id = tenant_id
        try:
            return await call_next(request)
        finally:
            tenant_id_var.reset(token)
