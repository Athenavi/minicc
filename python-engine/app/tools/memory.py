"""Memory tools (remember / recall / forget) 注册到本地工具注册表。

行为（记忆四层架构 L2 档案卡）：
- 服务可用（PG 已初始化，main.py lifespan 注入）→ 走 MemoryService：
  租户/用户隔离、四类槽位、语义检索、去重整理
- 服务不可用（无 PG 的独立运行/测试）→ 回退旧版全局 MemoryStore（兼容行为）

用户身份取自工具执行上下文（app.tools.context，AgentRuntime.run 内设置）。
"""
from __future__ import annotations

from typing import Any, Optional

from app.tools.registry import registry
from app.tools.context import get_tenant_id, get_user_id
from app.memory.store import store as memory_store


def _svc():
    from app.memory.service import get_service
    return get_service()


async def remember(key: str, value: str, slot: str = "fact") -> dict[str, Any]:
    if not key or not value:
        return {"error": "key and value are required"}

    svc = _svc()
    if svc is None:
        # 独立运行回退（无 PostgreSQL）：保持旧版全局 KV 行为
        memory_store.save(key, value, source="ai")
        return {"output": f"Remembered: {key} = {value}"}

    user_id = get_user_id()
    if not user_id:
        return {"error": "memory service requires user context (run via agent runtime)"}
    tenant_id = get_tenant_id() or "default"
    try:
        result = await svc.upsert(
            tenant_id=tenant_id, user_id=user_id,
            slot=slot or "fact", key=key, value=value,
            confidence=60, source="tool_written",
        )
    except ValueError as e:
        return {"error": str(e)}
    entry = result["entry"]
    out = f"Remembered [{entry['slot']}] {entry['key']}: {entry['value']}"
    if result.get("duplicate_of"):
        dup = result["duplicate_of"]
        out += f" (note: similar to existing '{dup['key']}')"
    return {"output": out, "id": entry["id"], "slot": entry["slot"]}


async def recall(query: str = "", slot: Optional[str] = None) -> dict[str, Any]:
    svc = _svc()
    if svc is None:
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

    user_id = get_user_id()
    if not user_id:
        return {"error": "memory service requires user context (run via agent runtime)"}
    tenant_id = get_tenant_id() or "default"

    if query:
        data = await svc.search(tenant_id, user_id, query=query, top_k=10, slot=slot or None)
        results = data["results"]
        if not results:
            return {"output": f"No memories found for: {query}"}
        lines = [
            f"- [{r['slot_label']}] {r['key']}: {r['value']} (relevance {r['score']:.2f})"
            for r in results
        ]
        return {"output": "\n".join(lines), "count": len(results)}

    # 空 query：列出全部（按槽位分组）
    data = await svc.list_entries(tenant_id, user_id)
    if not data["entries"]:
        return {"output": "No memories saved yet."}
    lines = [f"- [{e['slot_label']}] {e['key']}: {e['value']}" for e in data["entries"]]
    return {"output": "\n".join(lines), "count": data["total"]}


async def forget(key: str, slot: Optional[str] = None) -> dict[str, Any]:
    if not key:
        return {"error": "key is required"}

    svc = _svc()
    if svc is None:
        memory_store.delete(key)
        return {"output": f"Forgot: {key}"}

    user_id = get_user_id()
    if not user_id:
        return {"error": "memory service requires user context (run via agent runtime)"}
    tenant_id = get_tenant_id() or "default"
    deleted = await svc.forget_by_key(tenant_id, user_id, key, slot=slot or None)
    if not deleted:
        return {"output": f"No memory found for key: {key}"}
    return {"output": f"Forgot: {key}", "deleted": deleted}


registry.register(
    name="remember",
    description=(
        "Save an important fact, decision, or preference to the user's persistent "
        "long-term memory (cross-session). slot: identity/preference/decision/fact."
    ),
    parameters={
        "type": "object",
        "properties": {
            "key": {"type": "string", "description": "Unique key for this memory (e.g. 'timezone')"},
            "value": {"type": "string", "description": "Memory content to remember"},
            "slot": {
                "type": "string",
                "enum": ["identity", "preference", "decision", "fact"],
                "description": "Memory category slot (default: fact)",
            },
        },
        "required": ["key", "value"],
    },
    handler=remember,
)

registry.register(
    name="recall",
    description=(
        "Retrieve the user's long-term memories by semantic search "
        "(similarity + rerank), or list all when query is empty."
    ),
    parameters={
        "type": "object",
        "properties": {
            "query": {"type": "string", "description": "Search query to find matching memories", "default": ""},
            "slot": {
                "type": "string",
                "enum": ["identity", "preference", "decision", "fact"],
                "description": "Optional: restrict search to one slot",
            },
        },
    },
    handler=recall,
)

registry.register(
    name="forget",
    description="Remove a saved memory by key (optionally scoped to a slot).",
    parameters={
        "type": "object",
        "properties": {
            "key": {"type": "string", "description": "Key of the memory to forget"},
            "slot": {
                "type": "string",
                "enum": ["identity", "preference", "decision", "fact"],
                "description": "Optional: only forget within this slot",
            },
        },
        "required": ["key"],
    },
    handler=forget,
)
