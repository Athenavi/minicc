"""权限与审批状态机 PermissionHandler。

MiniCC 安全体系的核心——"先审批后执行"的看门狗。
LLM 发起工具调用 → 主循环挂起 → WebSocket 审批请求 → 用户响应 → 执行或中止。
"""

from __future__ import annotations

import asyncio
import enum
import logging
import uuid
from typing import Callable, Optional

from app.models.permission import PermissionLevel, PermissionRequest
from app.models.tool import ToolCall, ToolResult

logger = logging.getLogger("minicc.permission")

APPROVAL_TIMEOUT = 300  # 5 分钟


class PermissionResult(enum.Enum):
    APPROVED = "approved"
    REJECTED = "rejected"
    TIMEOUT = "timeout"


class PendingRequest:
    """一个等待用户审批的请求。"""

    def __init__(self, request: PermissionRequest) -> None:
        self.request = request
        self._event = asyncio.Event()
        self.result: PermissionResult | None = None

    async def wait(self, timeout: float = APPROVAL_TIMEOUT) -> PermissionResult:
        """等待用户响应。超时视为拒绝。"""
        try:
            await asyncio.wait_for(self._event.wait(), timeout=timeout)
        except asyncio.TimeoutError:
            self.result = PermissionResult.TIMEOUT
            return PermissionResult.TIMEOUT
        return self.result  # type: ignore[return-value]

    def resolve(self, result: PermissionResult) -> None:
        """用户已响应，解除等待。"""
        self.result = result
        self._event.set()


class PermissionHandler:
    """权限与审批状态机。

    管理所有待审批请求、处理用户响应、维护"始终允许"和拒绝记忆。
    """

    def __init__(self, send_callback: Optional[Callable] = None, approval_timeout: float = APPROVAL_TIMEOUT) -> None:
        self._send = send_callback
        self._approval_timeout = approval_timeout
        self._mode: str = "ask"  # "ask" | "auto" | "yolo"
        self._pending: dict[str, PendingRequest] = {}
        self._always_allow: dict[str, PermissionLevel] = {}
        self._denied: set[str] = set()

    def set_mode(self, mode: str) -> None:
        """设置执行模式：ask（询问）/ auto（自动）/ yolo（全部自动）。"""
        if mode not in ("ask", "auto", "yolo"):
            raise ValueError(f"Invalid mode: {mode}")
        self._mode = mode
        logger.info("Permission mode set to: %s", mode)

    async def request_permission(
        self,
        tool_call: ToolCall,
        level: PermissionLevel,
        reason: str = "",
        diff_preview: str | None = None,
    ) -> PermissionResult:
        """请求权限。

        - READ 级别：自动放行
        - ask 模式：WRITE/EXECUTE 需审批
        - auto 模式：READ/WRITE 自动放行，EXECUTE 需审批
        - yolo 模式：全部自动放行
        """
        if level == PermissionLevel.READ:
            return PermissionResult.APPROVED

        if self._is_always_allowed(tool_call.name, level):
            return PermissionResult.APPROVED

        if tool_call.name in self._denied:
            return PermissionResult.REJECTED

        if self._mode == "yolo":
            return PermissionResult.APPROVED

        if self._mode == "auto" and level == PermissionLevel.WRITE:
            return PermissionResult.APPROVED

        # 创建审批请求
        perm_req = PermissionRequest(
            id=uuid.uuid4().hex[:12],
            tool_name=tool_call.name,
            tool_input=tool_call.input,
            level=level,
            reason=reason,
            diff_preview=diff_preview,
        )

        pending = PendingRequest(perm_req)
        self._pending[perm_req.id] = pending

        # 通过回调发送审批请求到前端
        if self._send:
            await self._send({
                "type": "permission_required",
                "payload": perm_req.model_dump(),
            })

        logger.info("Awaiting approval: %s (%s)", tool_call.name, level.value)

        # 等待用户响应
        result = await pending.wait(timeout=self._approval_timeout)

        # 清理
        self._pending.pop(perm_req.id, None)

        if result == PermissionResult.REJECTED:
            self._denied.add(tool_call.name)

        return result

    def handle_user_response(self, request_id: str, action: str) -> None:
        """处理用户的前端响应。"""
        pending = self._pending.get(request_id)
        if not pending:
            logger.warning("Unknown permission request: %s", request_id)
            return

        if action == "approve":
            pending.resolve(PermissionResult.APPROVED)
            logger.info("Approved: %s", pending.request.tool_name)
        elif action == "reject":
            pending.resolve(PermissionResult.REJECTED)
            logger.info("Rejected: %s", pending.request.tool_name)
        elif action == "always_allow":
            pending.resolve(PermissionResult.APPROVED)
            self._always_allow[pending.request.tool_name] = pending.request.level
            logger.info("Always allow: %s (%s)", pending.request.tool_name, pending.request.level.value)
        else:
            logger.warning("Unknown approval action: %s", action)

    def cancel_all_pending(self) -> None:
        """取消所有待审批请求（用户中断时）。"""
        for pending in self._pending.values():
            pending.resolve(PermissionResult.REJECTED)
        self._pending.clear()
        logger.info("Cancelled %d pending permission requests", len(self._pending))

    def _is_always_allowed(self, tool_name: str, level: PermissionLevel) -> bool:
        """检查工具是否已被用户标记为'始终允许'。

        如果用户允许了 EXECUTE 级别，那么同工具的 READ/WRITE 也自动允许。
        """
        allowed_level = self._always_allow.get(tool_name)
        if allowed_level is None:
            return False
        # EXECUTE >= WRITE >= READ，所以如果允许了更高等级则低等级也允许
        return allowed_level >= level

    def format_rejection_feedback(self, tool_name: str) -> str:
        """生成给 LLM 的拒绝反馈。"""
        if tool_name in self._denied:
            return f"The {tool_name} operation was just rejected by the user. Suggest an alternative approach that doesn't require this operation."
        return f"The {tool_name} operation was rejected by the user."
