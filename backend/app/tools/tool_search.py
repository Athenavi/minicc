"""工具搜索 — 帮助模型在大量工具中找到合适的工具。

对应 Claude Code 的 ToolSearchTool。
当工具数量很多时，模型可以先用此工具搜索可用工具。
"""

from __future__ import annotations

from pydantic import BaseModel, Field

from app.models.tool import ToolResult
from app.models.permission import PermissionLevel
from app.tools.base import BaseTool, ToolCategory, ToolRegistry, ToolUseContext


class ToolSearchInput(BaseModel):
    query: str = Field(description="Search query to find relevant tools")
    category: str | None = Field(default=None, description="Optional category filter")


class ToolSearchTool(BaseTool):
    """在可用工具中搜索。当你不知道用哪个工具时，先搜索。"""

    name = "tool_search"
    description = "Search through available tools to find the right one for your task."
    input_schema = ToolSearchInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SESSION

    def __init__(self, registry: ToolRegistry) -> None:
        super().__init__()
        self._registry = registry

    async def execute(self, input_data: ToolSearchInput, context: ToolUseContext | None = None) -> ToolResult:
        query = input_data.query.lower()
        tools = self._registry.list_tools()

        # Filter by category
        if input_data.category:
            tools = [t for t in tools if t.category.value == input_data.category]

        # Simple keyword matching
        results = []
        for t in tools:
            score = 0
            if query in t.name.lower():
                score += 3
            if query in t.description.lower():
                score += 2
            if t.get_prompt() and query in t.get_prompt().lower():
                score += 1
            if score > 0:
                results.append((score, t))

        results.sort(key=lambda x: -x[0])

        if not results:
            return ToolResult(tool_call_id="", output=f"No tools found matching '{input_data.query}'")

        lines = [f"Found {len(results)} matching tool(s):\n"]
        for score, t in results[:10]:
            prompt_snippet = ""
            if t.get_prompt():
                prompt_snippet = " — " + t.get_prompt().split("\n")[0][:80]
            lines.append(f"- {t.name} [{t.category.value}] {t.description[:100]}{prompt_snippet}")

        return ToolResult(tool_call_id="", output="\n".join(lines))
