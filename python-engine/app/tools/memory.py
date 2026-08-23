"""Memory tools (remember / recall / forget / memory_search) 注册到本地工具注册表。

行为（记忆四层架构）：
- 使用 MemoryService（L2 档案卡 + L3 摘要召回）
- 支持四类槽位：identity/preference/decision/fact
- 支持冲突检测与处理
- memory_search 支持 L3 语义检索（占位）

用户身份取自工具执行上下文（app.tools.context，AgentRuntime.run 内设置）。
"""
from __future__ import annotations

import logging
from typing import Any, Optional

from app.tools.registry import registry
from app.tools.context import get_tenant_id, get_user_id, get_session_id
from app.memory.layers import SlotType, SourceType, Scope

logger = logging.getLogger(__name__)


def _get_memory_service():
    """获取 MemoryService 实例（从应用上下文中）。"""
    try:
        from app.memory.service import get_memory_service
        return get_memory_service()
    except ImportError:
        return None


async def remember(key: str, value: str, slot: str = "fact") -> dict[str, Any]:
    """记忆工具：保存用户档案条目。

    Args:
        key: 记忆键。
        value: 记忆值。
        slot: 槽位类型。

    Returns:
        操作结果。
    """
    if not key or not value:
        return {"error": "key and value are required"}

    svc = _get_memory_service()
    if svc is None:
        return {
            "output": f"Memory service not available. Cannot remember: {key}",
            "warning": "Memory service is not initialized. Please configure PostgreSQL.",
        }

    user_id = get_user_id()
    if not user_id:
        return {"error": "memory service requires user context (run via agent runtime)"}

    tenant_id = get_tenant_id() or "default"
    session_id = get_session_id() or "unknown"

    # 映射槽位
    try:
        slot_type = SlotType(slot.lower())
    except ValueError:
        slot_type = SlotType.FACT

    try:
        result = await svc.update_profile(
            tenant_id=tenant_id,
            user_id=user_id,
            slot=slot_type,
            item_key=key,
            item_value=value,
            confidence=60,
            source=SourceType.TOOL_WRITTEN,
        )

        if result.conflict:
            # 冲突检测：产出警告，让 Agent 知晓
            conflict = result.conflict
            return {
                "output": (
                    f"⚠️ Conflict detected for [{slot_type.value}] {key}: "
                    f"Old value: {conflict.old_value}, New value: {conflict.new_value}. "
                    f"Please confirm with user before overwriting."
                ),
                "conflict": {
                    "conflict_id": conflict.conflict_id,
                    "slot": conflict.slot.value,
                    "key": conflict.item_key,
                    "old_value": conflict.old_value,
                    "new_value": conflict.new_value,
                },
                "needs_confirmation": True,
            }

        # 成功
        item = result.item
        out = f"✅ Remembered [{slot_type.value}] {key}: {value}"
        if item and item.version > 1:
            out += f" (updated, version {item.version})"
        return {"output": out, "success": True}

    except Exception as e:
        logger.error("Remember failed: %s", e)
        return {"error": f"Failed to remember: {str(e)}"}


async def recall(query: str = "", slot: Optional[str] = None) -> dict[str, Any]:
    """回忆工具：召回用户记忆。

    Args:
        query: 查询文本。
        slot: 可选，限制槽位。

    Returns:
        操作结果，包含 L2 档案卡 + L3 摘要。
    """
    svc = _get_memory_service()
    if svc is None:
        return {
            "output": "Memory service not available. Cannot recall memories.",
            "warning": "Memory service is not initialized.",
        }

    user_id = get_user_id()
    if not user_id:
        return {"error": "memory service requires user context (run via agent runtime)"}

    tenant_id = get_tenant_id() or "default"
    session_id = get_session_id() or "unknown"

    scope = Scope(
        tenant_id=tenant_id,
        user_id=user_id,
        session_id=session_id,
    )

    try:
        result = await svc.recall(scope=scope, query=query)

        parts = []

        # L2 档案卡
        if result.profile_block:
            parts.append("📋 **用户档案卡 (L2)**:")
            parts.append(result.profile_block)

        # L3 摘要（占位，Task 16 实现后返回）
        if result.summary_items:
            parts.append("\n📚 **相关历史摘要 (L3)**:")
            for item in result.summary_items[:5]:
                parts.append(f"- [{item.session_id}] {item.content[:100]}... (score: {item.score:.2f})")

        if not parts:
            return {"output": "No memories found yet."}

        return {"output": "\n".join(parts), "count": len(result.summary_items)}

    except Exception as e:
        logger.error("Recall failed: %s", e)
        return {"error": f"Failed to recall: {str(e)}"}


