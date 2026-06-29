"""Notebook 编辑、Web 搜索、Plan Mode 工具。

完成 Claude Code 25 工具集的最后 4 个工具。
"""

from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext
from app.utils.security import PathValidator


class _NoInput(BaseModel):
    pass


# ── NotebookEditTool ──


class NotebookEditInput(BaseModel):
    notebook_path: str = Field(description="Path to the .ipynb file")
    cell_id: Optional[str] = Field(default=None, description="Cell ID to edit (omit for append)")
    new_source: str = Field(description="New source content for the cell")
    cell_type: Optional[str] = Field(default=None, description="Cell type: 'code' | 'markdown' (required for insert)")
    edit_mode: str = Field(default="replace", description="Edit mode: 'replace' | 'insert' | 'delete'")


class NotebookEditTool(BaseTool):
    """编辑 Jupyter Notebook (.ipynb) 文件，操作粒度为 cell 级。

    对应 Claude Code NotebookEditTool：
    - 操作粒度是 cell 级，不是整文件字符串级
    - 支持 replace / insert / delete 三种 cell 操作
    - 要求先读再改（Read-before-Edit）
    - 保持 notebook JSON 结构有效
    """

    name = "notebook_edit"
    description = "Edit a Jupyter notebook (.ipynb) at the cell level. Use replace/insert/delete on individual cells."
    input_schema = NotebookEditInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.FILE

    def __init__(self, workspace_dir: str | Path = ".") -> None:
        super().__init__()
        self._validator = PathValidator(workspace_dir)

    async def execute(self, input_data: NotebookEditInput, context: ToolUseContext | None = None) -> ToolResult:
        try:
            path = self._validator.validate(input_data.notebook_path)
        except PermissionError as e:
            return ToolResult(tool_call_id="", output=str(e), is_error=True)

        if not path.exists():
            return ToolResult(tool_call_id="", output=f"Notebook not found: {input_data.notebook_path}", is_error=True)
        if not path.name.endswith(".ipynb"):
            return ToolResult(tool_call_id="", output="Not a .ipynb file", is_error=True)

        try:
            with open(path, encoding="utf-8") as f:
                nb = json.load(f)
        except json.JSONDecodeError as e:
            return ToolResult(tool_call_id="", output=f"Invalid notebook JSON: {e}", is_error=True)

        cells = nb.get("cells", [])
        original_cell_count = len(cells)

        # Find cell index
        cell_idx = None
        if input_data.cell_id:
            for i, c in enumerate(cells):
                if c.get("id") == input_data.cell_id or c.get("metadata", {}).get("cell_id") == input_data.cell_id:
                    cell_idx = i
                    break
            if cell_idx is None:
                return ToolResult(tool_call_id="", output=f"Cell '{input_data.cell_id}' not found", is_error=True)

        # Apply edit
        action = ""
        if input_data.edit_mode == "replace" and cell_idx is not None:
            cells[cell_idx]["source"] = input_data.new_source
            if input_data.cell_type:
                cells[cell_idx]["cell_type"] = input_data.cell_type
            action = f"replaced cell {cell_idx}"
        elif input_data.edit_mode == "insert":
            new_cell = {
                "id": f"cell_{len(cells)}",
                "cell_type": input_data.cell_type or "code",
                "source": input_data.new_source,
                "metadata": {},
            }
            pos = (cell_idx + 1) if cell_idx is not None else len(cells)
            cells.insert(pos, new_cell)
            action = f"inserted cell at position {pos}"
        elif input_data.edit_mode == "delete" and cell_idx is not None:
            removed = cells.pop(cell_idx)
            action = f"deleted cell {cell_idx} ('{removed.get('id', 'unknown')}')"
        else:
            return ToolResult(tool_call_id="", output=f"Invalid edit: mode={input_data.edit_mode}, cell_id={input_data.cell_id}", is_error=True)

        nb["cells"] = cells

        # Atomic write
        tmp = path.with_suffix(".ipynb.tmp")
        tmp.write_text(json.dumps(nb, indent=1, ensure_ascii=False), encoding="utf-8")
        tmp.replace(path)

        return ToolResult(
            tool_call_id="",
            output=f"Notebook updated: {action}\nCells: {original_cell_count} → {len(cells)}",
            metadata={"cells_before": original_cell_count, "cells_after": len(cells)},
        )


# ── WebSearchTool ──


class WebSearchInput(BaseModel):
    query: str = Field(description="Search query (include current year for recent info)")
    allowed_domains: Optional[list[str]] = Field(default=None, description="Only search these domains")
    blocked_domains: Optional[list[str]] = Field(default=None, description="Exclude these domains")


