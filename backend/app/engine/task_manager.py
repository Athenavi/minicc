"""TaskManager — 后台任务管理器。

管理所有正在运行的 Agent 会话，支持断开后继续执行、会话快照。
"""

from __future__ import annotations

import asyncio
import logging
from typing import Optional

from app.engine.query_engine import QueryEngine

logger = logging.getLogger("minicc.task")


class TaskManager:
    """后台任务管理器。管理所有活跃会话任务。"""

    def __init__(self) -> None:
        self._tasks: dict[str, asyncio.Task] = {}
        self._engines: dict[str, QueryEngine] = {}

    def is_session_active(self, session_id: str) -> bool:
        """检查会话是否仍在运行。"""
        task = self._tasks.get(session_id)
        return task is not None and not task.done()

    def register(self, session_id: str, engine: QueryEngine) -> None:
        """注册一个运行中的 QueryEngine。"""
        self._engines[session_id] = engine

    async def start_session_task(self, session_id: str, coro) -> asyncio.Task:
        """启动后台会话任务。即使 WebSocket 断开，任务继续在后台运行。"""
        task = asyncio.create_task(self._run_with_cleanup(session_id, coro))
        self._tasks[session_id] = task
        return task

    async def _run_with_cleanup(self, session_id: str, coro) -> None:
        try:
            await coro
        except asyncio.CancelledError:
            logger.info("Session task cancelled: %s", session_id)
        except Exception:
            logger.exception("Session task error: %s", session_id)
        finally:
            self._tasks.pop(session_id, None)
            self._engines.pop(session_id, None)
            logger.info("Session task cleaned up: %s", session_id)

    async def cancel_session(self, session_id: str) -> bool:
        """取消运行中的会话。"""
        engine = self._engines.get(session_id)
        if engine:
            engine.cancel()

        task = self._tasks.get(session_id)
        if task and not task.done():
            task.cancel()
            try:
                await asyncio.wait_for(task, timeout=5)
            except (asyncio.TimeoutError, asyncio.CancelledError):
                pass
            return True
        return False

    async def cancel_all(self) -> None:
        """取消所有运行中的会话。"""
        for sid in list(self._tasks.keys()):
            await self.cancel_session(sid)

    def get_active_count(self) -> int:
        """获取活跃会话数。"""
        return len(self._tasks)

    async def cleanup_stale(self, max_idle_minutes: int = 30) -> int:
        """清理超时会话。返回清理数量。"""
        # 简单实现：取消所有超过 max_idle_minutes 的会话
        # Phase 3 完善：检查最后活跃时间
        return 0
