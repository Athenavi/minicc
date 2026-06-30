"""BaseTool 抽象基类与 ToolRegistry 注册中心。

对应 Claude Code 的 Tool.ts / tools.ts 设计：
- Tool.ts 定义统一协议（输入 schema、执行上下文、权限契约）
- tools.ts 是系统的工具目录（动态筛选、分组、条件暴露）

设计原则：
1. 工具不是命令快捷方式，而是系统级能力对象
2. 工具执行在完整运行时上下文里（ToolUseContext）
3. 权限前置：执行前、暴露阶段都受约束
4. 工具集是可配置、可裁剪的
"""

from __future__ import annotations

import enum
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Any, Callable, Optional

from pydantic import BaseModel

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult


# ── 工具分类 ────────────────────────────────────────────


class ToolCategory(str, enum.Enum):
    """工具分类（对应 Claude Code 的 4 组工具）。"""
    FILE = "file"  # 文件读写编辑
    SHELL = "shell"  # 终端命令
    SEARCH = "search"  # 搜索查找
    WEB = "web"  # 网络访问
    SESSION = "session"  # 会话控制（ask, todo, plan mode）
    EXTENSION = "extension"  # 扩展接入（MCP, LSP）
    AGENT = "agent"  # 协作与任务


# ── 运行时上下文 ──
#
# 对应 Claude Code 的 ToolUseContext：
# 工具不是在真空中执行，而是被放进完整会话运行时里的。


@dataclass
class ToolUseContext:
    """工具执行时的完整运行时上下文。

    对应 Claude Code 的 ToolUseContext 类型。
    """
    options: dict[str, Any] = field(default_factory=lambda: {
        "debug": False,
        "verbose": False,
    })
    abort_event: Any = None  # asyncio.Event — 用于中断感知
    messages: list[Any] = field(default_factory=list)  # 当前消息历史
    file_cache: dict[str, Any] = field(default_factory=dict)  # 文件读取缓存
    get_app_state: Optional[Callable] = None
    set_app_state: Optional[Callable] = None


@dataclass
class ToolPermissionContext:
    """工具权限上下文 — 在执行前过滤工具可见性。

    对应 Claude Code 的 filterToolsByDenyRules()。
    """
    disabled_tools: set[str] = field(default_factory=set)
    enabled_categories: set[str] = field(default_factory=lambda: {c.value for c in ToolCategory})
    platform: str = ""


# ── BaseTool ────────────────────────────────────────────


class BaseTool(ABC):
    """所有工具的抽象基类。

    对应 Claude Code 的 Tool 协议：
    - 明确输入 schema（ToolInputJSONSchema）
    - 统一上下文（ToolUseContext）
    - 可被权限系统约束（ToolPermissionContext）
    - 可反馈进度和状态
    """

    name: str = ""
    description: str = ""
    input_schema: type[BaseModel] = BaseModel
    permission_level: PermissionLevel = PermissionLevel.READ
    category: ToolCategory = ToolCategory.FILE
    # 是否需要用户确认（用于 AskUserQuestion 等交互工具）
    interactive: bool = False

    def get_prompt(self) -> str | None:
        """返回工具级 prompt（指导模型如何正确使用此工具）。

        对应 Claude Code 工具自带的 prompt 设计：
        - 不只是在 schema 里写 description
        - 而是用自然语言告诉模型"什么时候用、怎么用、不要怎么用"
        """
        return None

    @abstractmethod
    async def execute(self, input_data: BaseModel, context: ToolUseContext | None = None) -> ToolResult:
        """执行工具调用。

        context 参数携带完整的运行时上下文：
        - abort_event: 中断感知
        - messages: 当前消息历史
        - options: 运行时选项
        """
        ...

    def to_anthropic_tool(self) -> dict:
        """序列化为 Anthropic tool 格式。"""
        return {
            "name": self.name,
            "description": self.description,
            "input_schema": self.input_schema.model_json_schema(),
        }

    def to_openai_tool(self) -> dict:
        """序列化为 OpenAI tool 格式。"""
        return {
            "type": "function",
            "function": {
                "name": self.name,
                "description": self.description,
                "parameters": self.input_schema.model_json_schema(),
            },
        }


