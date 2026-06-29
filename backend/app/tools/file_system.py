"""文件系统工具集 — ReadFile / WriteToFile / StrReplaceEditor。

参考 Claude Code 的 FileReadTool / FileWriteTool / FileEditTool 设计。

关键优化：
1. 文件变更检测：通过内容哈希判断文件是否被修改过
2. 分页体验：显示上下文行 + 进度指示
3. 原子写入 + 备份：写入前备份原文件到 .minicc/.backup/
4. 精确编辑：上下文行显示、undo 支持
"""

from __future__ import annotations

import difflib
import hashlib
import os
from datetime import datetime, timezone
from pathlib import Path
from tempfile import NamedTemporaryFile
from typing import Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolUseContext
from app.utils.security import PathValidator

MAX_FILE_SIZE = 1_024 * 1_024  # 1MB
LINE_NUMBER_WIDTH = 6
CONTEXT_LINES = 3  # 编辑时显示的上下文行数


# ── 文件状态工具 ─────────────────────────────────────────


def compute_file_hash(path: Path) -> str:
    """计算文件内容的 MD5 哈希。"""
    return hashlib.md5(path.read_bytes()).hexdigest()


def backup_file(path: Path) -> Optional[Path]:
    """备份文件到 .minicc/.backup/。返回备份路径。"""
    try:
        backup_dir = path.parent / ".minicc" / ".backup"
        backup_dir.mkdir(parents=True, exist_ok=True)
        stamp = datetime.now(timezone.utc).strftime("%Y%m%d_%H%M%S")
        backup_path = backup_dir / f"{path.name}.{stamp}.bak"
        backup_path.write_bytes(path.read_bytes())
        return backup_path
    except Exception:
        return None


def format_line_number(lineno: int, width: int = LINE_NUMBER_WIDTH) -> str:
    """格式化行号，右对齐。"""
    return str(lineno).rjust(width)


def make_diff_preview(original: str, modified: str, file_path: str = "file", max_lines: int = 80) -> str:
    """生成 Unified Diff，限制最大行数。"""
    diff = difflib.unified_diff(
        original.splitlines(keepends=True),
        modified.splitlines(keepends=True),
        fromfile=f"a/{file_path}",
        tofile=f"b/{file_path}",
    )
    lines = list(diff)
    if len(lines) > max_lines:
        lines = lines[:max_lines]
        lines.append("...\n[diff truncated: too many lines]")
    return "".join(lines)


# ── Diff 生成器 ─────────────────────────────────────────


class DiffGenerator:
    """生成 Unified Diff，用于审批预览。"""

    @staticmethod
    def generate_diff(original: str, modified: str, file_path: str = "file") -> str:
        return make_diff_preview(original, modified, file_path)

    @staticmethod
    def generate_diff_for_tool(file_path: str, tool_name: str, tool_input: dict) -> str | None:
        path = Path(file_path)
        if not path.exists():
            if tool_name == "write_to_file":
                return f"(new file: {file_path})"
            return None

        original = path.read_text(encoding="utf-8")

        if tool_name == "write_to_file":
            modified = tool_input.get("content", "")
        elif tool_name == "str_replace_editor":
            old = tool_input.get("old_string", "")
            new = tool_input.get("new_string", "")
            replace_all = tool_input.get("replace_all", False)
            modified = original.replace(old, new) if replace_all else original.replace(old, new, 1)
        else:
            return None

        return make_diff_preview(original, modified, file_path)


# ── 读取工具 ───────────────────────────────────────────


class ReadFileToolInput(BaseModel):
    path: str = Field(description="File path (relative to workspace or absolute)")
    offset: int = Field(default=0, ge=0, description="Starting line (0-indexed)")
    limit: int = Field(default=100, ge=1, le=2000, description="Max lines to return")
    show_line_numbers: bool = Field(default=True, description="Show line numbers in output")


