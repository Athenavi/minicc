"""MCPClientPool — 用户级 MCP 连接池（无状态友好的可重建缓存）。

- 按配置指纹去重：相同 command/args/env 的 MCP 服务器多用户共享一个子进程连接；
  工具注册到本地 registry 时合并归属用户集合（owner）。
- 25s 轮询「活跃用户」的插件配置：签名变化则重连该用户的服务器并重注册工具；
  用户不再活跃时释放其工具与连接引用（引用归零的连接关闭）。
- 状态均为进程内可重建缓存：实例重启后从 PluginStore 重新加载（幂等）。

多实例部署：各实例独立运行本池（MCP stdio 子进程无法跨实例共享）；
ActiveTracker 换 Redis 实现后，轮询范围由共享活跃标记驱动。
"""
from __future__ import annotations

import asyncio
import json
import logging
from dataclasses import dataclass, field
from typing import Any

from app.mcp.client import MCPClient
from app.plugins.store import ActiveTracker, PluginStore, ServerConfig
from app.tools.registry import registry

logger = logging.getLogger(__name__)

POLL_INTERVAL = 25  # 秒


def _fingerprint(server: ServerConfig) -> str:
    """配置指纹：name/command/args/env 的规范化 JSON。"""
    payload = {
        "command": server.command,
        "args": server.args,
        "env": server.env,
    }
    return json.dumps(payload, ensure_ascii=False, sort_keys=True)


@dataclass
class _SharedConnection:
    """按配置指纹共享的 MCP 连接及其用户集合。"""
    key: str
    client: MCPClient
    users: set[str] = field(default_factory=set)


class MCPClientPool:
    def __init__(self, store: PluginStore | None = None, tracker: ActiveTracker | None = None) -> None:
        self._store = store or PluginStore()
        self._tracker = tracker or ActiveTracker()
        self._conns: dict[str, _SharedConnection] = {}   # fingerprint -> shared conn
        self._user_sigs: dict[str, str] = {}             # user_id -> 已加载配置签名
        self._user_conns: dict[str, set[str]] = {}       # user_id -> 引用的指纹集合
        self._user_tools: dict[str, set[str]] = {}       # user_id -> 注册的工具名集合
        self._lock = asyncio.Lock()
        self._poll_task: asyncio.Task | None = None

    async def start(self) -> None:
        if self._poll_task is None:
            self._poll_task = asyncio.create_task(self._poll_loop())

    async def stop(self) -> None:
        if self._poll_task:
            self._poll_task.cancel()
            try:
                await self._poll_task
            except asyncio.CancelledError:
                pass
            self._poll_task = None
        async with self._lock:
            for key in list(self._conns):
                await self._close_conn(key)
            self._conns.clear()

    async def _poll_loop(self) -> None:
        while True:
            await asyncio.sleep(POLL_INTERVAL)
            try:
                await self.reconcile()
            except Exception as e:  # 轮询失败不影响主流程
                logger.warning("mcp reconcile failed: %s", e, exc_info=True)

    async def reconcile(self) -> None:
        """处理活跃用户的配置变动；回收不再活跃用户。"""
        active = set(self._tracker.active_users())
        async with self._lock:
            # 1. 同步活跃用户
            for uid in active:
                await self._sync_user_locked(uid)
            # 2. 回收已非活跃用户
            for uid in list(self._user_sigs):
                if uid not in active:
                    await self._release_user_locked(uid)
            # 3. 关闭引用归零的连接
            for key in [k for k, c in self._conns.items() if not c.users]:
                await self._close_conn(key)
            # 4. 清理过期活跃标记（避免内存增长）
            self._tracker.prune()

    async def _sync_user_locked(self, uid: str) -> None:
        sig = self._store.signature(uid)
        if sig == self._user_sigs.get(uid):
            return  # 配置无变动
        # 变更：先释放旧的，再按新配置建立
        await self._release_user_locked(uid)

        servers = self._store.active_servers(uid)
        self._user_conns[uid] = set()
        self._user_tools[uid] = set()
        for server in servers:
            key = _fingerprint(server)
            shared = self._conns.get(key)
            if shared is None:
                client = MCPClient([_server_to_def(server)])
                try:
                    await client.start()
                except Exception as e:  # 单个服务器失败不阻塞其余
                    logger.warning("mcp connect %s failed for user %s: %s", server.name, uid, e)
                    continue
                shared = _SharedConnection(key=key, client=client)
                self._conns[key] = shared
                logger.info("mcp shared connection created: %s", key)
            shared.users.add(uid)
            self._user_conns[uid].add(key)
            for tool in shared.client.tools:
                registry.register(
                    name=tool.name,
                    description=tool.description,
                    parameters=tool.input_schema,
                    handler=_make_tool_handler(shared.client, tool.name),
                    owner=uid,
                )
                self._user_tools[uid].add(tool.name)
        self._user_sigs[uid] = sig
        if self._user_tools[uid]:
            logger.info("user %s mcp tools: %d", uid, len(self._user_tools[uid]))

    async def _release_user_locked(self, uid: str) -> None:
        """移除用户对工具与连接的引用。"""
        for tool_name in self._user_tools.pop(uid, set()):
            tool = registry.get(tool_name)
            if tool is None:
                continue
            tool.owners.discard(uid)
            if not tool.owners:
                registry.unregister(tool_name)
        for key in self._user_conns.pop(uid, set()):
            if shared := self._conns.get(key):
                shared.users.discard(uid)
        self._user_sigs.pop(uid, None)

    async def _close_conn(self, key: str) -> None:
        shared = self._conns.pop(key, None)
        if shared is not None:
            try:
                await shared.client.close()
            except Exception as e:  # noqa: BLE001
                logger.warning("mcp close %s failed: %s", key, e)

    def status(self) -> dict[str, Any]:
        return {
            "active_users": len(self._tracker.active_users()),
            "shared_connections": len(self._conns),
            "user_loaded": len(self._user_sigs),
        }


def _server_to_def(server: ServerConfig):
    from app.mcp.client import ServerDef
    return ServerDef(name=server.name, command=server.command, args=server.args, env=server.env)


def _make_tool_handler(client: MCPClient, tool_name: str):
    """构造 MCP 工具 handler（绑定共享连接）。"""

    async def handler(**kwargs: Any) -> dict[str, Any]:
        return await client.call_tool(tool_name, kwargs)

    return handler