async def forget(key: str, slot: Optional[str] = None) -> dict[str, Any]:
    """遗忘工具：删除用户档案条目。

    Args:
        key: 记忆键。
        slot: 可选，限制槽位。

    Returns:
        操作结果。
    """
    if not key:
        return {"error": "key is required"}

    svc = _get_memory_service()
    if svc is None:
        return {
            "output": "Memory service not available. Cannot forget.",
            "warning": "Memory service is not initialized.",
        }

    user_id = get_user_id()
    if not user_id:
        return {"error": "memory service requires user context (run via agent runtime)"}

    tenant_id = get_tenant_id() or "default"

    # 映射槽位
    try:
        slot_type = SlotType(slot.lower()) if slot else SlotType.FACT
    except ValueError:
        slot_type = SlotType.FACT

    try:
        deleted = await svc.forget(
            tenant_id=tenant_id,
            user_id=user_id,
            slot=slot_type,
            item_key=key,
        )

        if deleted:
            return {"output": f"✅ Forgot: {key}", "success": True}
        else:
            return {"output": f"No memory found for key: {key}", "success": False}

    except Exception as e:
        logger.error("Forget failed: %s", e)
        return {"error": f"Failed to forget: {str(e)}"}


async def memory_search(query: str, limit: int = 10) -> dict[str, Any]:
    """记忆搜索工具：语义搜索记忆（L3 占位实现）。

    Args:
        query: 查询文本。
        limit: 返回数量限制。

    Returns:
        操作结果。
    """
    svc = _get_memory_service()
    if svc is None:
        return {"output": "Memory service not available.", "results": []}

    user_id = get_user_id()
    if not user_id:
        return {"error": "memory service requires user context"}

    tenant_id = get_tenant_id() or "default"
    session_id = get_session_id() or "unknown"

    scope = Scope(
        tenant_id=tenant_id,
        user_id=user_id,
        session_id=session_id,
    )

    try:
        # 调用 recall 方法（L2 + L3 合并）
        result = await svc.recall(scope=scope, query=query)

        # 从 summary_items 中提取 L3 结果
        l3_results = result.summary_items[:limit] if hasattr(result, 'summary_items') else []

        if not l3_results:
            return {"output": f"No semantic memories found for: {query}", "results": []}

        # 格式化结果
        formatted = []
        for item in l3_results:
            formatted.append({
                "id": item.id,
                "content": item.content,
                "topics": item.topics,
                "score": item.score,
                "session_id": item.session_id,
                "created_at": item.created_at,
            })

        return {
            "output": f"Found {len(formatted)} memories for: {query}",
            "results": formatted,
            "count": len(formatted),
        }

    except Exception as e:
        logger.error("Memory search failed: %s", e)
        return {"error": f"Search failed: {str(e)}", "results": []}


# ── 注册工具 ──────────────────────────────────────────────────────────

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
        "Retrieve the user's long-term memories. "
        "Returns both L2 profile card and L3 semantic summaries."
    ),
    parameters={
        "type": "object",
        "properties": {
            "query": {
                "type": "string",
                "description": "Search query to find matching memories",
                "default": "",
            },
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

registry.register(
    name="memory_search",
    description=(
        "Perform semantic search across user's long-term memory. "
        "Returns ranked results with relevance scores."
    ),
    parameters={
        "type": "object",
        "properties": {
            "query": {
                "type": "string",
                "description": "Natural language query to search for",
            },
            "limit": {
                "type": "integer",
                "description": "Maximum number of results to return (default: 10)",
                "default": 10,
            },
        },
        "required": ["query"],
    },
    handler=memory_search,
)
