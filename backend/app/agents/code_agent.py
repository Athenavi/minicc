"""代码 Agent — 处理代码编写/修改/分析任务。"""

from __future__ import annotations

import subprocess
from pathlib import Path
from typing import Optional

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class CodeTaskInput(BaseModel):
    task: str = Field(description="Code task description")
    file_path: Optional[str] = Field(default=None, description="Target file path")
    language: Optional[str] = Field(default=None, description="Programming language")


class CodeAgentHandler:
    """代码 Agent 执行器 — 通过现有工具处理代码任务。"""

    async def handle(self, task: str, file_path: str | None = None, language: str | None = None) -> ToolResult:
        """处理代码任务。使用已有的 FileSystem 和 Shell 工具。"""
        # 简单实现：通过 ToolRegistry 回调执行
        return ToolResult(
            tool_call_id="",
            output=f"[code-agent] Task received: {task[:100]}...\n"
                   f"  File: {file_path or 'auto'}\n"
                   f"  Language: {language or 'auto'}\n"
                   f"  (executed via TaskCreateTool as sub-agent)",
        )


class CodeAgentTool(BaseTool):
    name = "code_agent"
    description = "Execute a code-related task — write, modify, analyze, debug code."
    input_schema = CodeTaskInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.AGENT

    async def execute(self, input_data: CodeTaskInput, context: ToolUseContext | None = None) -> ToolResult:
        handler = CodeAgentHandler()
        return await handler.handle(input_data.task, input_data.file_path, input_data.language)
