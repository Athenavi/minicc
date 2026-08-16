"""
Agent Runtime — 完整的 ReAct / Plan-and-Execute 推理循环
替代 Go 侧的 Agent 编排，成为 Python 数据面的核心
"""
from __future__ import annotations

import json
import logging
import time
from typing import AsyncIterator, Optional
from dataclasses import dataclass, field

from app.config import settings
from app.agent.modes import AgentMode, CORE_TOOL_NAMES, ModeConfig, get_mode_config
from app.gateway.router import GatewayRouter
from app.tools.registry import registry as local_tool_registry
logger = logging.getLogger(__name__)

# ── Token 节省参数（默认值；可被 CompactionConfig 覆盖，租户/模式可配） ──
MAX_CONTEXT_TOKENS = 4000        # 上下文预算（tokens），超出触发压缩
MAX_MESSAGES = 8                 # 消息数量硬上限（含 system），超出丢弃中间消息
SNIP_THRESHOLD = 0.60            # 60% → 压缩旧工具结果
PRUNE_THRESHOLD = 0.80           # 80% → 裁剪旧消息
TOOL_RESULT_MAX_CHARS = 8000     # 工具调用结果截断长度（2000 太小，read_file 内容常被截断成不可用片段）
TOOL_RESULT_HEAD = 1200          # head 保留长度
TOOL_RESULT_TAIL = 500           # tail 保留长度


@dataclass(frozen=True)
class CompactionConfig:
    """历史截断策略配置（SaaS：租户/模式可手动确认方式、大小、策略）。

    对应 deepseek-harness compaction-basic 的 thresholdRatio / retainTokens /
    maxTokens 语义，参数化后由 mode_overrides.json / 租户配置注入。

    - strategy: "auto"（按阈值分级）| "snipe"（只截断长工具结果）| "prune"（丢中间消息）| "none"
    - threshold_ratio: 触发压缩的上下文压力比例（0~1），对应 deepseek thresholdRatio
    - max_messages: 消息数量硬上限（含 system）
    - max_context_tokens: 上下文预算
    - tool_result_max_chars / head / tail: 工具结果截断参数
    """
    strategy: str = "auto"
    max_messages: int = MAX_MESSAGES
    max_context_tokens: int = MAX_CONTEXT_TOKENS
    threshold_ratio: float = PRUNE_THRESHOLD
    snipe_ratio: float = SNIP_THRESHOLD
    tool_result_max_chars: int = TOOL_RESULT_MAX_CHARS
    tool_result_head: int = TOOL_RESULT_HEAD
    tool_result_tail: int = TOOL_RESULT_TAIL


def _normalize_msg(role: str, content: str = "", tool_call_id: str = "",
                   tool_calls: list | None = None, **extra) -> dict:
    """规范化消息：中立格式（provider-agnostic，SaaS 架构决策）。

    内部一律使用中立格式 {role, content, tool_call_id, tool_calls:[{id,name,arguments}]}，
    到 gateway 边界才由 message_codec.to_chat_messages 适配具体提供商。
    """
    from app.agent.message_codec import make_message
    return make_message(role=role, content=content, tool_call_id=tool_call_id,
                        tool_calls=tool_calls, **extra)


def _to_chat_messages(messages: list[dict]):
    """中立格式 → gateway.ChatMessage（provider 边界，新增提供商在此适配）。"""
    from app.agent.message_codec import to_chat_messages
    return to_chat_messages(messages)


def _auto_normalize(messages: list[dict]) -> list[dict]:
    """自动推断消息格式并统一中立格式（OpenAI/Anthropic/Gemini/中立）。"""
    from app.agent.message_codec import auto_normalize
    return auto_normalize(messages)


def _estimate_tokens(messages: list[dict]) -> int:
    """Token 估算（4 chars ≈ 1 token）"""
    total = 0
    for m in messages:
        total += 4  # role overhead
        for v in m.values():
            if isinstance(v, str):
                total += len(v) // 4
            elif isinstance(v, list):
                total += len(json.dumps(v, ensure_ascii=False, default=str)) // 4
    # 消息间分隔符
    total += len(messages) * 2
    return total


# ── 工具结果截断（head + tail + middle indicator） ──

