"""jobs — 后台任务（对应 deepseek-harness dsh-tool-jobs）

run_in_background 将命令放入后台执行（asyncio task），立即返回 job_id；
job_output 轮询结果，job_kill 取消。命令在持久 shell 中执行（跨调用状态
共享），与 foreground 的 persistent_shell 同一后端。
"""
from __future__ import annotations

import asyncio
import logging
import uuid
from typing import Any, Optional

from app.tools.context import get_session_id
from app.tools.registry import registry

logger = logging.getLogger(__name__)

_jobs: dict[str, asyncio.Task] = {}
_job_shell_key: dict[str, str] = {}  # job_id → shell key（session 隔离）


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
            return {"job_id": job_id, "status": "cancelled"}
        except Exception as e:  # noqa: BLE001
            return {"job_id": job_id, "status": "failed", "error": str(e)}
        _jobs.pop(job_id, None)
        _job_shell_key.pop(job_id, None)
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
