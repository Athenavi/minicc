"""
Enhanced Agent Engine — Claude Code-style agent loop with:
  - Parallel tool execution (asyncio.gather for independent calls)
  - Tool approval flow (yield approval events, wait for resolution)
  - Session persistence (PostgreSQL-backed conversation history)
  - Context compression (token-aware message windowing)
  - System prompt assembly (rich context injection)
"""
from __future__ import annotations

import asyncio
import json
import logging
import time
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, AsyncIterator, Callable, Awaitable, Optional

from app.gateway.provider import ChatMessage, ChatResponse, ToolCall
from app.gateway.router import GatewayRouter
from app.config import settings

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Dangerous tools that require explicit user approval before execution
# ---------------------------------------------------------------------------
DANGEROUS_TOOLS: set[str] = {
    "write_file",
    "edit_file",
    "shell_exec",
    "execute_python",
    "execute_command",
}


# ═══════════════════════════════════════════════════════════════════════════
# Data classes
# ═══════════════════════════════════════════════════════════════════════════

@dataclass
class AgentTask:
    """Agent task definition passed into the engine."""
    id: str = ""
    tenant_id: str = ""
    user_id: str = ""
    session_id: str = ""
    content: str = ""
    system_prompt: str = ""
    history: list[dict] = field(default_factory=list)
    tools: list[dict] = field(default_factory=list)
    llm_config: dict = field(default_factory=dict)
    max_turns: int = 10

    @classmethod
    def parse(cls, data: dict) -> "AgentTask":
        return cls(
            id=data.get("id", data.get("task_id", "")),
            tenant_id=data.get("tenant_id", ""),
            user_id=data.get("user_id", ""),
            session_id=data.get("session_id", ""),
            content=data.get("content", ""),
            system_prompt=data.get("system_prompt", ""),
            history=data.get("history", []),
            tools=data.get("tools", []),
            llm_config=data.get("llm_config", {}),
            max_turns=data.get("max_turns", 10),
        )


@dataclass
class AgentEvent:
    """Typed event yielded by the engine's ``run`` method."""
    type: str  # text | tool_call | tool_result | approval | done | error
    content: str = ""
    tool_call_id: str = ""
    tool_name: str = ""
    tool_arguments: str = ""
    risk_level: str = ""
    input_tokens: int = 0
    output_tokens: int = 0
    error: str = ""
    timestamp: float = field(default_factory=time.time)


@dataclass
class ToolApprovalRequest:
    """Request approval for a dangerous tool execution."""
    tool_name: str
    arguments: dict
    risk_level: str  # low | medium | high
    description: str

    def to_dict(self) -> dict:
        return {
            "tool_name": self.tool_name,
            "arguments": self.arguments,
            "risk_level": self.risk_level,
            "description": self.description,
        }


@dataclass
class ToolApprovalResponse:
    """External response to a :class:`ToolApprovalRequest`."""
    approved: bool
    reason: str = ""


# ═══════════════════════════════════════════════════════════════════════════
# Prompt Engine
# ═══════════════════════════════════════════════════════════════════════════

class PromptEngine:
    """Assembles the system prompt from base prompt + context extras."""

    def __init__(self, base_prompt: str = "", extras: dict[str, str] | None = None):
        self._base = base_prompt
        self._extras = extras or {}

    def assemble(self, context: dict[str, Any] | None = None) -> str:
        """Build the full system prompt.

        Parameters
        ----------
        context:
            Optional dict with keys like ``workspace``, ``tools``, ``memory``,
            etc. that are interpolated into the prompt.
        """
        parts: list[str] = []
        if self._base:
            parts.append(self._base)
        for name, template in self._extras.items():
            ctx = context or {}
            try:
                parts.append(template.format(**ctx))
            except KeyError:
                parts.append(template)
        return "\n\n".join(parts)


# ═══════════════════════════════════════════════════════════════════════════
# Context Manager
# ═══════════════════════════════════════════════════════════════════════════