def _truncate_text(text: str, max_chars: int = TOOL_RESULT_MAX_CHARS,
                   head_chars: int = TOOL_RESULT_HEAD, tail_chars: int = TOOL_RESULT_TAIL) -> str:
    """截断过长文本：保留 head + tail，中间用标记代替"""
    if len(text) <= max_chars:
        return text
    head = text[:head_chars]
    tail = text[-tail_chars:]
    middle_len = len(text) - head_chars - tail_chars
    return f"{head}...(truncated {middle_len} chars)...{tail}"


def _truncate_tool_result(result: dict, cfg: CompactionConfig | None = None) -> str:
    """截断过长的工具结果：保留 head + tail，中间用标记代替。

    S 安全修复：进上下文前先过输出清洗（宿主路径/secret 替换），
    防止工具输出把宿主文件结构泄露给模型/用户。
    """
    from app.agent.guards import OutputGuard
    text = json.dumps(result, ensure_ascii=False, default=str)
    text = OutputGuard(max_hits=10_000).sanitize(text)
    if cfg is not None:
        return _truncate_text(text, cfg.tool_result_max_chars, cfg.tool_result_head, cfg.tool_result_tail)
    return _truncate_text(text)


# ── 消息配对压缩 ──

def _snip_tool_results(messages: list[dict], cfg: CompactionConfig | None = None) -> list[dict]:
    """Snip 阶段：压缩旧工具结果（保留 head + tail），user/assistant 消息不动"""
    cfg = cfg or CompactionConfig()
    result = []
    for msg in messages:
        if msg.get("role") == "tool" and msg.get("tool_call_id"):
            content = msg.get("content", "")
            if len(content) > cfg.tool_result_max_chars // 2:
                content = _truncate_text(content, cfg.tool_result_max_chars, cfg.tool_result_head, cfg.tool_result_tail)
            result.append({**msg, "content": content})
        else:
            result.append(msg)
    return result


def _prune_messages(messages: list[dict], cfg: CompactionConfig | None = None) -> list[dict]:
    """Prune 阶段：不区分消息类型，保留系统提示 + 最近非系统消息

    策略：
    1. 工具结果 → 截断到 head + tail（保留可用信息，避免 AI 丢失上下文）
    2. user/assistant 配对 → 保留最近的几轮
    3. 不足时保留原始消息
    """
    cfg = cfg or CompactionConfig()
    # 先分离系统消息
    system_msgs = [m for m in messages if m.get("role") == "system"]
    other_msgs = [m for m in messages if m.get("role") != "system"]

    # 处理工具消息：截断而非占位符（占位符会让 AI 丢失工具结果上下文）
    processed = []
    for m in other_msgs:
        if m.get("role") == "tool" and m.get("tool_call_id"):
            processed.append({
                "role": "tool",
                "tool_call_id": m["tool_call_id"],
                "content": _truncate_text(m.get("content", "") or "",
                                          cfg.tool_result_max_chars, cfg.tool_result_head, cfg.tool_result_tail),
            })
        else:
            processed.append(m)

    # 保留最近的消息（不超过预算）
    budget = cfg.max_messages - len(system_msgs)
    if budget <= 0:
        return system_msgs

    # 从最近位置往前取 budget 条；若 kept 中存在孤儿 tool 消息
    # （前面不是带 tool_calls 的 assistant），向前扩展到其配对的
    # assistant(tool_calls)，保证 API 配对完整、工具结果不丢失
    start = max(0, len(processed) - budget)
    # 起点本身是 tool 时，其配对的 assistant 可能在 kept 之外，先补一位
    if start < len(processed) and processed[start].get("role") == "tool" and start > 0:
        start -= 1
    while True:
        need = start
        for idx in range(start, len(processed)):
            m = processed[idx]
            if m.get("role") == "tool":
                prev_ok = (
                    idx > start
                    and processed[idx - 1].get("role") == "assistant"
                    and processed[idx - 1].get("tool_calls")
                )
                if not prev_ok:
                    # 向前找配对的 assistant(tool_calls)
                    j = idx - 1
                    while j >= 0 and not (processed[j].get("role") == "assistant" and processed[j].get("tool_calls")):
                        j -= 1
                    if j >= 0 and j < need:
                        need = j
        if need == start:
            break
        start = need
    kept = processed[start:]
    result = system_msgs + kept
    logger.info(
        "Prune: %d → %d messages (dropped %d old)",
        len(messages), len(result), len(processed) - len(kept),
    )
    return result


