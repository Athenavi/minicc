"""插件热加载器 — 动态导入 Python 文件中的 Tool 子类。

安全边界（参考 Claude Code MCP skills 处理）：
- 本地插件 = 可信，允许完整能力
- MCP skills = 远程不可信，限制执行能力
"""

from __future__ import annotations

import importlib.util
import logging
from pathlib import Path
from typing import Optional

from app.tools.base import BaseTool, ToolRegistry

logger = logging.getLogger("minicc.plugin")


class PluginLoader:
    """Python 插件热加载器。

    扫描 ~/.minicc/plugins 目录，动态导入 Tool 子类并注册到 ToolRegistry。
    支持文件修改后自动重载（使用 watchfiles，可选依赖）。
    """

    def __init__(self, plugin_dirs: Optional[list[Path]] = None) -> None:
        self._plugin_dirs = plugin_dirs or [
            Path.home() / ".minicc" / "plugins",
            Path.cwd() / ".minicc" / "plugins",
        ]

    async def load_plugins(self, registry: ToolRegistry) -> int:
        """加载所有插件。返回成功加载数。"""
        count = 0
        for plugin_dir in self._plugin_dirs:
            if not plugin_dir.exists():
                continue
            for file in sorted(plugin_dir.glob("*.py")):
                if file.name.startswith("_"):
                    continue
                try:
                    tool = self._load_single_tool(file)
                    if tool:
                        registry.register(tool)
                        count += 1
                        logger.info("Plugin loaded: %s → %s", file.name, tool.name)
                except Exception as exc:
                    logger.warning("Plugin load failed: %s — %s", file.name, exc)
        return count

    def _load_single_tool(self, file: Path) -> Optional[BaseTool]:
        """从单个 .py 文件加载 Tool 子类。"""
        spec = importlib.util.spec_from_file_location(file.stem, file)
        if not spec or not spec.loader:
            return None

        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)

        for attr_name in dir(module):
            attr = getattr(module, attr_name)
            if (isinstance(attr, type)
                    and issubclass(attr, BaseTool)
                    and attr is not BaseTool
                    and attr.name):
                return attr()
        return None

    def watch_for_changes(self, registry: ToolRegistry) -> None:
        """监听文件变化自动重载（需要 watchfiles 包）。"""
        try:
            from watchfiles import watch
        except ImportError:
            logger.info("watchfiles not installed — plugin auto-reload disabled")
            return

        import asyncio
        asyncio.create_task(self._watch_loop(registry))

    async def _watch_loop(self, registry: ToolRegistry) -> None:
        """后台监听循环。"""
        try:
            from watchfiles import awatch
            for _ in awatch(*[str(d) for d in self._plugin_dirs if d.exists()]):
                await self.load_plugins(registry)
        except Exception as exc:
            logger.warning("Plugin watch error: %s", exc)
