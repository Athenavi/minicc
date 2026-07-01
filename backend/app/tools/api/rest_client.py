"""API 集成工具 — REST/GraphQL 客户端。"""

from __future__ import annotations

from typing import Any, Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class ApiRequestInput(BaseModel):
    url: str = Field(description="Request URL")
    method: str = Field(default="GET", description="HTTP method: GET/POST/PUT/DELETE/PATCH")
    headers: Optional[dict[str, str]] = Field(default=None)
    body: Optional[Any] = Field(default=None, description="Request body (dict for JSON)")
    timeout: int = Field(default=30, ge=1, le=120)


class ApiRequestTool(BaseTool):
    name = "api_request"
    description = "Make an HTTP request to any REST API endpoint."
    input_schema = ApiRequestInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.WEB

    async def execute(self, input_data: ApiRequestInput, context: ToolUseContext | None = None) -> ToolResult:
        import httpx, json
        try:
            async with httpx.AsyncClient(timeout=input_data.timeout) as client:
                resp = await client.request(
                    method=input_data.method,
                    url=input_data.url,
                    headers=input_data.headers,
                    json=input_data.body if isinstance(input_data.body, dict) else None,
                    content=input_data.body if isinstance(input_data.body, str) else None,
                )
            try:
                data = resp.json()
                output = json.dumps(data, indent=2, ensure_ascii=False)[:10000]
            except Exception:
                output = resp.text[:10000]
            return ToolResult(
                tool_call_id="",
                output=f"[api] {resp.status_code} {input_data.method} {input_data.url}\n{output}",
                metadata={"status": resp.status_code},
            )
        except Exception as exc:
            return ToolResult(tool_call_id="", output=f"[api] Error: {exc}", is_error=True)


def register_api_tools(registry) -> None:
    registry.register(ApiRequestTool())
