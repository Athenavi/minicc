"""jobs — 后台任务（对应 deepseek-harness dsh-tool-jobs）

run_in_background 将命令放入后台执行（asyncio task），立即返回 job_id；
job_output 轮询结果，job_kill 取消。命令在持久 shell 中执行（跨调用状态
共享），与 foreground 的 persistent_shell 同一后端。

P2 修复: 添加完成任务的自动清理，防止长期运行中 _jobs 字典无限增长。
"""
from __future__ import annotations

import asyncio
import logging
import time
import uuid
from typing import Any, Optional

from app.tools.context import get_session_id
from app.tools.registry import registry

logger = logging.getLogger(__name__)

_jobs: dict[str, asyncio.Task] = {}
_job_shell_key: dict[str, str] = {}  # job_id → shell key（session 隔离）
_job_created_at: dict[str, float] = {}  # job_id → creation timestamp

# Cleanup: tasks completing more than this many hours ago are auto-removed
_JOB_TTL_HOURS = 24
_cleanup_started = False


async def _start_cleanup_loop() -> None:
    """Periodically remove stale completed tasks to prevent memory leaks."""
    global _cleanup_started
    if _cleanup_started:
        return
    _cleanup_started = True
    while True:
        await asyncio.sleep(3600)  # every hour
        now = time.monotonic()
        stale_ids = []
        for job_id, task in list(_jobs.items()):
            created = _job_created_at.get(job_id, now)
            age_hours = (now - created) / 3600
            if task.done() and age_hours > _JOB_TTL_HOURS:
                # Retrieve result to suppress "task was destroyed" warning
                try:
                    task.result()
                except Exception:
                    pass
                stale_ids.append(job_id)
            elif not task.done() and age_hours > 48:
                # Kill tasks running > 48 hours (likely stuck)
                task.cancel()
                stale_ids.append(job_id)
        for job_id in stale_ids:
            _jobs.pop(job_id, None)
            _job_shell_key.pop(job_id, None)
            _job_created_at.pop(job_id, None)
        if stale_ids:
            logger.info("cleaned up %d stale background jobs", len(stale_ids))


def _ensure_cleanup_started() -> None:
    """Start the cleanup loop if not already running."""
    asyncio.create_task(_start_cleanup_loop())


async def run_in_background(command: str) -> dict[str, Any]:
    """Run *command* in the background; returns a job id."""
    if not command.strip():
        return {"error": "command is required"}
    job_id = f"job_{uuid.uuid4().hex[:10]}"
    shell_key = get_session_id() or "default"

    from app.tools.terminal import _terminal

    async def _run() -> dict[str, Any]:
        return await _terminal.execute(shell_key, command, timeout=3600)

    _jobs[job_id] = asyncio.create_task(_run())
    _job_shell_key[job_id] = shell_key
    _job_created_at[job_id] = time.monotonic()
    _ensure_cleanup_started()
    return {"job_id": job_id, "status": "started", "note": "check with job_output(job_id)"}


async def job_output(job_id: str) -> dict[str, Any]:
    """Return the background job's result if finished, or its status."""
    task = _jobs.get(job_id)
    if task is None:
        return {"error": f"unknown job: {job_id}"}
    if task.done():
        try:
            result = task.result()
        except asyncio.CancelledError:
            _jobs.pop(job_id, None)
            _job_shell_key.pop(job_id, None)
            _job_created_at.pop(job_id, None)
            return {"job_id": job_id, "status": "cancelled"}
        except Exception as e:  # noqa: BLE001
            _jobs.pop(job_id, None)
            _job_shell_key.pop(job_id, None)
            _job_created_at.pop(job_id, None)
            return {"job_id": job_id, "status": "failed", "error": str(e)}
        _jobs.pop(job_id, None)
        _job_shell_key.pop(job_id, None)
        _job_created_at.pop(job_id, None)
        return {"job_id": job_id, "status": "completed", **result}
    return {"job_id": job_id, "status": "running"}


async def job_kill(job_id: str) -> dict[str, Any]:
    """Cancel a running background job."""
    task = _jobs.get(job_id)
    if task is None:
        return {"error": f"unknown job: {job_id}"}
    task.cancel()
    _jobs.pop(job_id, None)
    _job_shell_key.pop(job_id, None)
    _job_created_at.pop(job_id, None)
    return {"job_id": job_id, "status": "cancelled"}


registry.register(
    name="run_in_background",
    description="Run a command in the background and get a job id immediately. Poll with job_output, stop with job_kill.",
    parameters={
        "type": "object",
        "properties": {"command": {"type": "string"}},
        "required": ["command"],
    },
    handler=run_in_background,
)

registry.register(
    name="job_output",
    description="Fetch a background job's result (completed) or its status (running).",
    parameters={
        "type": "object",
        "properties": {"job_id": {"type": "string"}},
        "required": ["job_id"],
    },
    handler=job_output,
)

registry.register(
    name="job_kill",
    description="Cancel a running background job.",
    parameters={
        "type": "object",
        "properties": {"job_id": {"type": "string"}},
        "required": ["job_id"],
    },
    handler=job_kill,
)
