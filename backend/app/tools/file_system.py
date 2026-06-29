"""文件系统工具集 — ReadFile / WriteToFile / StrReplaceEditor。

参考 Claude Code 的 FileReadTool / FileWriteTool / FileEditTool 设计。
"""

from __future__ import annotations

import difflib
import os
from pathlib import Path
from tempfile import NamedTemporaryFile
from typing import Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool
from app.utils.security import PathValidator

MAX_FILE_SIZE = 1_024 * 1_024  # 1MB
LINE_NUMBER_WIDTH = 6


# ── Diff 生成器 ─────────────────────────────────────────


class DiffGenerator:
    """生成 Unified Diff，用于审批预览。"""

    @staticmethod
    def generate_diff(original: str, modified: str, file_path: str = "file") -> str:
        """返回 Unified Diff 格式字符串。"""
        diff = difflib.unified_diff(
            original.splitlines(keepends=True),
            modified.splitlines(keepends=True),
            fromfile=f"a/{file_path}",
            tofile=f"b/{file_path}",
        )
        return "".join(diff)

    @staticmethod
    def generate_diff_for_tool(file_path: str, tool_name: str, tool_input: dict) -> str | None:
        """根据工具类型和输入，生成操作的 diff 预览。"""
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
            if replace_all:
                modified = original.replace(old, new)
            else:
                modified = original.replace(old, new, 1)
        else:
            return None

        return DiffGenerator.generate_diff(original, modified, file_path)


# ── 读取工具 ───────────────────────────────────────────


class ReadFileToolInput(BaseModel):
    path: str = Field(description="File path (relative to workspace or absolute)")
    offset: int = Field(default=0, ge=0, description="Starting line (0-indexed)")
    limit: int = Field(default=100, ge=1, le=2000, description="Max lines to return")


class ReadFileTool(BaseTool):
    """读取文件内容，支持行号显示和分页。"""

    name = "read_file"
    description = "Read a file's contents with line numbers. Use for viewing source code or config files."
    input_schema = ReadFileToolInput
    permission_level = PermissionLevel.READ

    def get_prompt(self) -> str | None:
        return (
            "Reads a file from the local filesystem.\n\n"
            "Usage:\n"
            "- The path parameter must be an absolute or workspace-relative path\n"
            "- By default, reads up to 100 lines starting from the beginning\n"
            "- Use offset and limit to paginate through large files\n"
            "- This tool can only read files, not directories (use bash ls for directories)\n"
            "- Binary files will be detected and skipped"
        )
    description = "Read a file's contents with line numbers. Use for viewing source code or config files."
    input_schema = ReadFileToolInput
    permission_level = PermissionLevel.READ

    def __init__(self, workspace_dir: str | Path = ".") -> None:
        super().__init__()
        self._validator = PathValidator(workspace_dir)

    async def execute(self, input_data: ReadFileToolInput) -> ToolResult:
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
            return ToolResult(
                tool_call_id="",
                output=f"(binary file: {path.name})",
            )

        # Size check
        size = path.stat().st_size
        if size > MAX_FILE_SIZE:
            return ToolResult(
                tool_call_id="",
                output=f"(file too large: {size / 1024 / 1024:.1f}MB, max 1MB)",
            )

        content = path.read_text(encoding="utf-8")
        lines = content.splitlines()
        total = len(lines)

        # Apply offset/limit
        start = input_data.offset
        end = start + input_data.limit
        selected = lines[start:end]

        # Format with line numbers
        result_lines = []
        for i, line in enumerate(selected):
            lineno = start + i + 1
            result_lines.append(f"{str(lineno).rjust(LINE_NUMBER_WIDTH)}|{line}")

        if end < total:
            result_lines.append(f"\n[{end}-{total} of {total} lines remain]")

        output = "\n".join(result_lines)
        metadata = {"total_lines": total, "returned_lines": len(selected)}

        return ToolResult(tool_call_id="", output=output, metadata=metadata)


# ── 写入工具 ───────────────────────────────────────────


class WriteToFileToolInput(BaseModel):
    path: str = Field(description="File path")
    content: str = Field(description="Complete file content")
    create_parents: bool = Field(default=True, description="Auto-create parent directories")


