"""Bash 命令工具 — 命令解析、安全分析、语义分类。

对应 Claude Code BashTool 设计：
- 命令语义分类（search/read/list vs 其他）
- 命令解析（管道、链接符）
- 只读约束检查
- 危险命令检测
- 流式输出回调
- 退出码分类
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

OUTPUT_MAX_CHARS = 100_000
LINE_MAX_CHARS = 10_000
TAIL_RATIO = 0.3

SENSITIVE_ENV_PATTERNS = [
    re.compile(r, re.IGNORECASE)
    for r in [
        "API_KEY", "APIKEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD",
        "AUTH_TOKEN", "ACCESS_KEY", "SECRET_KEY", "PRIVATE_KEY",
        "MINICC_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY",
    ]
]

DANGEROUS_PATTERNS = [
    r"\brm\s+-rf(\s+|$)", r"\bdd\s+if=", r"\bmkfs\b",
    r">/dev/sd", r"chmod\s+777", r"\bshutdown\b", r"\breboot\b",
    r":\(\s*\{",
]

EXIT_CODE_MESSAGES = {
    1: "General error", 2: "Misuse of shell builtins",
    126: "Command not executable", 127: "Command not found",
    128: "Invalid exit argument", 130: "Terminated by Ctrl+C",
    137: "Killed (OOM?)", 139: "Segmentation fault",
}

READ_ONLY_COMMANDS = {
    "cat", "head", "tail", "less", "more", "grep", "egrep", "fgrep",
    "find", "locate", "which", "whereis", "pwd", "ls", "echo", "printf",
    "env", "printenv", "date", "whoami", "id", "uname", "hostname",
    "git status", "git log", "git diff", "git show", "git branch",
    "npm list", "pip list", "cargo metadata",
}

SEARCH_COMMANDS = {"grep", "egrep", "fgrep", "find", "locate", "ag", "rg", "ack"}


# ── 命令解析 ───────────────────────────────────────────


def split_command_with_operators(command: str) -> list[str]:
    """将命令按操作符分割为片段。

    对应 Claude Code 的 splitCommandWithOperators()。
    正确处理管道、&&、||、; 等链接符。
    """
    # 按优先级分割：; > || > && > |
    for sep in [";", "||", "&&", "|"]:
        parts = _split_respecting_quotes(command, sep)
        if len(parts) > 1:
            return [p.strip() for p in parts]
    return [command.strip()]


def _split_respecting_quotes(text: str, sep: str) -> list[str]:
    """分割字符串但尊重引号内的内容。"""
    result = []
    current = []
    in_single = False
    in_double = False
    i = 0
    while i < len(text):
        ch = text[i]
        if ch == "'" and not in_double:
            in_single = not in_single
        elif ch == '"' and not in_single:
            in_double = not in_double
        elif not in_single and not in_double and text[i:i+len(sep)] == sep:
            result.append("".join(current))
            current = []
            i += len(sep)
            continue
        current.append(ch)
        i += 1
    result.append("".join(current))
    return result


def get_first_word(cmd_part: str) -> str:
    """获取命令片段的第一个单词。"""
    return cmd_part.strip().split(maxsplit=1)[0] if cmd_part.strip() else ""


# ── 语义分类 ───────────────────────────────────────────


def is_search_or_read_bash_command(command: str) -> dict:
    """判断命令是搜索、读取还是列表操作。

    对应 Claude Code 的 isSearchOrReadBashCommand()。
    """
    result = {"is_search": False, "is_read": False, "is_list": False}

    parts = split_command_with_operators(command)
    if not parts:
        return result

    first_cmd = get_first_word(parts[0]).lower()

    # 搜索命令
    if first_cmd in SEARCH_COMMANDS:
        result["is_search"] = True
        return result

    # 读取命令
    if first_cmd in ("cat", "head", "tail", "less", "more", "echo", "printf"):
        result["is_read"] = True
        return result

    # 列表命令
    if first_cmd in ("ls", "find", "locate"):
        result["is_list"] = True
        return result

    # Git 只读操作
    if first_cmd == "git" and len(parts[0].split()) > 1:
        git_sub = parts[0].split()[1].lower()
        if git_sub in ("status", "log", "diff", "show", "branch", "remote", "config"):
            result["is_read"] = True
            return result

    return result


# ── 只读约束检查 ──


def check_read_only_constraints(command: str) -> tuple[bool, str]:
    """检查命令是否在只读约束下安全执行。

    对应 Claude Code 的 checkReadOnlyConstraints()。
    Returns (is_readonly_safe, reason_if_not)。
    """
    parts = split_command_with_operators(command)
    if not parts:
        return False, "Empty command"

    first_cmd = get_first_word(parts[0]).lower()
    main_cmd = parts[0].strip()

    # 已知只读命令
    for ro_cmd in READ_ONLY_COMMANDS:
        if main_cmd.startswith(ro_cmd):
            return True, ""

    # 写操作标记
    write_markers = [">", ">>", "rm ", "mv ", "cp ", "mk", "touch", "chmod", "chown", "ln ", "dd "]
    for marker in write_markers:
        if marker in main_cmd:
            return False, f"Command contains write operation: {marker}"

    return False, "Unknown command — assuming non-readonly"


# ── 工具函数 ──


def is_dangerous_command(command: str) -> tuple[bool, str]:
    for pattern in DANGEROUS_PATTERNS:
        if re.search(pattern, command):
            return True, f"matches pattern: {pattern}"
    return False, ""


def classify_exit_code(code: int) -> str:
    return EXIT_CODE_MESSAGES.get(code, f"Exit code {code}")


class ShellExecutorInput(BaseModel):
    command: str = Field(description="Shell command to execute", max_length=4096)
    description: Optional[str] = Field(default=None, description="Intent explanation for approval display")
    timeout: int = Field(default=30, ge=1, le=300, description="Timeout in seconds")
    workdir: Optional[str] = Field(default=None, description="Working directory")
    is_dangerous: bool = Field(default=False, description="Marked as potentially dangerous")


class ShellExecutorTool(BaseTool):
    """执行 Shell 命令。需 EXECUTE 权限。"""

    name = "bash"
    description = "Execute a shell command. You MUST explain the command's intent before running it."
    input_schema = ShellExecutorInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.SHELL

    on_output: Optional[Callable[[str, str], None]] = None

    def get_prompt(self) -> str | None:
        return (
            "Execute a shell command on the local system.\n\n"
            "IMPORTANT:\n"
            "- Explain the command's intent before running it\n"
            "- When a dedicated tool exists (read_file, grep, glob), use it instead\n"
            "- Prefer non-destructive operations\n"
            "- Be extra careful with delete/overwrite commands\n"
            "- Commands timeout after 30s by default\n"
            "- Long outputs are auto-truncated\n\n"
            "Do NOT use bash for reading files, searching code, finding files, or editing files."
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

        # 语义分类 + 只读检查
        semantics = is_search_or_read_bash_command(input_data.command)
        is_readonly_safe, readonly_reason = check_read_only_constraints(input_data.command)

        # 危险命令检测
        danger, _ = is_dangerous_command(input_data.command)

        # 构建输出 header
        header_parts = []
        if semantics["is_search"]:
            header_parts.append("[search]")
        elif semantics["is_read"]:
            header_parts.append("[read]")
        elif semantics["is_list"]:
            header_parts.append("[list]")
        if is_readonly_safe:
            header_parts.append("[readonly]")
        if danger:
            header_parts.append("[! DANGEROUS]")

        header = " ".join(header_parts)
        if header:
            header += "\n"

        try:
            process = await asyncio.create_subprocess_exec(
                "bash", "-c", input_data.command,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
                cwd=cwd,
                env=self._get_safe_env(),
            )
        except FileNotFoundError:
            return ToolResult(tool_call_id="", output="bash not found", is_error=True)

        stdout_lines: list[str] = []
        stderr_lines: list[str] = []

        try:
            async with asyncio.timeout(input_data.timeout):
                async def read_stream(stream, lines, label):
                    async for line_bytes in stream:
                        decoded = line_bytes.decode("utf-8", errors="replace")
                        lines.append(decoded)
                        if self.on_output:
                            self.on_output(decoded, label)

                async with asyncio.TaskGroup() as tg:
                    tg.create_task(read_stream(process.stdout, stdout_lines, "stdout"))
                    tg.create_task(read_stream(process.stderr, stderr_lines, "stderr"))

        except asyncio.TimeoutError:
            process.kill()
            return ToolResult(
                tool_call_id="",
                output=f"{header}[timeout: {input_data.timeout}s]\n{self._truncate_output(''.join(stdout_lines[-20:]))}",
                is_error=True,
            )
        except Exception as exc:
            process.kill()
            return ToolResult(tool_call_id="", output=f"Execution error: {exc}", is_error=True)

        exit_code = await process.wait()

        output_parts = [header]
        if stdout_lines:
            output_parts.append(self._truncate_output("".join(stdout_lines)))
        if stderr_lines:
            output_parts.append(f"[stderr]\n{self._truncate_output(''.join(stderr_lines))}")

        output = "".join(output_parts)
        exit_desc = classify_exit_code(exit_code)
        metadata = {"exit_code": exit_code, "exit_description": exit_desc, "semantics": semantics}

        if exit_code != 0:
            return ToolResult(tool_call_id="", output=f"{output}[{exit_desc}]", is_error=True)

        return ToolResult(tool_call_id="", output=output, metadata=metadata)

    def _get_safe_env(self) -> dict[str, str]:
        env = dict(os.environ)
        env["DEBIAN_FRONTEND"] = "noninteractive"
        env["PAGER"] = "cat"
        env["GIT_PAGER"] = "cat"
        for key in list(env.keys()):
            for pattern in SENSITIVE_ENV_PATTERNS:
                if pattern.search(key):
                    del env[key]
                    break
        return env

    @staticmethod
    def _truncate_output(output: str) -> str:
        if len(output) <= OUTPUT_MAX_CHARS:
            return output
        truncated_lines = []
        for line in output.splitlines(keepends=True):
            if len(line) > LINE_MAX_CHARS:
                truncated_lines.append(line[:5000] + f"\n... [line truncated: {len(line)} chars] ...\n" + line[-5000:])
            else:
                truncated_lines.append(line)
        full = "".join(truncated_lines)
        if len(full) <= OUTPUT_MAX_CHARS:
            return full
        head_len = int(OUTPUT_MAX_CHARS * TAIL_RATIO)
        return f"{full[:head_len]}\n... [truncated: {len(full)} chars total] ...\n{full[-OUTPUT_MAX_CHARS+head_len:]}"
