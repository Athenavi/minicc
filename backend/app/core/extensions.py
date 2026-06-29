"""ExtensionLoader — 扩展加载器。

管理 MCP 客户端、LSP 客户端和本地 Python 插件的完整生命周期。

设计理念（参考 Claude Code）：
- MCP = 向外扩能力（接入外部系统、工具、资源）
- LSP = 向内补语义（诊断、符号、引用、跳转定义）
- 外部能力统一翻译成 BaseTool，在 ToolRegistry 中一视同仁
"""

from __future__ import annotations

import json
import logging
from pathlib import Path
from typing import Optional

from pydantic import BaseModel, Field

from app.core.lsp_client import (
    LSPClient,
    LSPConfig,
    LSPFindReferencesTool,
    LSPGoToDefinitionTool,
    LSPHoverTool,
)
from app.core.mcp_client import (
    ListMcpResourcesTool,
    MCPClient,
    MCPServerConfig,
    MCPToolAdapter,
    ReadMcpResourceTool,
)
from app.core.plugin_loader import PluginLoader
from app.tools.base import ToolRegistry

logger = logging.getLogger("minicc.extensions")


class ExtensionsConfig(BaseModel):
    """扩展配置模型。"""
    mcp_servers: dict[str, MCPServerConfig] = Field(default_factory=dict)
    lsp_configs: dict[str, LSPConfig] = Field(default_factory=dict)
    plugin_dirs: list[str] = Field(default_factory=lambda: ["~/.minicc/plugins"])


class ExtensionLoader:
    """扩展加载器。管理 MCP/LSP/Plugin 的生命周期。"""

    def __init__(self, tool_registry: ToolRegistry) -> None:
        self._registry = tool_registry
        self.mcp_clients: dict[str, MCPClient] = {}
        self.lsp_clients: dict[str, LSPClient] = {}
        self._plugin_loader = PluginLoader()
        self._config = ExtensionsConfig()

    async def load_all(self, config: Optional[ExtensionsConfig] = None) -> None:
        """加载所有配置的扩展。"""
        if config:
            self._config = config

        await self._load_mcp_servers(self._config.mcp_servers)
        await self._load_lsp_clients(self._config.lsp_configs)
        await self._load_plugins(self._config.plugin_dirs)

    async def load_mcp_servers_from_config(self, config_path: str = ".minicc/mcp.json") -> None:
        """从配置文件加载 MCP 服务器配置。"""
        path = Path(config_path)
        if not path.exists():
            logger.info("No MCP config found: %s", config_path)
            return

        try:
            data = json.loads(path.read_text(encoding="utf-8"))
            servers = {}
            for name, cfg in data.get("mcpServers", {}).items():
                servers[name] = MCPServerConfig(
                    command=cfg.get("command"),
                    args=cfg.get("args", []),
                    url=cfg.get("url"),
                    transport=cfg.get("transport", "stdio"),
                    env=cfg.get("env", {}),
                )
            await self._load_mcp_servers(servers)
        except Exception as exc:
            logger.warning("Failed to load MCP config: %s — %s", config_path, exc)

    async def _load_mcp_servers(self, servers: dict[str, MCPServerConfig]) -> None:
        """连接所有 MCP 服务器并注册工具。"""
        for name, config in servers.items():
            client = MCPClient(name, config)
            ok = await client.connect()
            if not ok:
                logger.warning("MCP server skipped: %s", name)
                continue

            self.mcp_clients[name] = client

            # 注册 MCP 工具
            for tool_def in client.get_tools():
                adapter = MCPToolAdapter(client, tool_def)
                self._registry.register(adapter)
                logger.info("MCP tool registered: %s/%s", name, tool_def.name)

            # 注册资源桥接工具（如有资源）
            if client.get_resources():
                self._registry.register(ListMcpResourcesTool(self.mcp_clients))
                self._registry.register(ReadMcpResourceTool(self.mcp_clients))
                logger.info("MCP resource tools registered for: %s", name)

            # 注册动态刷新回调
            client.set_tools_changed_callback(
                lambda c=client, n=name: self._on_mcp_tools_changed(c, n)
            )

    async def _load_lsp_clients(self, configs: dict[str, LSPConfig]) -> None:
        """初始化 LSP 客户端并注册 LSP 工具。"""
        for lang, config in configs.items():
            client = LSPClient(config)
            ok = await client.start()
            if not ok:
                logger.warning("LSP client skipped: %s", lang)
                continue

            self.lsp_clients[lang] = client

            # 注册 LSP 工具
            self._registry.register(LSPGoToDefinitionTool(client))
            self._registry.register(LSPFindReferencesTool(client))
            self._registry.register(LSPHoverTool(client))
            logger.info("LSP tools registered for: %s", lang)

    async def _load_plugins(self, dirs: list[str]) -> None:
        """加载 Python 插件。"""
        plugin_paths = [Path(d).expanduser() for d in dirs]
        self._plugin_loader = PluginLoader(plugin_paths)
        count = await self._plugin_loader.load_plugins(self._registry)
        if count > 0:
            logger.info("Plugins loaded: %d", count)

    async def refresh_mcp_tools(self) -> None:
        """刷新所有 MCP 服务器的工具列表。

        对应 Claude Code 中每轮 turn 之间的 refreshTools() 调用。
        """
        for name, client in self.mcp_clients.items():
            old_count = len(client.get_tools())
            await client.refresh_tools()
            new_count = len(client.get_tools())

            if new_count != old_count:
                logger.info("MCP tools changed: %s (%d → %d)", name, old_count, new_count)
                # 重新注册工具
                self._reload_mcp_tools(client, name)

    def _on_mcp_tools_changed(self, client: MCPClient, name: str) -> None:
        """MCP server 通知 tools/list_changed 时的处理。"""
        logger.info("MCP tools/list_changed: %s — refreshing", name)
        import asyncio
        asyncio.create_task(self._reload_mcp_tools_async(client, name))

    async def _reload_mcp_tools_async(self, client: MCPClient, name: str) -> None:
        """异步重新加载 MCP 工具。"""
        await client.refresh_tools()
        self._reload_mcp_tools(client, name)

    def _reload_mcp_tools(self, client: MCPClient, name: str) -> None:
        """重新注册指定 MCP 客户端的所有工具。"""
        # 先移除旧工具
        keys_to_remove = [k for k in list(self._registry._tools.keys()) if k.startswith(f"mcp_{name}_")]
        for key in keys_to_remove:
            self._registry._tools.pop(key, None)

        # 注册新工具
        for tool_def in client.get_tools():
            adapter = MCPToolAdapter(client, tool_def)
            self._registry.register(adapter)

    async def shutdown_all(self) -> None:
        """优雅关闭所有扩展。"""
        for name, client in self.mcp_clients.items():
            await client.shutdown()
        for lang, client in self.lsp_clients.items():
            await client.shutdown()
        self.mcp_clients.clear()
        self.lsp_clients.clear()
        logger.info("All extensions shut down")
