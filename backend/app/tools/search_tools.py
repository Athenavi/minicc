"""搜索工具 — Glob 文件查找、Grep 内容搜索。

对应 Claude Code 的 GlobTool / GrepTool。
"""

from __future__ import annotations

import fnmatch
import os
import re
from pathlib import Path
from typing import Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class GlobInput(BaseModel):
    pattern: str = Field(description="Glob pattern (e.g. '**/*.py', 'src/**/*.ts')")
    path: str = Field(default=".", description="Search root directory")


class GlobTool(BaseTool):
    """按 glob 模式查找文件。"""

    name = "glob"
    description = "Find files matching a glob pattern. Use for locating files by name or extension."
    input_schema = GlobInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SEARCH

    def get_prompt(self) -> str | None:
        return (
            "Find files matching a glob pattern.\n\n"
            "Usage:\n"
            "- Supports ** for recursive matching\n"
            "- Examples: '**/*.py', 'src/**/*.ts', '*.md'\n"
            "- Returns up to 100 matches\n"
            "- Use before read_file to locate the right file"
        )

    async def execute(self, input_data: GlobInput, context: ToolUseContext | None = None) -> ToolResult:
        root = Path(input_data.path).resolve()
        if not root.exists():
            return ToolResult(tool_call_id="", output=f"Path not found: {input_data.path}", is_error=True)

        matches = []
        for file in root.rglob("*") if "**" in input_data.pattern else root.glob(input_data.pattern):
            if file.is_file():
                try:
                    rel = file.relative_to(root)
                    matches.append(str(rel))
                except ValueError:
                    matches.append(str(file))
                if len(matches) >= 100:
                    break

        if not matches:
            return ToolResult(tool_call_id="", output=f"No files matching '{input_data.pattern}'")

        output = f"Found {len(matches)} file(s):\n" + "\n".join(sorted(matches))
        return ToolResult(tool_call_id="", output=output)


class GrepInput(BaseModel):
    pattern: str = Field(description="Regular expression to search for")
    path: str = Field(default=".", description="Search root directory")
    include: Optional[str] = Field(default=None, description="File glob to include (e.g. '*.py')")
    max_results: int = Field(default=50, ge=1, le=200, description="Maximum results")


class GrepTool(BaseTool):
    """在文件内容中搜索正则表达式匹配。"""

    name = "grep"
    description = "Search file contents for a regex pattern. Use for finding usages, definitions, or specific code."
    input_schema = GrepInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SEARCH

    def get_prompt(self) -> str | None:
        return (
            "Search file contents for a regex pattern.\n\n"
            "Usage:\n"
            "- Supports Python regex syntax\n"
            "- Use include to limit to specific file types\n"
            "- Max 200 results to avoid flooding context\n"
            "- Use before read_file to find what to read"
        )

    async def execute(self, input_data: GrepInput, context: ToolUseContext | None = None) -> ToolResult:
        root = Path(input_data.path).resolve()
        if not root.exists():
            return ToolResult(tool_call_id="", output=f"Path not found: {input_data.path}", is_error=True)

        try:
            regex = re.compile(input_data.pattern)
        except re.error as e:
            return ToolResult(tool_call_id="", output=f"Invalid regex: {e}", is_error=True)

        results = []
        for file in root.rglob("*"):
            if file.is_file():
                # Apply include filter
                if input_data.include and not fnmatch.fnmatch(file.name, input_data.include):
                    continue
                # Skip binary files and hidden dirs
                if any(p.startswith(".") for p in file.relative_to(root).parts):
                    continue
                try:
                    for i, line in enumerate(file.read_text(encoding="utf-8", errors="replace").splitlines(), 1):
                        if regex.search(line):
                            results.append(f"{file.relative_to(root)}:{i}:{line.strip()[:200]}")
                            if len(results) >= input_data.max_results:
                                break
                except Exception:
                    continue
            if len(results) >= input_data.max_results:
                break

        if not results:
            return ToolResult(tool_call_id="", output=f"No matches for '{input_data.pattern}'")

        output = f"Found {len(results)} match(es):\n" + "\n".join(results)
        return ToolResult(tool_call_id="", output=output)
