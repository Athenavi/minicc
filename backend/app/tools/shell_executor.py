"""Shell 执行器 — ShellExecutorTool。

安全地执行 Shell 命令：asyncio 子进程、超时控制、输出截断、环境隔离。
这是 MiniCC 最有价值但也最危险的工具——审批流程是真正的安全屏障。
"""

from __future__ import annotations

import asyncio
import os
import re
from pathlib import Path
from typing import Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool
from app.utils.security import PathValidator

OUTPUT_MAX_CHARS = 100_000  # ~100KB
LINE_MAX_CHARS = 10_000
TAIL_RATIO = 0.3  # 截断时保留首尾各 30%

# 环境变量黑名单（大小写不敏感匹配）
SENSITIVE_ENV_PATTERNS = [
    re.compile(r, re.IGNORECASE)
    for r in [
        "API_KEY", "APIKEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD",
        "AUTH_TOKEN", "ACCESS_KEY", "SECRET_KEY", "PRIVATE_KEY",
        "MINICC_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY",
    ]
]


class ShellExecutorInput(BaseModel):
    command: str = Field(
        description="Shell command to execute",
        max_length=4096,
    )
    description: Optional[str] = Field(
        default=None,
        description="Natural language explanation of the command's intent (for approval display)",
    )
    timeout: int = Field(
        default=30,
        ge=1,
        le=300,
        description="Timeout in seconds",
    )
    workdir: Optional[str] = Field(
        default=None,
        description="Working directory (defaults to workspace_dir)",
    )


class ShellExecutorTool(BaseTool):
    """执行 Shell 命令。需 EXECUTE 权限。"""

    name = "bash"
    description = """Execute a shell command. You MUST explain the command's intent before running it.
Prefer non-destructive operations. Be extra careful with delete/overwrite commands."""
    input_schema = ShellExecutorInput
    permission_level = PermissionLevel.EXECUTE

    def __init__(self, workspace_dir: str | Path = ".") -> None:
        super().__init__()
        self._workspace_dir = Path(workspace_dir).resolve()
        self._validator = PathValidator(workspace_dir)

    async def execute(self, input_data: ShellExecutorInput) -> ToolResult:
        # 工作目录验证
        cwd = self._workspace_dir
        if input_data.workdir:
            try:
                cwd = self._validator.validate(input_data.workdir)
            except PermissionError as e:
                return ToolResult(tool_call_id="", output=str(e), is_error=True)

        try:
            process = await asyncio.create_subprocess_exec(
                "bash", "-c", input_data.command,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
                cwd=cwd,
                env=self._get_safe_env(),
            )
        except FileNotFoundError:
            return ToolResult(tool_call_id="", output="bash not found on this system", is_error=True)

        stdout_lines: list[str] = []
        stderr_lines: list[str] = []

        try:
            async with asyncio.timeout(input_data.timeout):
                # 读取 stdout 和 stderr
                async def read_stream(stream, lines: list[str], label: str):
                    async for line_bytes in stream:
                        decoded = line_bytes.decode("utf-8", errors="replace")
                        lines.append(decoded)

                async with asyncio.TaskGroup() as tg:
                    tg.create_task(read_stream(process.stdout, stdout_lines, "stdout"))
                    tg.create_task(read_stream(process.stderr, stderr_lines, "stderr"))

        except asyncio.TimeoutError:
            process.kill()
            output = self._truncate_output("".join(stdout_lines[-20:]))
            return ToolResult(
                tool_call_id="",
                output=f"[timeout: {input_data.timeout}s]\n{output}",
                is_error=True,
            )
        except Exception as exc:
            process.kill()
            return ToolResult(
                tool_call_id="",
                output=f"Execution error: {exc}",
                is_error=True,
            )

        exit_code = await process.wait()

        # 组合输出
        output_parts = []
        if stdout_lines:
            output_parts.append(self._truncate_output("".join(stdout_lines)))
        if stderr_lines:
            output_parts.append(f"[stderr]\n{self._truncate_output(''.join(stderr_lines))}")

        output = "".join(output_parts)
        metadata = {"exit_code": exit_code}

        if exit_code != 0:
            return ToolResult(
                tool_call_id="",
                output=f"[exit code: {exit_code}]\n{output}",
                is_error=True,
            )

        return ToolResult(tool_call_id="", output=output, metadata=metadata)

    def _get_safe_env(self) -> dict[str, str]:
        """生成安全的子进程环境变量。"""
        env = dict(os.environ)

        # 设置安全默认值
        env["DEBIAN_FRONTEND"] = "noninteractive"
        env["PAGER"] = "cat"
        env["GIT_PAGER"] = "cat"

        # 移除敏感变量
        keys_to_remove = []
        for key in env:
            for pattern in SENSITIVE_ENV_PATTERNS:
                if pattern.search(key):
                    keys_to_remove.append(key)
                    break

        for key in keys_to_remove:
            del env[key]

        return env

    @staticmethod
    def _truncate_output(output: str) -> str:
        """截断超长输出。"""
        if len(output) <= OUTPUT_MAX_CHARS:
            return output

        # 截断超长行
        truncated_lines = []
        for line in output.splitlines(keepends=True):
            if len(line) > LINE_MAX_CHARS:
                truncated_lines.append(
                    line[:5000] + f"\n... [line truncated: {len(line)} chars] ...\n" + line[-5000:]
                )
            else:
                truncated_lines.append(line)

        full = "".join(truncated_lines)

        if len(full) <= OUTPUT_MAX_CHARS:
            return full

        # 保留首尾
        head_len = int(OUTPUT_MAX_CHARS * TAIL_RATIO)
        tail_len = OUTPUT_MAX_CHARS - head_len
        head = full[:head_len]
        tail = full[-tail_len:]
        return f"{head}\n... [truncated: {len(full)} chars total] ...\n{tail}"