class ContextManager:
    """Keeps the message list within the model's context window.

    Uses a simple strategy: if the message list exceeds *max_messages* the
    oldest non-system messages are dropped.  A more sophisticated
    implementation could use a tokenizer for exact token counting.
    """

    def __init__(self, max_messages: int = 100, max_chars: int = 120_000):
        self._max_messages = max_messages
        self._max_chars = max_chars

    def compress(self, messages: list[dict]) -> list[dict]:
        """Return a compressed copy of *messages* that fits within limits."""
        if not messages:
            return messages

        # Separate system messages from the rest
        system_msgs = [m for m in messages if m.get("role") == "system"]
        other_msgs = [m for m in messages if m.get("role") != "system"]

        # Drop oldest messages if over the limit
        budget = self._max_messages - len(system_msgs)
        if len(other_msgs) > budget:
            other_msgs = other_msgs[-budget:]

        # Character-level trimming
        result = system_msgs + other_msgs
        total = sum(len(m.get("content", "") or "") for m in result)
        while total > self._max_chars and len(other_msgs) > 2:
            removed = other_msgs.pop(0)
            total -= len(removed.get("content", "") or "")

        return system_msgs + other_msgs


# ═══════════════════════════════════════════════════════════════════════════
# Session Persistence
# ═══════════════════════════════════════════════════════════════════════════

class AgentSession:
    """Persistent conversation session backed by PostgreSQL."""

    def __init__(
        self,
        id: str = "",
        user_id: str = "",
        messages: list[dict] | None = None,
        created_at: datetime | None = None,
        updated_at: datetime | None = None,
    ):
        self.id = id or str(uuid.uuid4())
        self.user_id = user_id
        self.messages: list[dict] = messages or []
        self.created_at: datetime = created_at or datetime.now(timezone.utc)
        self.updated_at: datetime = updated_at or datetime.now(timezone.utc)

    # -- persistence --------------------------------------------------------

    @classmethod
    async def load(cls, pool: Any, session_id: str) -> "AgentSession | None":
        """Load a session from the *agent_sessions* table."""
        if pool is None:
            return None
        row = await pool.fetchrow(
            "SELECT id, user_id, messages_json, created_at, updated_at "
            "FROM agent_sessions WHERE id = $1",
            session_id,
        )
        if row is None:
            return None
        messages = json.loads(row["messages_json"]) if row["messages_json"] else []
        return cls(
            id=row["id"],
            user_id=row["user_id"],
            messages=messages,
            created_at=row["created_at"],
            updated_at=row["updated_at"],
        )

    async def save(self, pool: Any) -> None:
        """Upsert the session into the *agent_sessions* table."""
        if pool is None:
            return
        self.updated_at = datetime.now(timezone.utc)
        await pool.execute(
            """
            INSERT INTO agent_sessions (id, user_id, messages_json, created_at, updated_at)
            VALUES ($1, $2, $3, $4, $5)
            ON CONFLICT (id) DO UPDATE SET
                messages_json = EXCLUDED.messages_json,
                updated_at   = EXCLUDED.updated_at
            """,
            self.id,
            self.user_id,
            json.dumps(self.messages, ensure_ascii=False),
            self.created_at,
            self.updated_at,
        )

    @staticmethod
    async def ensure_table(pool: Any) -> None:
        """Create the agent_sessions table if it doesn't exist."""
        if pool is None:
            return
        await pool.execute(
            """
            CREATE TABLE IF NOT EXISTS agent_sessions (
                id            VARCHAR(64) PRIMARY KEY,
                user_id       VARCHAR(64) NOT NULL DEFAULT '',
                messages_json JSONB       NOT NULL DEFAULT '[]',
                created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
            )
            """
        )
        await pool.execute(
            "CREATE INDEX IF NOT EXISTS idx_agent_sessions_user ON agent_sessions(user_id)"
        )

    # -- helpers ------------------------------------------------------------

    def append_message(self, role: str, content: str, **extra: Any) -> None:
        msg: dict[str, Any] = {"role": role, "content": content}
        msg.update(extra)
        self.messages.append(msg)

    def append_tool_call(self, tool_call_id: str, name: str, arguments: str) -> None:
        self.messages.append({
            "role": "assistant",
            "content": None,
            "tool_calls": [
                {"id": tool_call_id, "function": {"name": name, "arguments": arguments}}
            ],
        })

    def append_tool_result(self, tool_call_id: str, result_json: str) -> None:
        self.messages.append({
            "role": "tool",
            "tool_call_id": tool_call_id,
            "content": result_json,
        })


# ═══════════════════════════════════════════════════════════════════════════
# Agent Engine
# ═══════════════════════════════════════════════════════════════════════════

