"""Memory tools (remember / recall / forget) 注册到本地工具注册表。

实现对标 Go `internal/memory/tools.go`（已标记 DEPRECATED），行为兼容：
- remember: 保存 key/value
- recall: 按 query 搜索或列出全部
- forget: 删除指定 key
"""
from __future__ import annotations

from typing import Any

from app.tools.registry import registry
from app.memory.store import store as memory_store


async def remember(key: str, value: str) -> dict[str, Any]:
    if not key or not value:
        return {"error": "key and value are required"}
    memory_store.save(key, value, source="ai")
    return {"output": f"Remembered: {key} = {value}"}


async def recall(query: str = "") -> dict[str, Any]:
    if not query:
        facts = memory_store.all()
        if not facts:
            return {"output": "No facts saved yet."}
        lines = [f"- {f.key}: {f.value}" for f in facts]
        return {"output": "\n".join(lines), "count": len(facts)}
    results = memory_store.search(query)
    if not results:
        return {"output": f"No facts found for: {query}"}
    lines = [f"- {f.key}: {f.value}" for f in results]
    return {"output": "\n".join(lines), "count": len(results)}


async def forget(key: str) -> dict[str, Any]:
    if not key:
        return {"error": "key is required"}
    memory_store.delete(key)
    return {"output": f"Forgot: {key}"}


registry.register(
    name="remember",
    description="Save an important fact, decision, or finding to persistent memory.",
    parameters={
        "type": "object",
        "properties": {
            "key": {"type": "string", "description": "Unique key for this fact"},
            "value": {"type": "string", "description": "Fact content to remember"},
        },
        "required": ["key", "value"],
    },
    handler=remember,
)

registry.register(
    name="recall",
    description="Retrieve saved facts from persistent memory.",
    parameters={
        "type": "object",
        "properties": {
            "query": {"type": "string", "description": "Search query to find matching facts", "default": ""},
        },
    },
    handler=recall,
)

registry.register(
    name="forget",
    description="Remove a saved fact from persistent memory by key.",
    parameters={
        "type": "object",
        "properties": {
            "key": {"type": "string", "description": "Key of the fact to forget"},
        },
        "required": ["key"],
    },
    handler=forget,
)