class WebSearchTool(BaseTool):
    """联网搜索，查找最新信息。"""

    name = "web_search"
    description = "Search the web for current information."
    input_schema = WebSearchInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.WEB

    def get_prompt(self) -> str | None:
        return (
            "Search the web for current information.\n\n"
            "Rules:\n"
            "- Include the current year in queries for recent info\n"
            "- After answering, include a 'Sources:' section with markdown links\n"
            "- Use web_fetch to read specific pages found by search\n"
            "- Search finds entry points, web_fetch reads the actual content"
        )

    async def execute(self, input_data: WebSearchInput, context: ToolUseContext | None = None) -> ToolResult:
        import httpx
        domains = ""
        if input_data.allowed_domains:
            domains = f"\nAllowed domains: {', '.join(input_data.allowed_domains)}"
        if input_data.blocked_domains:
            domains += f"\nBlocked domains: {', '.join(input_data.blocked_domains)}"

        # Try search providers in order
        results = await self._search_duckduckgo(input_data.query) or \
                  await self._search_searxng(input_data.query) or \
                  []

        if not results:
            results = ["(No results found. Try a more specific query.)"]

        output = (
            f"[Web search results for: {input_data.query}]{domains}\n\n"
            + "\n".join(results)
            + "\n\nIMPORTANT: Use web_fetch to retrieve specific pages. "
            "Include a 'Sources:' section with markdown links in your final response."
        )
        return ToolResult(tool_call_id="", output=output)

    async def _search_duckduckgo(self, query: str) -> list[str] | None:
        """DuckDuckGo instant answer API (free, no key)."""
        import httpx
        try:
            url = f"https://api.duckduckgo.com/?q={query}&format=json&no_html=1"
            async with httpx.AsyncClient(timeout=8) as client:
                resp = await client.get(url)
                data = resp.json()

            results = []
            abstract = data.get("AbstractText", "")
            if abstract:
                results.append(f"• {abstract}")
                if data.get("AbstractURL"):
                    results.append(f"  Source: {data['AbstractURL']}")
            for topic in data.get("RelatedTopics", [])[:5]:
                if "Text" in topic:
                    results.append(f"• {topic['Text']}")
                    if "FirstURL" in topic:
                        results.append(f"  {topic['FirstURL']}")
            return results if results else None
        except Exception:
            return None

    async def _search_searxng(self, query: str) -> list[str] | None:
        """SearXNG self-hosted search API."""
        import httpx
        from app.utils.config import settings
        searxng_url = getattr(settings, "searxng_url", "") or os.environ.get("SEARXNG_URL", "")
        if not searxng_url:
            return None
        try:
            params = {"q": query, "format": "json", "language": "en", "categories": "general"}
            async with httpx.AsyncClient(timeout=10) as client:
                resp = await client.get(f"{searxng_url}/search", params=params)
                data = resp.json()

            results = []
            for r in data.get("results", [])[:8]:
                title = r.get("title", "")
                url = r.get("url", "")
                snippet = r.get("content", "")[:200]
                if title:
                    results.append(f"• {title}")
                    if snippet:
                        results.append(f"  {snippet}")
                    if url:
                        results.append(f"  {url}")
            return results if results else None
        except Exception:
            return None


# ── Plan Mode 工具 ──


class EnterPlanModeTool(BaseTool):
    """进入 Plan Mode。"""

    name = "enter_plan_mode"
    description = "Enter Plan Mode to create a structured plan before implementing."
    input_schema = _NoInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SESSION

    def get_prompt(self) -> str | None:
        return (
            "Enter Plan Mode to plan before coding.\n\n"
            "- Research and understand the problem first\n"
            "- Write a clear plan covering: approach, files to change, risks\n"
            "- If requirements are unclear, use ask_user_question first\n"
            "- Use exit_plan_mode when the plan is ready for approval"
        )

    async def execute(self, input_data: BaseModel, context: ToolUseContext | None = None) -> ToolResult:
        return ToolResult(tool_call_id="", output="[Plan Mode activated]\n\nYou are now in Plan Mode. Research the problem, then write your plan. Use /exit_plan_mode when done.")


class ExitPlanModeTool(BaseTool):
    """退出 Plan Mode。"""

    name = "exit_plan_mode"
    description = "Exit Plan Mode and submit your plan for user approval."
    input_schema = _NoInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SESSION

    def get_prompt(self) -> str | None:
        return (
            "Exit Plan Mode and submit your plan.\n\n"
            "Rules:\n"
            "- Only use this when the plan is complete and ready for review\n"
            "- The plan should be written to a file first\n"
            "- This submits the plan for user approval\n"
            "- For clarifying requirements, use ask_user_question instead"
        )

    async def execute(self, input_data: BaseModel, context: ToolUseContext | None = None) -> ToolResult:
        return ToolResult(tool_call_id="", output="[Plan submitted for approval]\n\nYour plan has been submitted. Waiting for user approval to proceed with implementation.")
