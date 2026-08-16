"""Tools API endpoints. """
from __future__ import annotations

from typing import Any

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

from app.tools.registry import registry

router = APIRouter(tags=["tools"])


@router.get("/v1/tools")
async def list_tools() -> dict[str, Any]:
    tools = registry.to_openai_tools()
    return {"tools": tools}


class ToolExecuteRequest(BaseModel):
    name: str
    input: dict[str, Any] = {}


@router.post("/v1/tools/execute")
async def execute_tool(body: ToolExecuteRequest) -> dict[str, Any]:
    result = await registry.execute(body.name, body.input)
    if isinstance(result, dict) and "error" in result and len(result) == 1:
        raise HTTPException(status_code=404, detail=result["error"])
    return result
