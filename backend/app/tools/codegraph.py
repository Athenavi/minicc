"""CodeGraph — 代码理解与文件引用系统。

参照 Reasonix fileref/search.go 设计：
- 文件路径搜索（子串匹配，分层优先级）
- 跳过 node_modules/.git/build 等目录
- 与 LSP 集成获取符号信息
"""

from __future__ import annotations

import os
import re
from pathlib import Path
from typing import Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext

SKIP_DIRS = {
    ".git", "node_modules", "__pycache__", ".venv", "venv",
    ".next", "dist", "build", ".astro", ".wailsjs",
}
SKIP_EXTENSIONS = {".pyc", ".pyo", ".so", ".dll", ".dylib", ".exe", ".bin"}
MAX_WALK = 5000


class FileRefSearchInput(BaseModel):
    query: str = Field(description="Search query (filename or path segment)")
    root: str = Field(default=".", description="Search root directory")
    max_results: int = Field(default=20, ge=1, le=100)


class CodeSearchInput(BaseModel):
    pattern: str = Field(description="Regex pattern to search in file contents")
    include: Optional[str] = Field(default=None, description="File glob filter (e.g. '*.py')")
    root: str = Field(default=".", description="Search root")
    max_results: int = Field(default=20, ge=1, le=100)


class CodeGraphSearchTool(BaseTool):
    """搜索代码库中的文件和符号。结合了文件路径搜索和内容搜索。"""

    name = "code_search"
    description = "Search the codebase for files and symbols."
    input_schema = FileRefSearchInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SEARCH

    def get_prompt(self) -> str | None:
        return (
            "Search the codebase for files and symbols.\n\n"
            "Usage:\n"
            "- Use file_ref_search to find files by path/name\n"
            "- Use code_search to search file contents by regex\n"
            "- Always search before reading — find the right file first\n"
            "- Skips .git, node_modules, __pycache__"
        )

    async def execute(self, input_data: FileRefSearchInput | CodeSearchInput, context: ToolUseContext | None = None) -> ToolResult:
        if isinstance(input_data, CodeSearchInput):
            return await self._content_search(input_data)
        return await self._file_search(input_data)

    async def _file_search(self, input_data: FileRefSearchInput) -> ToolResult:
        """文件路径搜索（参照 Reasonix fileref.Search）。"""
        root = Path(input_data.root).resolve()
        if not root.exists():
            return ToolResult(tool_call_id="", output=f"Path not found: {input_data.root}", is_error=True)

        query = input_data.query.lower()
        results: list[tuple[int, str]] = []  # (priority, path)

        for i, entry in enumerate(root.rglob("*")):
            if i > MAX_WALK:
                break
            # Skip ignored dirs
            if any(p in SKIP_DIRS for p in entry.parts):
                continue

            rel = str(entry.relative_to(root))
            name = entry.name.lower()

            # Priority 0: exact filename match
            if query == name:
                results.append((0, rel))
            # Priority 1: query in basename
            elif query in name:
                results.append((1, rel))
            # Priority 2: query in path segment
            elif query in rel.lower():
                results.append((2, rel))

            if len(results) >= input_data.max_results:
                break

        if not results:
            return ToolResult(tool_call_id="", output=f"No files found matching '{input_data.query}'")

        results.sort(key=lambda x: (x[0], x[1]))
        lines = [f"Found {len(results)} file(s):\n"]
        for pri, path in results:
            prefix = "📁" if (root / path).is_dir() else "📄"
            lines.append(f"  {prefix} {path}")
        return ToolResult(tool_call_id="", output="\n".join(lines))

    async def _content_search(self, input_data: CodeSearchInput) -> ToolResult:
        """文件内容正则搜索（类似 grep）。"""
        root = Path(input_data.root).resolve()
        if not root.exists():
            return ToolResult(tool_call_id="", output=f"Path not found: {input_data.root}", is_error=True)

        try:
            regex = re.compile(input_data.pattern)
        except re.error as e:
            return ToolResult(tool_call_id="", output=f"Invalid regex: {e}", is_error=True)

        results: list[str] = []
        for entry in root.rglob("*"):
            if entry.is_file() and not any(p in SKIP_DIRS for p in entry.parts):
                if input_data.include and not entry.name.endswith(tuple(input_data.include.split(","))):
                    continue
                if entry.suffix in SKIP_EXTENSIONS:
                    continue
                try:
                    for i, line in enumerate(entry.read_text(encoding="utf-8", errors="replace").splitlines(), 1):
                        if regex.search(line):
                            rel = str(entry.relative_to(root))
                            results.append(f"{rel}:{i}: {line.strip()[:150]}")
                            if len(results) >= input_data.max_results:
                                break
                    if len(results) >= input_data.max_results:
                        break
                except Exception:
                    continue

        if not results:
            return ToolResult(tool_call_id="", output=f"No matches for '{input_data.pattern}'")
        return ToolResult(tool_call_id="", output=f"Found {len(results)} match(es):\n" + "\n".join(results))


# Register function
def register_codegraph_tools(registry, workspace_dir: str = ".") -> None:
    """注册 CodeGraph 工具。"""
    registry.register(CodeGraphSearchTool())