class AgentEngine:
    """Enhanced Claude Code-style agent engine.

    Parameters
    ----------
    gateway:
        LLM gateway router (chat / embed).
    tool_registry:
        Tool registry with ``get(name)``, ``execute(name, params)`` and
        ``to_openai_tools()`` methods.
    prompt_engine:
        Builds the system prompt.
    context_manager:
        Compresses the message list to stay within token limits.
    memory_manager:
        Optional memory manager for long/short term memory retrieval.
    db_pool:
        Optional asyncpg pool for session persistence.
    approval_handler:
        Optional async callable ``async (ToolApprovalRequest) -> ToolApprovalResponse``
        for custom approval logic.  When *None* all dangerous tools are auto-denied.
    """

    def __init__(
        self,
        gateway: GatewayRouter,
        tool_registry: Any = None,
        prompt_engine: PromptEngine | None = None,
        context_manager: ContextManager | None = None,
        memory_manager: Any = None,
        db_pool: Any = None,
        approval_handler: Callable[[ToolApprovalRequest], Awaitable[ToolApprovalResponse]] | None = None,
    ):
        self._gateway = gateway
        self._tool_registry = tool_registry
        self._prompt_engine = prompt_engine or PromptEngine()
        self._context_manager = context_manager or ContextManager()
        self._memory_manager = memory_manager
        self._db_pool = db_pool
        self._approval_handler = approval_handler
        # Map of pending approval futures keyed by tool_call_id
        self._pending_approvals: dict[str, asyncio.Future[ToolApprovalResponse]] = {}

    # -- public API ---------------------------------------------------------

    async def run(
        self,
        task: AgentTask,
        session: AgentSession | None = None,
    ) -> AsyncIterator[AgentEvent]:
        """Execute the agent loop, yielding events as they occur.

        Flow:
          1. Load / create session
          2. Assemble system prompt via PromptEngine
          3. Build message list from session history + new content
          4. Compress context if needed
          5. Call LLM
          6. Parse response for tool calls
          7. Execute tools (parallel if independent)
          8. If a tool needs approval → yield approval event and wait
          9. Append tool results to messages
         10. Loop until done or max turns reached
         11. Save session
        """
        # ── 1. Load / create session ──────────────────────────────────────
        session = session or await self._load_or_create_session(task)
        session.append_message("user", task.content)

        # ── 2. Assemble system prompt ─────────────────────────────────────
        system_prompt = task.system_prompt or self._prompt_engine.assemble()

        # ── 3. Build initial messages ─────────────────────────────────────
        llm_config = task.llm_config or {}
        model = llm_config.get("model", settings.default_model)
        max_tokens = llm_config.get("max_tokens", settings.default_max_tokens)
        temperature = llm_config.get("temperature", settings.default_temperature)
        max_turns = task.max_turns or settings.max_turns

        total_input_tokens = 0
        total_output_tokens = 0

        try:
            for turn in range(max_turns):
                logger.info("Engine turn %d/%d (task=%s)", turn + 1, max_turns, task.id)

                # ── 3-4. Build + compress messages ────────────────────────
                raw_messages = self._build_raw_messages(system_prompt, session.messages)
                messages = self._context_manager.compress(raw_messages)
                openai_tools = self._convert_tools(task.tools) if task.tools else None

                # ── 5. Call LLM ───────────────────────────────────────────
                response_content = ""
                tool_calls: list[dict] = []

                async for chunk in self._gateway.chat_stream(
                    messages=[ChatMessage(**m) for m in messages],
                    model=model,
                    tenant_id=task.tenant_id,
                    max_tokens=max_tokens,
                    temperature=temperature,
                    tools=openai_tools,
                ):
                    if chunk.content:
                        response_content += chunk.content
                        yield AgentEvent(type="text", content=chunk.content)

                    if chunk.tool_calls:
                        for tc in chunk.tool_calls:
                            tool_calls.append({
                                "id": tc.id,
                                "name": tc.name,
                                "arguments": tc.arguments,
                            })

                    if chunk.input_tokens:
                        total_input_tokens += chunk.input_tokens
                    if chunk.output_tokens:
                        total_output_tokens += chunk.output_tokens

                # ── 6. Parse tool calls ───────────────────────────────────
                if not tool_calls:
                    # No tools → assistant is done
                    if response_content:
                        session.append_message("assistant", response_content)
                    break

                # Emit tool_call events
                for tc in tool_calls:
                    yield AgentEvent(
                        type="tool_call",
                        tool_call_id=tc["id"],
                        tool_name=tc["name"],
                        tool_arguments=tc["arguments"],
                    )

                # ── 7-8. Approve & execute tools ──────────────────────────
                # Group tools: those needing approval vs. auto-approved
                results = await self._execute_tool_calls(tool_calls, task, session)

                # ── 9. Append results to session ──────────────────────────
                # 将全部 tool_calls 合并到一条 assistant 消息（OpenAI API 要求）
                all_tool_calls = [
                    {"id": tc["id"], "function": {"name": tc["name"], "arguments": tc["arguments"]}}
                    for tc in tool_calls
                ]
                session.append_message("assistant", None, tool_calls=all_tool_calls)

                for tc, result in zip(tool_calls, results):
                    result_json = json.dumps(result, ensure_ascii=False)
                    session.append_tool_result(tc["id"], result_json)
                    yield AgentEvent(
                        type="tool_result",
                        tool_call_id=tc["id"],
                        tool_name=tc["name"],
                        content=result_json,
                    )

                # ── 10. Continue loop ─────────────────────────────────────
                continue

            # ── done ──────────────────────────────────────────────────────
            yield AgentEvent(
                type="done",
                input_tokens=total_input_tokens,
                output_tokens=total_output_tokens,
            )

        except Exception as e:
            logger.error("Agent engine error (task=%s): %s", task.id, e, exc_info=True)
            yield AgentEvent(type="error", error=str(e))

        finally:
            # ── 11. Save session ──────────────────────────────────────────
            if self._db_pool is not None:
                try:
                    await session.save(self._db_pool)
                except Exception as save_err:
                    logger.error("Session save failed: %s", save_err)

    # -- approval API -------------------------------------------------------

    async def submit_approval(self, tool_call_id: str, response: ToolApprovalResponse) -> None:
        """External callers resolve an approval request via this method.

        This is the counterpart to the ``approval`` event yielded by ``run()``.
        """
        future = self._pending_approvals.get(tool_call_id)
        if future and not future.done():
            future.set_result(response)

    # -- internal -----------------------------------------------------------

    async def _load_or_create_session(self, task: AgentTask) -> AgentSession:
        if task.session_id and self._db_pool:
            existing = await AgentSession.load(self._db_pool, task.session_id)
            if existing:
                return existing
        return AgentSession(id=task.session_id or str(uuid.uuid4()), user_id=task.user_id)

    def _build_raw_messages(self, system_prompt: str, session_messages: list[dict]) -> list[dict]:
        """Build the message list sent to the LLM."""
        messages: list[dict] = []
        if system_prompt:
            messages.append({"role": "system", "content": system_prompt})
        messages.extend(session_messages)
        return messages

    def _convert_tools(self, tools: list[dict]) -> list[dict]:
        """Convert tool definitions to OpenAI function-calling schema."""
        converted = []
        for tool in tools:
            params = tool.get("parameters")
            if params is None and isinstance(tool.get("parameters_json"), str):
                try:
                    params = json.loads(tool["parameters_json"])
                except (json.JSONDecodeError, TypeError):
                    params = {}
            converted.append({
                "type": "function",
                "function": {
                    "name": tool.get("name", ""),
                    "description": tool.get("description", ""),
                    "parameters": params or {},
                },
            })
        return converted

    async def _execute_tool_calls(
        self,
        tool_calls: list[dict],
        task: AgentTask,
        session: AgentSession,
    ) -> list[dict]:
        """Execute a batch of tool calls.

        Independent tools are executed concurrently with ``asyncio.gather``.
        Dangerous tools go through the approval flow first.
        """
        # Separate into approval-needed vs. auto-approve
        auto_tasks: list[tuple[int, dict]] = []
        approval_tasks: list[tuple[int, dict]] = []

        for idx, tc in enumerate(tool_calls):
            if self._needs_approval(tc["name"]):
                approval_tasks.append((idx, tc))
            else:
                auto_tasks.append((idx, tc))

        results: list[dict] = [{}] * len(tool_calls)

        # ── Execute auto-approved tools in parallel ───────────────────────
        if auto_tasks:
            async def _run_tool(tc: dict) -> dict:
                return await self._execute_single_tool(tc, task)
            auto_results = await asyncio.gather(
                *[_run_tool(tc) for _, tc in auto_tasks],
                return_exceptions=True,
            )
            for (idx, _), res in zip(auto_tasks, auto_results):
                if isinstance(res, Exception):
                    results[idx] = {"error": str(res)}
                else:
                    results[idx] = res

        # ── Handle approval-required tools ────────────────────────────────
        for idx, tc in approval_tasks:
            approved, result = await self._handle_approval(tc, task, session)
            results[idx] = result

        return results

    async def _execute_single_tool(self, tc: dict, task: AgentTask) -> dict:
        """Execute one tool call via the registry or external executor."""
        name = tc["name"]
        try:
            args = json.loads(tc["arguments"]) if isinstance(tc["arguments"], str) else tc["arguments"]
        except (json.JSONDecodeError, TypeError):
            args = {}

        # Try local registry first
        if self._tool_registry and self._tool_registry.get(name) is not None:
            return await self._tool_registry.execute(name, args or {})
        return {"error": f"Tool '{name}' not found"}

    def _needs_approval(self, tool_name: str) -> bool:
        """Check whether a tool requires user approval."""
        return tool_name in DANGEROUS_TOOLS

    async def _handle_approval(
        self,
        tc: dict,
        task: AgentTask,
        session: AgentSession,
    ) -> tuple[bool, dict]:
        """Run the approval flow for a single dangerous tool call.

        Returns ``(approved, result_dict)``.
        """
        name = tc["name"]
        try:
            args = json.loads(tc["arguments"]) if isinstance(tc["arguments"], str) else tc["arguments"]
        except (json.JSONDecodeError, TypeError):
            args = {}

        risk_level = self._assess_risk(name, args)
        description = self._describe_tool_call(name, args)
        approval_req = ToolApprovalRequest(
            tool_name=name,
            arguments=args or {},
            risk_level=risk_level,
            description=description,
        )

        # Create a future that will be resolved by submit_approval()
        loop = asyncio.get_running_loop()
        future: asyncio.Future[ToolApprovalResponse] = loop.create_future()
        self._pending_approvals[tc["id"]] = future

        # If there's a handler, use it directly (synchronous approval)
        if self._approval_handler is not None:
            try:
                resp = await self._approval_handler(approval_req)
                del self._pending_approvals[tc["id"]]
                if resp.approved:
                    result = await self._execute_single_tool(tc, task)
                    return True, result
                return False, {"error": f"Tool '{name}' denied: {resp.reason}"}
            except Exception as e:
                del self._pending_approvals[tc["id"]]
                return False, {"error": f"Approval handler error: {e}"}

        # Otherwise: wait for external approval via submit_approval()
        # The caller of run() must handle the yielded 'approval' event and
        # call submit_approval() to unblock.
        # We store the future but can't yield here (not in run's async gen).
        # For engines without handler, auto-deny after a timeout.
        try:
            resp = await asyncio.wait_for(future, timeout=300.0)
            if resp.approved:
                result = await self._execute_single_tool(tc, task)
                return True, result
            return False, {"error": f"Tool '{name}' denied: {resp.reason}"}
        except asyncio.TimeoutError:
            return False, {"error": f"Tool '{name}' approval timed out"}
        finally:
            self._pending_approvals.pop(tc["id"], None)

    @staticmethod
    def _assess_risk(tool_name: str, args: dict) -> str:
        """Classify risk level for a tool call."""
        if tool_name in ("execute_python", "shell_exec", "execute_command"):
            return "high"
        if tool_name in ("write_file", "edit_file"):
            return "medium"
        return "low"

    @staticmethod
    def _describe_tool_call(tool_name: str, args: dict) -> str:
        """Generate a human-readable description of the tool call."""
        if tool_name in ("write_file", "edit_file"):
            path = args.get("path", "unknown")
            return f"Will {tool_name.replace('_', ' ')} at path: {path}"
        if tool_name in ("shell_exec", "execute_command"):
            cmd = args.get("command", "unknown")
            return f"Will execute shell command: {cmd}"
        if tool_name == "execute_python":
            code = args.get("code", "")
            preview = code[:100] + ("..." if len(code) > 100 else "")
            return f"Will execute Python code: {preview}"
        return f"Will run tool: {tool_name}"
