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


# memory_search token 预算硬上限（字符数）
_MEMORY_SEARCH_MAX_CHARS = 6000
# 单次 query 最大字符数，防止超长 query 导致嵌入失败
_MEMORY_SEARCH_MAX_QUERY = 500


async def memory_search(query: str, limit: int = 10) -> dict[str, Any]:
    """记忆搜索工具：语义搜索记忆（L3 实现）。

    Args:
        query: 查询文本。
        limit: 返回数量限制（1–20，默认 10）。

    Returns:
        操作结果，包含 output / results / count。
    """
    # 输入校验
    if not query:
        return {"error": "query is required", "results": []}

    if len(query) > _MEMORY_SEARCH_MAX_QUERY:
        return {
            "error": f"query exceeds {_MEMORY_SEARCH_MAX_QUERY} chars limit",
            "results": [],
        }

    # 规范化 limit 范围
    try:
        limit = int(limit)
    except (TypeError, ValueError):
        limit = 10
    if limit < 1:
        limit = 1
    if limit > 20:
        limit = 20

    svc = _get_memory_service()
    if svc is None:
        return {"output": "Memory service not available.", "results": []}

    user_id = get_user_id()
    if not user_id:
        return {"error": "memory service requires user context", "results": []}

    tenant_id = get_tenant_id() or "default"
    session_id = get_session_id() or "unknown"

    scope = Scope(
        tenant_id=tenant_id,
        user_id=user_id,
        session_id=session_id,
    )

    try:
        # 调用 recall 方法并显式传递 top_k
        result = await svc.recall(scope=scope, query=query, top_k=limit)

        # 从 summary_items 中提取 L3 结果
        l3_results = result.summary_items[:limit] if hasattr(result, 'summary_items') else []

        if not l3_results:
            return {
                "output": f"No semantic memories found for: {query}",
                "results": [],
                "count": 0,
            }

        # 格式化结果，严格执行 token 字符预算
        formatted: list[dict[str, Any]] = []
        total_chars = 0
        for item in l3_results:
            content = item.content or ""
            # 单条截断，避免超长单条（含省略号总长度 ≤ 600）
            if len(content) > 597:
                content = content[:597] + "…"
            entry = {
                "id": item.id,
                "content": content,
                "topics": item.topics,
                "score": item.score,
                "session_id": item.session_id,
                "created_at": item.created_at,
            }
            entry_chars = len(content)
            # 保留至少 1 条结果，后续条目若超出预算则停止
            if formatted and total_chars + entry_chars > _MEMORY_SEARCH_MAX_CHARS:
                break
            formatted.append(entry)
            total_chars += entry_chars

        return {
            "output": f"Found {len(formatted)} memories for: {query}",
            "results": formatted,
            "count": len(formatted),
            "truncated": len(formatted) < len(l3_results),
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