def _compact_messages(messages: list[dict], cfg: CompactionConfig | None = None) -> list[dict]:
    """分级压缩：根据配置的策略、上下文压力和消息数量选择压缩方式

    策略（cfg.strategy）：
    - "none"   → 不压缩
    - "snipe"  → 只截断长工具结果（保留全部消息）
    - "prune"  → 只裁剪旧消息（保留系统 + 最近 N 条）
    - "auto"   → 默认：按阈值分级（SNIP → PRUNE）+ 消息数硬上限
    """
    cfg = cfg or CompactionConfig()
    if cfg.strategy == "none":
        return messages
    if cfg.strategy == "snipe":
        return _snip_tool_results(messages, cfg)
    if cfg.strategy == "prune":
        return _prune_messages(messages, cfg)

    # auto：硬上限（无论 token 使用率多少，消息数不能超过 cfg.max_messages）
    if len(messages) > cfg.max_messages:
        return _prune_messages(messages, cfg)

    # Token-based 压缩
    tokens = _estimate_tokens(messages)
    ratio = tokens / cfg.max_context_tokens if cfg.max_context_tokens else 0

    if ratio >= cfg.threshold_ratio:
        logger.info("Compact PRUNE at %.0f%% (%d/%d tokens)", ratio * 100, tokens, cfg.max_context_tokens)
        return _prune_messages(messages, cfg)
    elif ratio >= cfg.snipe_ratio:
        logger.info("Compact SNIP at %.0f%% (%d/%d tokens)", ratio * 100, tokens, cfg.max_context_tokens)
        return _snip_tool_results(messages, cfg)

    return messages  # 无需压缩


def _ensure_valid_tool_sequence(messages: list[dict]) -> list[dict]:
    """修复消息序列中的工具调用配对，保证 OpenAI/DeepSeek API 兼容。

    双向修复：
    1. 孤立 tool 消息（无前导 assistant(tool_calls)）→ 删除
    2. **部分配对的 assistant(tool_calls)**（中断/审批等待时缓存里
       assistant([c1,c2]) 只有 tool(c1)，缺 c2）→ 从未配对的 id 从
       assistant.tool_calls 中移除（保留 content），否则 API 报
       "assistant message with tool_calls must be followed by tool messages"
    """
    result = []
    pending_tc: dict | None = None   # 暂存的 assistant(tool_calls)
    pending_ids: set = set()         # 其声明的 tool_call_id
    answered_ids: set = set()        # 已收到 tool 结果的 id
    pending_tools: list[dict] = []   # 匹配的 tool 消息（flush 时排在 assistant 后）

    def _flush_pending() -> None:
        nonlocal pending_tc
        if pending_tc is None:
            return
        missing = pending_ids - answered_ids
        if missing:
            logger.warning("Dropping unmatched tool_calls %s (no tool result)", sorted(missing))
            pending_tc["tool_calls"] = [tc for tc in pending_tc["tool_calls"] if tc.get("id") in answered_ids]
        result.append(pending_tc)
        result.extend(pending_tools)
        pending_tc = None
        pending_tools.clear()

    for msg in messages:
        role = msg.get("role")
        if role == "assistant" and msg.get("tool_calls"):
            _flush_pending()
            pending_tc = dict(msg)
            pending_ids = {tc.get("id") for tc in msg["tool_calls"]}
            answered_ids = set()
            pending_tools = []
            continue
        if role == "tool" and pending_tc is not None and msg.get("tool_call_id") in pending_ids:
            answered_ids.add(msg.get("tool_call_id"))
            pending_tools.append(msg)
            continue
        if role == "tool":
            # 孤立 tool 消息（无前导 assistant(tool_calls)）
            logger.warning("Dropping orphan tool message (no preceding assistant with tool_calls)")
            continue
        _flush_pending()
        result.append(msg)
    _flush_pending()
    return result