class ReadFileTool(BaseTool):
    """读取文件内容，支持行号显示和分页。"""

    name = "read_file"
    description = "Read a file's contents with line numbers. Use for viewing source code or config files."
    input_schema = ReadFileToolInput
    permission_level = PermissionLevel.READ

    def __init__(self, workspace_dir: str | Path = ".") -> None:
        super().__init__()
        self._validator = PathValidator(workspace_dir)

    async def execute(self, input_data: ReadFileToolInput, context: ToolUseContext | None = None) -> ToolResult:
        try:
            path = self._validator.validate(input_data.path)
        except PermissionError as e:
            return ToolResult(tool_call_id="", output=str(e), is_error=True)

        if not path.exists():
            return ToolResult(tool_call_id="", output=f"File not found: {input_data.path}", is_error=True)
        if not path.is_file():
            return ToolResult(tool_call_id="", output=f"Not a file: {input_data.path}", is_error=True)

        # Binary detection
        if _is_binary(path):
            return ToolResult(tool_call_id="", output=f"(binary file: {path.name})")

        # Size check
        size = path.stat().st_size
        if size > MAX_FILE_SIZE:
            return ToolResult(tool_call_id="", output=f"(file too large: {size / 1024 / 1024:.1f}MB, max 1MB)")

        content = path.read_text(encoding="utf-8")
        lines = content.splitlines()
        total = len(lines)

        # Apply offset/limit
        start = input_data.offset
        end = start + input_data.limit
        selected = lines[start:end]

        # Format with line numbers
        if input_data.show_line_numbers:
            result_lines = []
            for i, line in enumerate(selected):
                lineno = start + i + 1
                result_lines.append(f"{format_line_number(lineno)}|{line}")
        else:
            result_lines = list(selected)

        # Pagination footer
        footer_parts = []
        if end < total:
            footer_parts.append(f"lines {start+1}-{end} of {total}")
        else:
            footer_parts.append(f"all {total} lines shown")

        # File hash for change detection
        file_hash = compute_file_hash(path)
        footer_parts.append(f"hash: {file_hash[:12]}")

        result_lines.append("")
        result_lines.append(f"[{'] ['.join(footer_parts)}]")

        output = "\n".join(result_lines)
        metadata = {
            "total_lines": total,
            "returned_lines": len(selected),
            "file_hash": file_hash,
            "file_size": size,
        }

        return ToolResult(tool_call_id="", output=output, metadata=metadata)


# ── 写入工具 ───────────────────────────────────────────


class WriteToFileToolInput(BaseModel):
    path: str = Field(description="File path")
    content: str = Field(description="Complete file content")
    create_parents: bool = Field(default=True, description="Auto-create parent directories")
    backup: bool = Field(default=True, description="Backup existing file before overwrite")


class WriteToFileTool(BaseTool):
    """创建新文件或完全覆盖已有文件。"""

    name = "write_to_file"
    description = "Create a new file or overwrite an existing one. Use for new files or large changes."
    input_schema = WriteToFileToolInput
    permission_level = PermissionLevel.WRITE

    def __init__(self, workspace_dir: str | Path = ".") -> None:
        super().__init__()
        self._validator = PathValidator(workspace_dir)

    async def execute(self, input_data: WriteToFileToolInput, context: ToolUseContext | None = None) -> ToolResult:
        try:
            path = self._validator.validate(input_data.path)
        except PermissionError as e:
            return ToolResult(tool_call_id="", output=str(e), is_error=True)

        # Backup existing file
        backup_path = None
        if input_data.backup and path.exists():
            backup_path = backup_file(path)

        # Generate diff preview
        diff = DiffGenerator.generate_diff_for_tool(str(path), self.name, input_data.model_dump())

        # Atomic write (tmp + rename)
        try:
            if input_data.create_parents:
                path.parent.mkdir(parents=True, exist_ok=True)

            tmp = path.with_suffix(f"{path.suffix}.tmp")
            tmp.write_text(input_data.content, encoding="utf-8")
            tmp.replace(path)

            action = "updated" if path.exists() else "created"
            output_parts = [f"File {action}: {input_data.path}"]
            if diff:
                output_parts.append(f"\n{diff}")
            if backup_path:
                output_parts.append(f"\n[backup saved to: {backup_path.name}]")

            return ToolResult(
                tool_call_id="",
                output="\n".join(output_parts),
                metadata={"action": action, "path": str(path), "backup": str(backup_path) if backup_path else None},
            )

        except Exception as e:
            return ToolResult(tool_call_id="", output=f"Write failed: {e}", is_error=True)


