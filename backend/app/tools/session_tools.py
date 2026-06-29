"""会话控制工具 — 用户交互、待办、Plan Mode。

对应 Claude Code 的会话控制工具组：
- AskUserQuestionTool: 向用户提问
- TodoWriteTool: 管理待办清单
"""

from __future__ import annotations

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class AskUserQuestionInput(BaseModel):
    question: str = Field(description="The question to ask the user")
    options: list[str] | None = Field(default=None, description="Optional multiple-choice options")


class AskUserQuestionTool(BaseTool):
    """向用户提问，等待用户响应。"""

    name = "ask_user_question"
    description = "Ask the user a question and wait for their response. Use when you need clarification or a decision."
    input_schema = AskUserQuestionInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SESSION
    interactive = True

    async def execute(self, input_data: AskUserQuestionInput, context: ToolUseContext | None = None) -> ToolResult:
        output = f"Question: {input_data.question}"
        if input_data.options:
            output += f"\nOptions: {', '.join(input_data.options)}"
        output += "\n(Waiting for user response...)"
        return ToolResult(tool_call_id="", output=output)


class TodoWriteInput(BaseModel):
    todo: str = Field(description="The task to add or update")
    status: str | None = Field(default=None, description="Task status: 'pending' | 'completed' | 'in_progress'")


class TodoWriteTool(BaseTool):
    """记录待办事项，跟踪任务进度。"""

    name = "todo_write"
    description = "Record a todo item or update task progress. Use to track what remains to be done."
    input_schema = TodoWriteInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SESSION

    async def execute(self, input_data: TodoWriteInput, context: ToolUseContext | None = None) -> ToolResult:
        status = input_data.status or "pending"
        return ToolResult(
            tool_call_id="",
            output=f"Todo recorded: [{status}] {input_data.todo}",
        )
