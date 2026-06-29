"""核心命令集 — /help, /status, /tools, /clear, /config。

对应 Claude Code 的 commands/ 目录结构。
"""

from __future__ import annotations

import os
import platform
from typing import Any

from app.commands import Command
from app.tools.base import ToolRegistry


class HelpCommand(Command):
    name = "/help"
    description = "Show available slash commands"
    aliases = ["/h"]

    def __init__(self, dispatcher) -> None:
        self._dispatcher = dispatcher

    async def execute(self, args: str, context: dict[str, Any]) -> str:
        cmds = self._dispatcher.list_commands()
        lines = ["## Available Commands\n"]
        for cmd in sorted(cmds, key=lambda c: c.name):
            lines.append(f"  {cmd.name:<20} {cmd.description}")
        lines.append(f"\n  /help                Show this help")
        lines.append(f"\nTotal: {len(cmds)} command(s)")
        return "\n".join(lines)


class StatusCommand(Command):
    name = "/status"
    description = "Show current session status"

    async def execute(self, args: str, context: dict[str, Any]) -> str:
        lines = ["## Session Status"]
        lines.append(f"Session ID: {context.get('session_id', 'N/A')}")
        lines.append(f"Messages: {context.get('message_count', 0)}")
        lines.append(f"Platform: {platform.platform()}")
        lines.append(f"Python: {platform.python_version()}")
        lines.append(f"CWD: {os.getcwd()}")
        lines.append(f"Provider: {context.get('provider', 'N/A')}")
        lines.append(f"Model: {context.get('model', 'N/A')}")

        usage = context.get("usage")
        if usage:
            lines.append(f"Tokens in: {usage.get('input_tokens', 0)}")
            lines.append(f"Tokens out: {usage.get('output_tokens', 0)}")
            lines.append(f"Cost: ${usage.get('total_cost_usd', 0):.6f}")
        return "\n".join(lines)


class ToolsCommand(Command):
    name = "/tools"
    description = "List all available tools"
    aliases = ["/t"]

    def __init__(self, registry: ToolRegistry) -> None:
        self._registry = registry

    async def execute(self, args: str, context: dict[str, Any]) -> str:
        tools = self._registry.list_tools()
        cat_filter = args.strip().lower()

        lines = ["## Available Tools\n"]
        by_cat: dict[str, list[str]] = {}
        for t in tools:
            cat = t.category.value if hasattr(t, "category") else "uncategorized"
            if cat_filter and cat_filter != cat:
                continue
            by_cat.setdefault(cat, [])
            by_cat[cat].append(f"  {t.name:<25} [{t.permission_level.value:<7}] {t.description[:80]}")

        for cat in sorted(by_cat):
            lines.append(f"### {cat.capitalize()}")
            lines.extend(by_cat[cat])
            lines.append("")

        lines.append(f"Total: {len(tools)} tool(s)")
        if cat_filter:
            lines.append(f"(filtered by category: {cat_filter})")
        return "\n".join(lines)


class ClearCommand(Command):
    name = "/clear"
    description = "Clear the conversation history"

    async def execute(self, args: str, context: dict[str, Any]) -> str:
        messages = context.get("messages", [])
        if messages:
            messages.clear()
        return "Conversation cleared."


class ConfigCommand(Command):
    name = "/config"
    description = "Show current configuration"

    async def execute(self, args: str, context: dict[str, Any]) -> str:
        from app.utils.config import settings
        lines = ["## Configuration"]
        lines.append(f"Provider: {settings.llm_provider}")
        lines.append(f"Model: {settings.llm_model}")
        lines.append(f"Max turns: {settings.max_tool_rounds}")
        lines.append(f"Max tokens: {settings.max_tokens}")
        lines.append(f"Redis: {settings.redis_url}")
        lines.append(f"Log level: {settings.log_level}")
        return "\n".join(lines)
