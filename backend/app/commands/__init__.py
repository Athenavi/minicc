"""命令系统 — Slash Commands。

对应 Claude Code 的 commands.ts：
命令和工具是两套机制：
- 工具系统：给模型调用
- 命令系统：给用户显式输入

命令是"人直接操作系统"的入口，
工具是"模型代替人操作系统"的入口。
两者共享同一个底层世界。
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import Any, Optional


class Command(ABC):
    """命令基类。每个命令对应一个 /<name> 用户输入。"""

    name: str = ""
    description: str = ""
    aliases: list[str] = []

    @abstractmethod
    async def execute(self, args: str, context: dict[str, Any]) -> str:
        """执行命令。args 是 /name 后面的参数。"""
        ...


class CommandDispatcher:
    """命令分发器。匹配 /<name> 输入并路由到对应的 Command。"""

    def __init__(self) -> None:
        self._commands: dict[str, Command] = {}

    def register(self, cmd: Command) -> None:
        if not cmd.name:
            raise ValueError("Command must have a name")
        self._commands[cmd.name] = cmd
        for alias in cmd.aliases:
            self._commands[alias] = cmd

    def get(self, name: str) -> Command | None:
        return self._commands.get(name)

    def list_commands(self) -> list[Command]:
        return list(set(self._commands.values()))

    async def dispatch(self, text: str, context: dict[str, Any]) -> str | None:
        """分发命令。如果 text 以 / 开头，尝试匹配命令。

        Returns:
            命令输出文本，或 None（不是命令）
        """
        if not text.startswith("/"):
            return None

        parts = text.split(maxsplit=1)
        cmd_name = parts[0].lower()
        args = parts[1] if len(parts) > 1 else ""

        cmd = self._commands.get(cmd_name)
        if not cmd:
            return f"Unknown command: {cmd_name}. Type /help for available commands."

        try:
            return await cmd.execute(args, context)
        except Exception as exc:
            return f"Command error: {exc}"