class WriteToFileTool(BaseTool):
    """创建新文件或完全覆盖已有文件。"""

    name = "write_to_file"
    description = "Create a new file or overwrite an existing one. Use for new files or large changes."
    input_schema = WriteToFileToolInput
    permission_level = PermissionLevel.WRITE

    def get_prompt(self) -> str | None:
        return (
            "Creates a new file or completely overwrites an existing file.\n\n"
            "Use this for:\n"
            "- Creating new files\n"
            "- Making large changes that affect most of a file\n"
            "- When str_replace_editor would need too many replacements\n\n"
            "For small, targeted changes, prefer str_replace_editor instead."
        )

    def __init__(self, workspace_dir: str | Path = ".") -> None:
        super().__init__()
        self._validator = PathValidator(workspace_dir)

    async def execute(self, input_data: WriteToFileToolInput) -> ToolResult:
        try:
            path = self._validator.validate(input_data.path)
        except PermissionError as e:
            return ToolResult(tool_call_id="", output=str(e), is_error=True)

        # Generate diff preview
        diff = DiffGenerator.generate_diff_for_tool(str(path), self.name, input_data.model_dump())

        # Atomic write
        try:
            if input_data.create_parents:
                path.parent.mkdir(parents=True, exist_ok=True)

            tmp = path.with_suffix(f"{path.suffix}.tmp")
            tmp.write_text(input_data.content, encoding="utf-8")
            tmp.replace(path)

            action = "updated" if path.exists() else "created"
            output = f"File {action}: {input_data.path}"
            if diff:
                output += f"\n\n{diff}"

            return ToolResult(tool_call_id="", output=output, metadata={"action": action, "path": str(path)})

        except Exception as e:
            return ToolResult(tool_call_id="", output=f"Write failed: {e}", is_error=True)


# ── 编辑工具 ───────────────────────────────────────────


class StrReplaceEditorInput(BaseModel):
    path: str = Field(description="File path")
    old_string: str = Field(description="Exact string to replace (must match uniquely)")
    new_string: str = Field(description="Replacement string")
    replace_all: bool = Field(default=False, description="Replace all occurrences")


class StrReplaceEditorTool(BaseTool):
    """在文件中进行精确字符串替换编辑。"""

    name = "str_replace_editor"
    description = "Edit a file by replacing an exact string. Best for targeted code changes."
    input_schema = StrReplaceEditorInput
    permission_level = PermissionLevel.WRITE

    def get_prompt(self) -> str | None:
        return (
            "Edit a file by replacing an exact string match.\n\n"
            "Use this for:\n"
            "- Fixing a bug in a specific function\n"
            "- Changing a variable name or string literal\n"
            "- Making small, targeted modifications\n\n"
            "Rules:\n"
            "- old_string must match EXACTLY once in the file\n"
            "- If it matches multiple times, use replace_all=True\n"
            "- If no match is found, check the file content first with read_file\n"
            "- For large changes affecting most of a file, use write_to_file instead"
        )

    def __init__(self, workspace_dir: str | Path = ".") -> None:
        super().__init__()
        self._validator = PathValidator(workspace_dir)

    async def execute(self, input_data: StrReplaceEditorInput) -> ToolResult:
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
            return ToolResult(
                tool_call_id="",
                output=f"No match found for string in {input_data.path}",
                is_error=True,
            )

        if count > 1 and not input_data.replace_all:
            return ToolResult(
                tool_call_id="",
                output=f"Found {count} matches. Use replace_all=True or provide a more specific match.",
                is_error=True,
            )

        # Apply replacement
        if input_data.replace_all:
            modified = content.replace(input_data.old_string, input_data.new_string)
        else:
            modified = content.replace(input_data.old_string, input_data.new_string, 1)

        # Generate diff
        diff = DiffGenerator.generate_diff(content, modified, str(input_data.path))

        # Atomic write
        try:
            tmp = path.with_suffix(f"{path.suffix}.tmp")
            tmp.write_text(modified, encoding="utf-8")
            tmp.replace(path)
        except Exception as e:
            return ToolResult(tool_call_id="", output=f"Write failed: {e}", is_error=True)

        return ToolResult(
            tool_call_id="",
            output=f"Applied edit to {input_data.path} ({count} occurrence(s))\n\n{diff}",
        )


# ── 工具组注册 ─────────────────────────────────────────


def register_file_tools(registry, workspace_dir: str | Path = ".") -> None:
    """注册所有文件系统工具到 registry。"""
    registry.register(ReadFileTool(workspace_dir))
    registry.register(WriteToFileTool(workspace_dir))
    registry.register(StrReplaceEditorTool(workspace_dir))


# ── 工具函数 ───────────────────────────────────────────


def _is_binary(path: Path) -> bool:
    """通过读取前 512 字节检查是否为二进制文件。"""
    try:
        with open(path, "rb") as f:
            chunk = f.read(512)
        return b"\0" in chunk
    except Exception:
        return True