@dataclass
class AgentTask:
    """Agent 任务定义"""
    id: str
    tenant_id: str
    user_id: str
    session_id: str
    content: str
    system_prompt: str = ""
    history: list[dict] = field(default_factory=list)
    tools: list[dict] = field(default_factory=list)
    llm_config: dict = field(default_factory=dict)
    max_turns: int = 5
    subagent_depth: int = 0  # S3: 委派深度（subagent 递归限制，MAX_DEPTH=3）
    
    @classmethod
    def parse(cls, data: dict) -> "AgentTask":
        """从字典解析任务"""
        return cls(
            id=data.get("task_id", ""),
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
    """Agent 事件"""
    type: str
    content: str = ""
    tool_call_id: str = ""
    tool_name: str = ""
    tool_arguments: str = ""
    input_tokens: int = 0
    output_tokens: int = 0
    error: str = ""
    timestamp: float = field(default_factory=time.time)


class AgentRuntime:
    """
    Agent 运行时 — 完整的推理循环
    
    职责：
    1. 消费 Redis Stream 任务
    2. 调用 LLM Gateway 执行推理（利用 session cache 保持前缀稳定）
    3. 选择并执行工具
    4. 管理推理状态和轮次
    5. 上报 Token 用量给 Go 计费
    """
    
    def __init__(
        self,
        gateway: GatewayRouter,
        tool_executor=None,
        sse_producer=None,
        session_store=None,
    ):
        self._gateway = gateway
        self._tool_executor = tool_executor
        self._sse = sse_producer
        self._session_store = session_store
        # 三栅栏（S 安全修复：输入/工具/输出）
        from app.agent.guards import InputGuard, OutputGuard, ToolGuard
        self._input_guard = InputGuard()
        self._tool_guard = ToolGuard()
        self._output_guard = OutputGuard(max_hits=3)
        # 待确认工具调用 future（外部经 submit_approval 解决）
        self._pending_approvals: dict[str, asyncio.Future] = {}
    
    async def run(self, task: AgentTask) -> AsyncIterator[AgentEvent]:
        """
        执行 Agent 推理循环
        
        Yields:
            AgentEvent: 推理事件（文本、工具调用、完成等）
        """
        start_time = time.time()
        total_input_tokens = 0
        total_output_tokens = 0

        # ── 0. 输入栅栏：注入检测（S 安全修复）────────────────────────────
        injection = self._input_guard.check(task.content)
        if injection:
            logger.warning("Input guard blocked (task=%s) pattern=%s", task.id, injection)
            yield AgentEvent(type="guardrail_blocked", content="输入包含不允许的指令，已拒绝本次请求")
            return
        
        try:
            # ── 0. 解析运行模式（persona/工具集/上下文/压缩策略） ──
            mode_cfg: ModeConfig = get_mode_config((task.llm_config or {}).get("mode"))
            if mode_cfg.persona:
                # 固定完整 persona（minimal/creative）：覆盖外部传入的 system_prompt
                task.system_prompt = mode_cfg.persona

            # ── 0.5 设置工具执行上下文（持久终端/子 agent 等需要 session 与网关） ──
            from app.tools.context import set_tool_context
            set_tool_context(
                session_id=task.session_id,
                user_id=task.user_id,
                tenant_id=task.tenant_id,
                gateway=self._gateway,
                subagent_depth=task.subagent_depth,
            )

            # ── 1. 从 session cache 加载或初始化消息列表 ──
            if self._session_store and task.session_id:
                history_msgs = self._build_history_msgs(task)
                messages = await self._session_store.get_or_init(task.session_id, history_msgs)
                # ── 自动推断消息格式并统一中立（SaaS：OpenAI/Anthropic/Gemini 来源均可消费） ──
                messages = _auto_normalize(messages)
                # ── 修复消息序列中的工具消息配对问题 ──
                messages = _ensure_valid_tool_sequence(messages)
                # 如果有新用户消息，追加到末尾（保持前缀稳定）
                if task.content:
                    messages.append(_normalize_msg(role="user", content=task.content))
            else:
                messages = self._build_messages(task)

            # ── 1.5 技能持久目录注入（含技能的模式；会话内已注入则跳过） ──
            if mode_cfg.include_context:
                from app.tools.skill_catalog import inject_skill_catalog
                messages = await inject_skill_catalog(messages)
            
            tools = self._convert_tools(task.tools) if task.tools else self._get_core_tools(mode_cfg)
            
            # 获取 LLM 配置
            llm_config = task.llm_config or {}
            model = llm_config.get("model", settings.default_model)
            max_tokens = llm_config.get("max_tokens", settings.default_max_tokens)
            temperature = llm_config.get("temperature", settings.default_temperature)
            
            # ── 消息规范化 + 分级压缩：请求开始时立即执行，确保上下文在预算内 ──
            messages = [_normalize_msg(
                role=m.get("role", "user"),
                content=m.get("content", "") or "",
                tool_call_id=m.get("tool_call_id", "") or "",
                tool_calls=m.get("tool_calls"),
            ) for m in messages]
            if mode_cfg.enable_compaction:
                # SaaS：截断策略由模式/租户配置（mode_overrides.json 的 compaction 字段）
                comp_cfg = CompactionConfig(**mode_cfg.compaction) if mode_cfg.compaction else None
                messages = _compact_messages(messages, comp_cfg)
            
            # 推理循环
            _thinking_last_flushed = 0  # 跨 turn 持久化，跟踪已向前端发送的 reasoning_content
            _last_reasoning = ""  # 保存最后轮次的思考内容，用于兜底输出
            _answered = False  # 是否已产生最终回答
            _cache_saved = False  # S 修复：缓存是否已保存（finally 兜底）
            for turn in range(task.max_turns):
                logger.info("Agent turn %d/%d (task=%s, msgs=%d)", turn + 1, task.max_turns, task.id, len(messages))
                
                # ── 消息规范化：确保字段顺序一致（保护 prefix cache）──
                messages = [_normalize_msg(
                    role=m.get("role", "user"),
                    content=m.get("content", "") or "",
                    tool_call_id=m.get("tool_call_id", "") or "",
                    tool_calls=m.get("tool_calls"),
                ) for m in messages]
                
                # ── 分级压缩：根据 token 使用量选择压缩策略（SaaS：策略可配）──
                if mode_cfg.enable_compaction:
                    comp_cfg = CompactionConfig(**mode_cfg.compaction) if mode_cfg.compaction else None
                    messages = _compact_messages(messages, comp_cfg)
                
                # ── 强制清理孤立的 tool 消息（确保 API 兼容性）──
                clean = []
                last_tc = False  # 最近一条 assistant 是否有 tool_calls
                for m in messages:
                    if m.get("role") == "tool":
                        if not last_tc:
                            logger.warning("Pre-call: dropping orphan tool msg (id=%s)", m.get("tool_call_id", "?"))
                            continue
                    clean.append(m)
                    if m.get("role") == "assistant":
                        last_tc = bool(m.get("tool_calls"))
                messages = clean

                # 调用 LLM
                response_content = ""
                reasoning_content = ""
                tool_calls = []
                has_reasoned = False  # 是否已收到 native reasoning_content（DeepSeek 模式）

                async for chunk in self._gateway.chat_stream(
                    # 中立格式 → gateway ChatMessage（provider 边界适配）
                    messages=_to_chat_messages(messages),
                    model=model,
                    tenant_id=task.tenant_id,
                    max_tokens=max_tokens,
                    temperature=temperature,
                    tools=tools,
                ):
                    # DeepSeek thinking mode 思考过程
                    if chunk.reasoning_content:
                        reasoning_content += chunk.reasoning_content
                        has_reasoned = True
                        # 累积 reasoning_content，在 content 开始前按 ~80 字为单位 yield
                        if not response_content:
                            new_len = len(reasoning_content)
                            if new_len - _thinking_last_flushed >= 80 or any(c in chunk.reasoning_content for c in "。！？\n"):
                                safe_thinking = self._output_guard.sanitize(reasoning_content[_thinking_last_flushed:])
                                yield AgentEvent(
                                    type="text",
                                    content=f"[thinking]{safe_thinking}[/thinking]",
                                )
                                _thinking_last_flushed = new_len

                    # 文本片段
                    if chunk.content:
                        safe = self._output_guard.sanitize(chunk.content)
                        if not safe:
                            chunk.content = ""
                        if has_reasoned:
                            # 已有 native reasoning_content → content 是实际回答
                            response_content += chunk.content
                            yield AgentEvent(
                                type="text",
                                content=safe,
                            )
                        elif not response_content and not tool_calls:
                            # 无 reasoning_content，无工具 → 兼容模式：首段内容可能含 [thinking]
                            reasoning_content += chunk.content
                        else:
                            response_content += chunk.content
                            yield AgentEvent(
                                type="text",
                                content=safe,
                            )
                        if self._output_guard.blocked:
                            logger.warning("Output guard blocked (task=%s): repeated host-path/secret leak", task.id)
                            yield AgentEvent(type="guardrail_blocked", content="输出包含敏感路径，已截断")
                            break
                    
                    # 工具调用
                    if chunk.tool_calls:
                        for tc in chunk.tool_calls:
                            tool_calls.append({
                                "id": tc.id,
                                "name": tc.name,
                                "arguments": tc.arguments,
                            })
                            yield AgentEvent(
                                type="tool_call",
                                tool_call_id=tc.id,
                                tool_name=tc.name,
                                tool_arguments=tc.arguments,
                            )
                    
                    # Token 用量
                    if chunk.input_tokens or chunk.output_tokens:
                        total_input_tokens += chunk.input_tokens
                        total_output_tokens += chunk.output_tokens
                    
                    # 错误响应（透出真实原因；旧消息作兜底）
                    if chunk.finish_reason == "error" and not chunk.content and not chunk.tool_calls:
                        yield AgentEvent(
                            type="error",
                            error=chunk.message or "LLM provider unavailable or not configured",
                        )
                        break
                
                # 如果有工具调用，执行工具
                if tool_calls:
                    _last_reasoning = reasoning_content  # 保存思考内容供后续兜底
                    # 先追加一条 assistant 消息，包含所有 tool_calls（OpenAI API 格式要求）
                    all_tool_calls = [
                        {"id": tc["id"], "function": {"name": tc["name"], "arguments": tc["arguments"]}}
                        for tc in tool_calls
                    ]
                    tc_msg_kwargs = {
                        "role": "assistant", "content": response_content or "",
                        "tool_calls": all_tool_calls,
                    }
                    messages.append(_normalize_msg(**tc_msg_kwargs))

                    for tc in tool_calls:
                        # 工具栅栏：三态裁决（S 安全修复）——block/confirm/allow
                        tool_result, approval_evt = await self._guarded_execute_tool(tc, task)
                        if approval_evt is not None:
                            # 转发用户确认事件（前端展示确认卡片，回调 /v1/agent/approval）
                            yield approval_evt

                        yield AgentEvent(
                            type="tool_result",
                            tool_call_id=tc["id"],
                            tool_name=tc["name"],
                            content=json.dumps(tool_result, ensure_ascii=False),
                        )

                        # tool 结果消息
                        truncated = _truncate_tool_result(tool_result)
                        messages.append(_normalize_msg(
                            role="tool", content=truncated,
                            tool_call_id=tc["id"],
                        ))

                    logger.info("Tool calls processed: %d tools, total msgs=%d", len(tool_calls), len(messages))
                    
                    # 继续推理
                    continue
                
                # 无工具调用，推理完成
                if response_content:
                    _answered = True
                    msg_kwargs = {"role": "assistant", "content": response_content}
                    if reasoning_content:
                        msg_kwargs["reasoning_content"] = reasoning_content
                    messages.append(_normalize_msg(**msg_kwargs))
                elif reasoning_content:
                    _answered = True
                    # 仅有思考内容（无 content 输出时），将思考内容作为最终回答
                    msg_kwargs = {"role": "assistant", "content": reasoning_content}
                    messages.append(_normalize_msg(**msg_kwargs))
                    yield AgentEvent(
                        type="text",
                        content=reasoning_content,
                    )
                break
            
            # ── 兜底：循环用尽但未产生回答时，输出最后的思考内容 ──
            if not _answered and _last_reasoning:
                logger.warning("Agent loop exhausted without final answer, using reasoning as fallback")
                yield AgentEvent(
                    type="text",
                    content=_last_reasoning,
                )
                # 仅当 reasoning 尚未作为消息保存时才追加
                if not messages or messages[-1].get("role") != "assistant":
                    msg_kwargs = {"role": "assistant", "content": _last_reasoning}
                    messages.append(_normalize_msg(**msg_kwargs))
            
            # ── 保存累积消息到 session cache（含工具调用消息，保持前缀稳定）──
            if self._session_store and task.session_id:
                await self._session_store.append(task.session_id, messages)
                _cache_saved = True
                logger.info("Session cache saved: %s (%d messages)", task.session_id, len(messages))
            
            # 发送完成事件
            yield AgentEvent(
                type="done",
                input_tokens=total_input_tokens,
                output_tokens=total_output_tokens,
            )
            
        except Exception as e:
            logger.error("Agent runtime error (task=%s): %s", task.id, e)
            yield AgentEvent(
                type="error",
                error=str(e),
            )
        finally:
            # S 修复：上下文丢失 — 任何退出路径（异常/SSE 中断/GeneratorExit）都保存缓存，
            # 保证"继续"时历史可续（用户消息不丢）
            if not _cache_saved and self._session_store and task.session_id and messages:
                try:
                    await self._session_store.append(task.session_id, messages)
                    logger.info("Session cache saved on exit: %s (%d messages)", task.session_id, len(messages))
                except Exception:
                    logger.exception("Session cache save on exit failed")
    
    def _build_messages(self, task: AgentTask) -> list[dict]:
        """构建 LLM 消息列表（完整路径：system + history + 当前用户消息）"""
        messages = []
        if task.system_prompt:
            messages.append(_normalize_msg(role="system", content=task.system_prompt))
        for msg in task.history:
            messages.append(_normalize_msg(
                role=msg.get("role", "user"),
                content=msg.get("content", ""),
                tool_call_id=msg.get("tool_call_id", ""),
                tool_calls=msg.get("tool_calls"),
            ))
        if task.content:
            messages.append(_normalize_msg(role="user", content=task.content))
        return messages
    
    def _build_history_msgs(self, task: AgentTask) -> list[dict]:
        """构建仅含历史的消息列表（不含当前用户消息，供 session cache 使用）"""
        messages = []
        if task.system_prompt:
            messages.append(_normalize_msg(role="system", content=task.system_prompt))
        for msg in task.history:
            messages.append(_normalize_msg(
                role=msg.get("role", "user"),
                content=msg.get("content", ""),
                tool_call_id=msg.get("tool_call_id", ""),
                tool_calls=msg.get("tool_calls"),
            ))
        return messages
    
    def _get_core_tools(self, mode_cfg: ModeConfig | None = None) -> list[dict] | None:
        """按模式过滤返回工具列表（Token Economy：只暴露模式允许的工具）。

        mode_cfg 省略时用 NORMAL；过滤后为空（如极简模式注册不全）回退全量核心工具。
        """
        from app.tools.registry import registry as local_tool_registry
        if mode_cfg is None:
            mode_cfg = get_mode_config(None)
        all_tools = local_tool_registry.to_openai_tools()
        allowed = mode_cfg.include_tools | mode_cfg.extra_tools
        core = [t for t in all_tools if t.get("function", {}).get("name") in allowed]
        if not core and mode_cfg.mode is AgentMode.MINIMAL:
            core = [t for t in all_tools if t.get("function", {}).get("name") in CORE_TOOL_NAMES]
        logger.info(
            "Core tools (%s mode): %d (total registered: %d)",
            mode_cfg.mode.value, len(core), len(all_tools),
        )
        return core if core else None

    def _convert_tools(self, tools: list[dict]) -> list[dict]:
        """将工具定义转换为 OpenAI function 格式"""
        converted = []
        for tool in tools:
            converted.append({
                "type": "function",
                "function": {
                    "name": tool.get("name", ""),
                    "description": tool.get("description", ""),
                    "parameters": json.loads(tool.get("parameters_json", "{}")) if isinstance(tool.get("parameters_json"), str) else tool.get("parameters", {}),
                },
            })
        return converted
    
    async def _guarded_execute_tool(self, tool_call: dict, task: AgentTask) -> tuple[dict, AgentEvent | None]:
        """工具栅栏三态执行：block（拒绝）/ confirm（等待用户确认）/ allow（直接执行）。

        返回 ``(tool_result, approval_event_or_None)``——调用方需 yield approval 事件（SSE）。
        """
        tool_name = tool_call["name"]
        try:
            targs = json.loads(tool_call["arguments"]) if isinstance(tool_call["arguments"], str) else tool_call["arguments"]
        except (json.JSONDecodeError, TypeError):
            targs = {}
        verdict = self._tool_guard.evaluate(tool_name, targs or {})
        if verdict.action == "block":
            logger.warning("Tool guard blocked %s reason=%s", tool_name, verdict.reason)
            return {"error": f"Tool '{tool_name}' blocked by guard: {verdict.reason}"}, None
        if verdict.action == "confirm":
            # 请求用户确认：返回确认事件并等待外部 submit_approval 解决
            tc_id = tool_call.get("id") or tool_name
            loop = asyncio.get_running_loop()
            future: asyncio.Future[bool] = loop.create_future()
            self._pending_approvals[tc_id] = future
            approval_evt = AgentEvent(
                type="approval",
                tool_call_id=tc_id,
                tool_name=tool_name,
                tool_arguments=json.dumps(targs, ensure_ascii=False),
                content=f"请求执行 {tool_name}",
            )
            try:
                approved = await asyncio.wait_for(future, timeout=300.0)
            except asyncio.TimeoutError:
                return {"error": f"Tool '{tool_name}' approval timed out"}, approval_evt
            finally:
                self._pending_approvals.pop(tc_id, None)
            if not approved:
                return {"error": f"Tool '{tool_name}' denied by user"}, approval_evt
        # allow / 已确认：正常执行
        logger.info("Executing tool %s (id=%s)", tool_name, tool_call.get("id"))
        return await self._execute_tool(tool_call, task), None

    async def submit_approval(self, tool_call_id: str, approved: bool, reason: str = "") -> bool:
        """外部（HTTP 端点）解决待确认的工具调用。返回是否已解决。"""
        future = self._pending_approvals.get(tool_call_id)
        if future is None or future.done():
            return False
        future.set_result(approved)
        return True

    async def _execute_tool(self, tool_call: dict, task: AgentTask) -> dict:
        """执行工具"""
        tool_name = tool_call["name"]
        tool_arguments = tool_call["arguments"]
        
        # 解析参数
        try:
            params = json.loads(tool_arguments) if isinstance(tool_arguments, str) else tool_arguments
        except json.JSONDecodeError:
            params = {"raw": tool_arguments}
        
        # 优先走本地 Python 工具注册表
        if local_tool_registry.get(tool_name) is not None:
            try:
                return await local_tool_registry.execute(tool_name, params or {})
            except Exception as e:
                logger.error("Local tool execution failed (%s): %s", tool_name, e)
                return {"error": str(e)}
        
        # 如果有外部工具执行器，使用它（兼容旧 Go 调用链）
        if self._tool_executor:
            try:
                result = await self._tool_executor.execute(
                    tool_name=tool_name,
                    params=params,
                    tenant_id=task.tenant_id,
                    user_id=task.user_id,
                )
                return result
            except Exception as e:
                logger.error("Tool execution failed (%s): %s", tool_name, e)
                return {"error": str(e)}
        
        # 默认返回未实现
        return {"error": f"Tool '{tool_name}' not implemented"}


async def run_agent(
    gateway: GatewayRouter,
    system_prompt: str,
    history: list[dict],
    content: str,
    tools: list[dict] = None,
    llm_config: dict = None,
    max_turns: int = None,
    tenant_id: str = "",
    provider_hint: str = "",
) -> AsyncIterator[dict]:
    """
    兼容旧接口的 Agent 推理函数
    """
    # 创建临时任务
    task = AgentTask(
        id=f"temp_{int(time.time())}",
        tenant_id=tenant_id,
        user_id="",
        session_id="",
        content=content,
        system_prompt=system_prompt,
        history=history,
        tools=tools or [],
        llm_config=llm_config or {},
        max_turns=max_turns or settings.max_turns,
    )
    
    # 创建运行时
    runtime = AgentRuntime(gateway=gateway)
    
    # 执行并转换事件格式
    async for event in runtime.run(task):
        yield {
            "type": event.type,
            "content": event.content,
            "id": event.tool_call_id,
            "name": event.tool_name,
            "arguments": event.tool_arguments,
            "input_tokens": event.input_tokens,
            "output_tokens": event.output_tokens,
            "message": event.error,
        }