# ── 动态工具过滤 ──
#
# 对应 Claude Code 的 filterToolsByDenyRules()：
# 有些工具会在模型看到它们之前，就先被权限规则过滤掉。


def filter_tools_by_context(
    tools: list[BaseTool],
    context: ToolPermissionContext,
) -> list[BaseTool]:
    """根据权限上下文过滤工具。

    过滤顺序：
    1. 禁用名单中的工具
    2. 未启用的分类
    3. 平台限制
    """
    return [
        t for t in tools
        if t.name not in context.disabled_tools
        and t.category.value in context.enabled_categories
    ]


def filter_tools_for_llm(
    tools: list[BaseTool],
    permission_context: ToolPermissionContext | None = None,
    max_tools: int = 60,
) -> list[BaseTool]:
    """过滤并限制工具数量（最终暴露给模型的工具集）。"""
    if permission_context:
        tools = filter_tools_by_context(tools, permission_context)

    # 按分类排序以便模型理解
    category_order = [c.value for c in ToolCategory]
    tools.sort(key=lambda t: (category_order.index(t.category.value) if t.category.value in category_order else 99, t.name))

    # 限制数量（防止工具列表撑爆上下文）
    if len(tools) > max_tools:
        tools = tools[:max_tools]

    return tools


# ── ToolRegistry ──


class ToolRegistry:
    """工具注册中心。支持注册、查找、序列化、动态过滤。

    对应 Claude Code 的 tools.ts：
    - getAlBaseTools() / getToolsForLLM()
    - 工具集不是死的，受环境和 feature 条件影响
    """

    def __init__(self):
        self._tools: dict[str, BaseTool] = {}

    def register(self, tool: BaseTool) -> None:
        if not tool.name:
            raise ValueError(f"Tool must have a name: {type(tool).__name__}")
        self._tools[tool.name] = tool

    def get(self, name: str) -> BaseTool | None:
        return self._tools.get(name)

    def list_tools(self, permission_context: ToolPermissionContext | None = None) -> list[BaseTool]:
        """列出工具，可选权限过滤。"""
        tools = list(self._tools.values())
        if permission_context:
            tools = filter_tools_by_context(tools, permission_context)
        return tools

    def get_by_category(self, category: ToolCategory) -> list[BaseTool]:
        """按分类获取工具。"""
        return [t for t in self._tools.values() if t.category == category]

    def to_anthropic_tools(self, permission_context: ToolPermissionContext | None = None) -> list[dict]:
        """序列化为 Anthropic tool 格式（可选权限过滤）。
        ⚡ 按 name 排序确保确定性输出，稳定 DeepSeek prefix cache。"""
        tools = filter_tools_for_llm(self.list_tools(permission_context))
        result = [t.to_anthropic_tool() for t in tools]
        result.sort(key=lambda x: x["name"])  # 确定性排序
        return result

    def to_openai_tools(self, permission_context: ToolPermissionContext | None = None) -> list[dict]:
        """序列化为 OpenAI tool 格式（可选权限过滤）。
        ⚡ 按 name 排序确保确定性输出，稳定 DeepSeek prefix cache。"""
        tools = filter_tools_for_llm(self.list_tools(permission_context))
        result = [t.to_openai_tool() for t in tools]
        result.sort(key=lambda x: x.get("function", {}).get("name", x.get("name", "")))
        return result

    def register_file_tools(self, workspace_dir: str = ".") -> None:
        """注册文件系统工具组。"""
        from app.tools.file_system import ReadFileTool, WriteToFileTool, StrReplaceEditorTool
        self.register(ReadFileTool(workspace_dir))
        self.register(WriteToFileTool(workspace_dir))
        self.register(StrReplaceEditorTool(workspace_dir))

    def register_shell_tool(self, workspace_dir: str = ".") -> None:
        """注册 Shell 工具。"""
        from app.tools.shell_executor import ShellExecutorTool
        self.register(ShellExecutorTool(workspace_dir))

    def get_tool_summary(self) -> list[dict]:
        """获取工具摘要（用于调试/审计）。"""
        return [
            {"name": t.name, "category": t.category.value, "level": t.permission_level.value}
            for t in self._tools.values()
        ]
