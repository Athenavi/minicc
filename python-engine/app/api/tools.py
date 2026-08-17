"""Tools API endpoints. """
from __future__ import annotations

from typing import Any

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

from app.tools.registry import registry

router = APIRouter(tags=["tools"])


@router.get("/v1/tools")
async def list_tools() -> dict[str, Any]:
    # 按工具上下文用户过滤（MCP 等用户级工具只对自己可见）
    tools = registry.to_openai_tools()
    return {"tools": tools}


class ToolExecuteRequest(BaseModel):
    name: str
    input: dict[str, Any] = {}
    user_id: str = ""


@router.post("/v1/tools/execute")
async def execute_tool(body: ToolExecuteRequest) -> dict[str, Any]:
    # Go 网关转发时携带 user_id；设置工具沙箱上下文（用户级隔离）
    if body.user_id:
        from app.tools.context import set_tool_context
        set_tool_context(user_id=body.user_id, tenant_id=body.user_id)
        from app.main import touch_user
        touch_user(body.user_id)
    result = await registry.execute(body.name, body.input, user_id=body.user_id)
    if isinstance(result, dict) and "error" in result and len(result) == 1:
        raise HTTPException(status_code=404, detail=result["error"])
    return result