# ── 编辑工具 ───────────────────────────────────────────


class StrReplaceEditorInput(BaseModel):
    path: str = Field(description="File path")
    old_string: str = Field(description="Exact string to replace (must match uniquely)")
    new_string: str = Field(description="Replacement string")
    replace_all: bool = Field(default=False, description="Replace all occurrences")
    backup: bool = Field(default=True, description="Backup file before edit")


class StrReplaceEditorTool(BaseTool):
    """在文件中进行精确字符串替换编辑。"""

    name = "str_replace_editor"
    description = "Edit a file by replacing an exact string. Best for targeted code changes."
    input_schema = StrReplaceEditorInput
    permission_level = PermissionLevel.WRITE

    def __init__(self, workspace_dir: str | Path = ".") -> None:
        super().__init__()
        self._validator = PathValidator(workspace_dir)

    async def execute(self, input_data: StrReplaceEditorInput, context: ToolUseContext | None = None) -> ToolResult:
        try:
            path = self._validator.validate(input_data.path)
        except PermissionError as e:
            return ToolResult(tool_call_id="", output=str(e), is_error=True)

        if not path.exists():
            return ToolResult(tool_call_id="", output=f"File not found: {input_data.path}", is_error=True)

        try:
            content = path.read_text(encoding="utf-8")
        except Exception as e:
            return ToolResult(tool_call_id="", output=f"Read failed: {e}", is_error=True)

        # Count occurrences
        count = content.count(input_data.old_string)

        if count == 0:
            return ToolResult(tool_call_id="", output=f"No match found for string in {input_data.path}", is_error=True)

        if count > 1 and not input_data.replace_all:
            return ToolResult(tool_call_id="", output=f"Found {count} matches. Use replace_all=True or provide a more specific match.", is_error=True)

        # Backup
        backup_path = None
        if input_data.backup:
            backup_path = backup_file(path)

        # Apply replacement
        if input_data.replace_all:
            modified = content.replace(input_data.old_string, input_data.new_string)
        else:
            modified = content.replace(input_data.old_string, input_data.new_string, 1)

        # Generate diff
        diff = make_diff_preview(content, modified, str(input_data.path))

        # Atomic write
        try:
            tmp = path.with_suffix(f"{path.suffix}.tmp")
            tmp.write_text(modified, encoding="utf-8")
            tmp.replace(path)
        except Exception as e:
            return ToolResult(tool_call_id="", output=f"Write failed: {e}", is_error=True)

        # Find context lines for old_string
        context_info = ""
        if count == 1 and not input_data.replace_all:
            idx = content.index(input_data.old_string)
            lines = content[:idx].splitlines()
            before_line = len(lines)
            context_lines = content.splitlines()
            start = max(0, before_line - CONTEXT_LINES - 1)
            end = min(len(context_lines), before_line + CONTEXT_LINES)
            ctx = context_lines[start:end]
            ctx_str = "\n".join(f"{format_line_number(start+i+1)}|{line}" for i, line in enumerate(ctx))
            context_info = f"\n\nEdit location (lines {start+1}-{end}):\n{ctx_str}"

        output_parts = [f"Applied edit to {input_data.path} ({count} occurrence(s))"]
        if context_info:
            output_parts.append(context_info)
        output_parts.append(f"\n{diff}")
        if backup_path:
            output_parts.append(f"\n[backup saved to: {backup_path.name}]")

        return ToolResult(tool_call_id="", output="\n".join(output_parts))


# ── 工具组注册 ─────────────────────────────────────────


def register_file_tools(registry, workspace_dir: str | Path = ".") -> None:
    """注册所有文件系统工具到 registry。"""
    registry.register(ReadFileTool(workspace_dir))
    registry.register(WriteToFileTool(workspace_dir))
    registry.register(StrReplaceEditorTool(workspace_dir))


# ── 工具函数 ───────────────────────────────────────────


def _is_binary(path: Path) -> bool:
    try:
        with open(path, "rb") as f:
            chunk = f.read(512)
        return b"\0" in chunk
    except Exception:
        return True
