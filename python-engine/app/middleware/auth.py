# 认证中间件 — JWT + 网关内部 token（API Key 校验由网关负责，引擎不处理）
from __future__ import annotations

import hmac
import logging
from typing import Optional

import jwt
from jwt import InvalidTokenError

from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from app.observability.logging import tenant_id_var

logger = logging.getLogger(__name__)

# 公开路径（不需要认证）
PUBLIC_PATHS = {"/healthz", "/readyz", "/metrics", "/info", "/docs", "/openapi.json"}


class AuthMiddleware(BaseHTTPMiddleware):
    """
    认证策略（优先级从高到低）:
      1. 公开路径直接放行
      2. Authorization: Bearer <jwt> → 解析 JWT 获取 tenant_id
      3. 网关代理路径：仅在 X-Internal-Token 与共享密钥匹配时，
         才接受 ?tenant_id= / ?user_id= 透传身份

    注意：本中间件按架构设计未挂载（引擎无鉴权，网关为认证边界），
    API Key 校验（apikey:*）由网关负责，引擎不查 Redis、不校验 X-API-Key。

    安全约束：
      - query 参数透传身份必须校验 X-Internal-Token（P0-3 防伪造）
      - JWT 必须包含非空 tenant_id claim（多租户隔离）
      - 所有比较使用 compare_digest 防计时攻击
    """

    def __init__(self, app, redis_client=None, jwt_secret: str = "", internal_token: str = ""):
        super().__init__(app)
        self._redis = redis_client
        self._jwt_secret = jwt_secret
        self._internal_token = internal_token

    async def dispatch(self, request: Request, call_next: RequestResponseEndpoint) -> Response:
        # 公开路径
        if request.url.path in PUBLIC_PATHS:
            return await call_next(request)

        # JWT 认证（API Key 校验由网关负责，见类 docstring）
        auth_header = request.headers.get("Authorization", "")
        if auth_header.startswith("Bearer "):
            token = auth_header[7:]
            tenant_id = await self._validate_jwt(token)
            if tenant_id:
                return self._set_tenant_and_continue(request, call_next, tenant_id)
            return JSONResponse({"error": "Invalid or expired token"}, status_code=401)

        # 网关代理路径：仅在 X-Internal-Token 校验通过时才信任 query 透传身份
        # 历史 P0-3：原代码无来源验证，任何直连客户端可伪造 ?tenant_id= 任意身份
        query_tid = request.query_params.get("tenant_id", "")
        query_uid = request.query_params.get("user_id", "")
        if query_tid and query_uid:
            if not self._is_internal_request(request):
                logger.warning(
                    "Rejected gateway-impersonation attempt: query tenant_id=%s without X-Internal-Token",
                    query_tid,
                )
                return JSONResponse(
                    {"error": "Gateway identity requires valid X-Internal-Token"},
                    status_code=401,
                )
            return self._set_tenant_and_continue(request, call_next, query_tid)

        return JSONResponse({"error": "Authentication required"}, status_code=401)

    def _is_internal_request(self, request: Request) -> bool:
        """校验 X-Internal-Token 是否与配置的 internal_token 常量时间匹配。

        未配置 internal_token 时拒绝所有 query 透传身份（fail-close）。
        """
        if not self._internal_token:
            return False
        provided = request.headers.get("X-Internal-Token", "")
        if not provided:
            return False
        # 常量时间比较，防计时侧信道
        return hmac.compare_digest(provided, self._internal_token)

    async def _validate_jwt(self, token: str) -> Optional[str]:
        """解析 JWT 获取 tenant_id，并校验黑名单。

        Go 网关签发的 JWT 中 tenant_id 字段非空即用；同时兼容 Go 代理路径透传的
        ?tenant_id= query 参数（X-Tenant-ID header 见 _dispatch 流程）。

        P2-1: 与 Go 端保持一致，登出后的 token 立即失效（查 Redis jwt:blacklist:<jti>）。
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
            # P2-1: 校验 JWT 黑名单（jti claim），与 Go 端 jwt:blacklist:<jti> 一致
            jti = payload.get("jti")
            if jti and self._redis:
                try:
                    blacklisted = await self._redis.exists(f"jwt:blacklist:{jti}")
                    if blacklisted:
                        logger.info("JWT rejected: blacklisted jti=%s", jti)
                        return None
                except Exception as e:
                    logger.warning("JWT blacklist check failed: %s", e)
            return tid
        except InvalidTokenError:
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
