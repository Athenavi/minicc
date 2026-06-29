"""网络工具 — WebFetch 网页抓取。

对应 Claude Code 的 WebFetchTool。
"""

from __future__ import annotations

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class WebFetchInput(BaseModel):
    url: str = Field(description="URL to fetch")
    max_length: int = Field(default=5000, ge=100, le=50000, description="Max characters to return")


class WebFetchTool(BaseTool):
    """获取 URL 内容，将 HTML 转为纯文本。"""

    name = "web_fetch"
    description = "Fetch a URL and return its text content. Use to read documentation, APIs, or web pages."
    input_schema = WebFetchInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.WEB

    def get_prompt(self) -> str | None:
        return (
            "Fetch a URL and return readable text content.\n\n"
            "Usage:\n"
            "- HTML pages are converted to plain text\n"
            "- Max 50,000 characters returned\n"
            "- Only use for programming-related URLs\n"
            "- NEVER guess or generate URLs"
        )

    async def execute(self, input_data: WebFetchInput, context: ToolUseContext | None = None) -> ToolResult:
        import httpx
        try:
            async with httpx.AsyncClient(timeout=15, follow_redirects=True) as client:
                resp = await client.get(input_data.url)
                resp.raise_for_status()

                content_type = resp.headers.get("content-type", "")
                text = resp.text

                # Simple HTML-to-text for HTML responses
                if "html" in content_type.lower():
                    import re
                    text = re.sub(r"<script[^>]*>.*?</script>", "", text, flags=re.DOTALL)
                    text = re.sub(r"<style[^>]*>.*?</style>", "", text, flags=re.DOTALL)
                    text = re.sub(r"<[^>]+>", " ", text)
                    text = re.sub(r"\s+", " ", text).strip()

                if len(text) > input_data.max_length:
                    text = text[:input_data.max_length] + "\n...(truncated)"

                return ToolResult(tool_call_id="", output=text)

        except Exception as exc:
            return ToolResult(tool_call_id="", output=f"Fetch failed: {exc}", is_error=True)
