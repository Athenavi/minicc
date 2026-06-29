"""Shell 执行器 — ShellExecutorTool。

安全地执行 Shell 命令：asyncio 子进程、超时控制、输出截断、环境隔离。
这是 MiniCC 最有价值但也最危险的工具——审批流程是真正的安全屏障。

参考 Claude Code BashTool 设计：
- 危险命令检测与标记
- 输出流式回调
- 退出码分类
- 环境变量安全过滤
"""

from __future__ import annotations

import asyncio
import os
import re
from pathlib import Path
from typing import Callable, Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext
from app.utils.security import PathValidator

OUTPUT_MAX_CHARS = 100_000  # ~100KB
LINE_MAX_CHARS = 10_000
TAIL_RATIO = 0.3  # 截断时保留首尾各 30%

# 危险命令模式（用于展示红色警告，不用于阻止执行）
DANGEROUS_PATTERNS = [
    r"\brm\s+-rf\s+/\b",
    r"\bdd\s+if=",
    r"\bmkfs\b",
    r"\b>/dev/sd",
    r"chmod\s+777",
    r"\bshutdown\b",
    r"\breboot\b",
    r":\(\)\s*\{",
]

# 环境变量黑名单
SENSITIVE_ENV_PATTERNS = [
    re.compile(r, re.IGNORECASE)
    for r in [
        "API_KEY", "APIKEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD",
        "AUTH_TOKEN", "ACCESS_KEY", "SECRET_KEY", "PRIVATE_KEY",
        "MINICC_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY",
    ]
]

# 退出码分类
EXIT_CODE_MESSAGES = {
    1: "General error",
    2: "Misuse of shell builtins",
    126: "Command not executable",
    127: "Command not found",
    128: "Invalid exit argument",
    130: "Terminated by Ctrl+C",
    137: "Killed (out of memory?)",
    139: "Segmentation fault",
}


def is_dangerous_command(command: str) -> tuple[bool, str]:
    """检测命令是否危险，返回 (is_dangerous, reason)。"""
    for pattern in DANGEROUS_PATTERNS:
        if re.search(pattern, command):
            return True, f"matches pattern: {pattern}"
    return False, ""


def classify_exit_code(code: int) -> str:
    """将退出码转为可读描述。"""
    return EXIT_CODE_MESSAGES.get(code, f"Exit code {code}")


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
    is_dangerous: bool = Field(
        default=False,
        description="Marked as potentially dangerous by the model",
    )


class ShellExecutorTool(BaseTool):
    """执行 Shell 命令。需 EXECUTE 权限。"""

    name = "bash"
    description = "Execute a shell command. You MUST explain the command's intent before running it. Prefer non-destructive operations."
    input_schema = ShellExecutorInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.SHELL

    # 流式输出回调（由 WebSocket 通道注入）
    on_output: Optional[Callable[[str, str], None]] = None

    def get_prompt(self) -> str | None:
        return (
            "Execute a shell command on the local system.\n\n"
            "IMPORTANT USAGE RULES:\n"
            "- You MUST explain the command's intent before running it\n"
            "- When a dedicated tool exists (read_file, grep, glob), use it instead of bash\n"
            "- Prefer non-destructive operations\n"
            "- Be extra careful with delete/overwrite commands (rm, dd, > redirect)\n"
            "- Commands timeout after 30 seconds by default\n"
            "- The working directory is the project root by default\n"
            "- Long outputs are automatically truncated\n\n"
            "Do NOT use bash for:\n"
            "- Reading files (use read_file)\n"
            "- Searching code (use grep)\n"
            "- Finding files (use glob)\n"
            "- Editing files (use str_replace_editor or write_to_file)"
        )

    def __init__(self, workspace_dir: str | Path = ".") -> None:
        super().__init__()
        self._workspace_dir = Path(workspace_dir).resolve()
        self._validator = PathValidator(workspace_dir)

    async def execute(self, input_data: ShellExecutorInput, context: ToolUseContext | None = None) -> ToolResult:
        # 工作目录验证
        cwd = self._workspace_dir
        if input_data.workdir:
            try:
                cwd = self._validator.validate(input_data.workdir)
            except PermissionError as e:
                return ToolResult(tool_call_id="", output=str(e), is_error=True)

        # 危险命令检测（仅标记，不阻止）
        danger, _ = is_dangerous_command(input_data.command)
        warning = ""
        if danger:
            warning = "\n[! WARNING: This command appears potentially dangerous]\n"

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
                async def read_stream(stream, lines: list[str], label: str):
                    async for line_bytes in stream:
                        decoded = line_bytes.decode("utf-8", errors="replace")
                        lines.append(decoded)
                        # 流式回调
                        if self.on_output:
                            self.on_output(decoded, label)

                async with asyncio.TaskGroup() as tg:
                    tg.create_task(read_stream(process.stdout, stdout_lines, "stdout"))
                    tg.create_task(read_stream(process.stderr, stderr_lines, "stderr"))

        except asyncio.TimeoutError:
            process.kill()
            output = self._truncate_output("".join(stdout_lines[-20:]))
            return ToolResult(
                tool_call_id="",
                output=f"{warning}[timeout: {input_data.timeout}s]\n{output}",
                is_error=True,
            )
        except Exception as exc:
            process.kill()
            return ToolResult(tool_call_id="", output=f"Execution error: {exc}", is_error=True)

        exit_code = await process.wait()

        # 组合输出
        output_parts = [warning] if warning else []
        if stdout_lines:
            output_parts.append(self._truncate_output("".join(stdout_lines)))
        if stderr_lines:
            output_parts.append(f"[stderr]\n{self._truncate_output(''.join(stderr_lines))}")

        output = "".join(output_parts)
        exit_desc = classify_exit_code(exit_code)
        metadata = {"exit_code": exit_code, "exit_description": exit_desc}

        if exit_code != 0:
            return ToolResult(
                tool_call_id="",
                output=f"{warning}[{exit_desc}]\n{output}",
                is_error=True,
            )

        return ToolResult(tool_call_id="", output=output, metadata=metadata)

    def _get_safe_env(self) -> dict[str, str]:
        """生成安全的子进程环境变量。"""
        env = dict(os.environ)
        env["DEBIAN_FRONTEND"] = "noninteractive"
        env["PAGER"] = "cat"
        env["GIT_PAGER"] = "cat"

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

        head_len = int(OUTPUT_MAX_CHARS * TAIL_RATIO)
        tail_len = OUTPUT_MAX_CHARS - head_len
        return f"{full[:head_len]}\n... [truncated: {len(full)} chars total] ...\n{full[-tail_len:]}"
