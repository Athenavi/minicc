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
from app.gateway.router import GatewayRouter
from app.tools.registry import registry as local_tool_registry
logger = logging.getLogger(__name__)

# ── Token 节省参数 ──
MAX_CONTEXT_TOKENS = 4000        # 上下文预算（tokens），超出触发压缩
MAX_MESSAGES = 8                 # 消息数量硬上限（含 system），超出丢弃中间消息
SNIP_THRESHOLD = 0.60            # 60% → 压缩旧工具结果
PRUNE_THRESHOLD = 0.80           # 80% → 裁剪旧消息
TOOL_RESULT_MAX_CHARS = 2000     # 工具调用结果截断长度
TOOL_RESULT_HEAD = 400           # head 保留长度
TOOL_RESULT_TAIL = 200           # tail 保留长度

# ── 核心工具列表（Token Economy：只暴露这些给 LLM，其余按需激活） ──
CORE_TOOL_NAMES = frozenset({
    "recall",        # 检索记忆（用户偏好/事实）
    "remember",      # 保存新事实
    "skill_list",    # 列出可用技能
    "skill_run",     # 执行技能
    "web_fetch",     # 获取外部信息
    "shell_exec",    # 执行命令
    "execute_python",  # 执行 Python
    "git_status",    # 查看项目状态
    "read_file",     # 读取文件
    "grep_files",    # 搜索文件
})


def _normalize_msg(role: str, content: str = "", tool_call_id: str = "",
                   tool_calls: list | None = None, **extra) -> dict:
    """规范化消息：固定字段顺序 role → content → tool_call_id → tool_calls
    确保 byte-exact 一致的 JSON 序列化。
    """
    msg: dict = {"role": role}
    # tool_calls 存在时 content 必须为 None（OpenAI API 规范），否则用传入值
    if tool_calls:
        msg["content"] = None
    else:
        msg["content"] = content if content is not None else ""
    if tool_call_id and role == "tool":
        msg["tool_call_id"] = tool_call_id
    if extra:
        # 剥离只应由模型输出的字段，避免泄漏到后续请求中
        extra.pop("reasoning_content", None)
        if extra:
            msg.update(extra)
    if tool_calls:
        enriched = []
        for tc in tool_calls:
            enriched_tc = {"id": tc["id"], "type": "function", "function": tc.get("function", {})}
            enriched.append(enriched_tc)
        msg["tool_calls"] = enriched
    return msg


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

def _truncate_tool_result(result: dict) -> str:
    """截断过长的工具结果：保留 head + tail，中间用标记代替"""
    text = json.dumps(result, ensure_ascii=False, default=str)
    if len(text) <= TOOL_RESULT_MAX_CHARS:
        return text
    head = text[:TOOL_RESULT_HEAD]
    tail = text[-TOOL_RESULT_TAIL:]
    middle_len = len(text) - TOOL_RESULT_HEAD - TOOL_RESULT_TAIL
    return f"{head}...(truncated {middle_len} chars)...{tail}"


# ── 消息配对压缩 ──

def _snip_tool_results(messages: list[dict]) -> list[dict]:
    """Snip 阶段：压缩旧工具结果（保留 head + tail），user/assistant 消息不动"""
    result = []
    for msg in messages:
        if msg.get("role") == "tool" and msg.get("tool_call_id"):
            content = msg.get("content", "")
            if len(content) > TOOL_RESULT_MAX_CHARS // 2:
                content = _truncate_tool_result({"data": content})
            result.append({**msg, "content": content})
        else:
            result.append(msg)
    return result


def _prune_messages(messages: list[dict]) -> list[dict]:
    """Prune 阶段：不区分消息类型，保留系统提示 + 最近非系统消息
    
    策略：
    1. 工具结果 → 极短占位符
    2. user/assistant 配对 → 保留最近的几轮
    3. 不足时保留原始消息
    """
    # 先分离系统消息
    system_msgs = [m for m in messages if m.get("role") == "system"]
    other_msgs = [m for m in messages if m.get("role") != "system"]

    # 处理工具消息为占位符
    processed = []
    for m in other_msgs:
        if m.get("role") == "tool" and m.get("tool_call_id"):
            processed.append({
                "role": "tool",
                "tool_call_id": m["tool_call_id"],
                "content": "[tool_result:compressed]",
            })
        else:
            processed.append(m)

    # 保留最近的消息（不超过预算）
    budget = MAX_MESSAGES - len(system_msgs)
    if budget <= 0:
        return system_msgs

    kept = processed[-budget:]
    result = system_msgs + kept
    logger.info(
        "Prune: %d → %d messages (dropped %d old)",
        len(messages), len(result), len(processed) - len(kept),
    )
    return result


def _compact_messages(messages: list[dict]) -> list[dict]:
    """分级压缩：根据上下文压力和消息数量选择策略"""
    # 硬上限：无论 token 使用率多少，消息数不能超过 MAX_MESSAGES
    if len(messages) > MAX_MESSAGES:
        return _prune_messages(messages)

    # Token-based 压缩
    tokens = _estimate_tokens(messages)
    ratio = tokens / MAX_CONTEXT_TOKENS

    if ratio >= PRUNE_THRESHOLD:
        logger.info("Compact PRUNE at %.0f%% (%d/%d tokens)", ratio * 100, tokens, MAX_CONTEXT_TOKENS)
        return _prune_messages(messages)
    elif ratio >= SNIP_THRESHOLD:
        logger.info("Compact SNIP at %.0f%% (%d/%d tokens)", ratio * 100, tokens, MAX_CONTEXT_TOKENS)
        return _snip_tool_results(messages)

    return messages  # 无需压缩


def _ensure_valid_tool_sequence(messages: list[dict]) -> list[dict]:
    """修复消息序列中的工具调用配对，确保每个 tool 消息前都有带 tool_calls 的 assistant 消息。
    
    旧 session cache 中可能缓存了错误的格式（每个工具调用单独一条 assistant 消息），
    此函数删除孤立的 tool 消息以保证序列符合 API 要求。
    """
    result = []
    for msg in messages:
        if msg.get("role") == "tool":
            # tool 消息前面必须有带 tool_calls 的 assistant 消息
            if not result or result[-1].get("role") != "assistant" or not result[-1].get("tool_calls"):
                logger.warning("Dropping orphan tool message (no preceding assistant with tool_calls)")
                continue
        result.append(msg)
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
    
    async def run(self, task: AgentTask) -> AsyncIterator[AgentEvent]:
        """
        执行 Agent 推理循环
        
        Yields:
            AgentEvent: 推理事件（文本、工具调用、完成等）
        """
        start_time = time.time()
        total_input_tokens = 0
        total_output_tokens = 0
        
        try:
            # ── 1. 从 session cache 加载或初始化消息列表 ──
            if self._session_store and task.session_id:
                history_msgs = self._build_history_msgs(task)
                messages = self._session_store.get_or_init(task.session_id, history_msgs)
                # ── 修复旧 session cache 中可能存在的工具消息配对问题 ──
                messages = _ensure_valid_tool_sequence(messages)
                # 如果有新用户消息，追加到末尾（保持前缀稳定）
                if task.content:
                    messages.append(_normalize_msg(role="user", content=task.content))
            else:
                messages = self._build_messages(task)
            
            tools = self._convert_tools(task.tools) if task.tools else self._get_core_tools()
            
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
            messages = _compact_messages(messages)
            
            # 推理循环
            _thinking_last_flushed = 0  # 跨 turn 持久化，跟踪已向前端发送的 reasoning_content
            for turn in range(task.max_turns):
                logger.info("Agent turn %d/%d (task=%s, msgs=%d)", turn + 1, task.max_turns, task.id, len(messages))
                
                # ── 消息规范化：确保字段顺序一致（保护 prefix cache）──
                messages = [_normalize_msg(
                    role=m.get("role", "user"),
                    content=m.get("content", "") or "",
                    tool_call_id=m.get("tool_call_id", "") or "",
                    tool_calls=m.get("tool_calls"),
                ) for m in messages]
                
                # ── 分级压缩：根据 token 使用量选择压缩策略 ──
                messages = _compact_messages(messages)
                
                # ── 强制清理孤立的 tool 消息（确保 API 兼容性）──
                clean = []
                for m in messages:
                    if m.get("role") == "tool":
                        if not clean or clean[-1].get("role") != "assistant" or not clean[-1].get("tool_calls"):
                            logger.warning("Pre-call: dropping orphan tool msg (id=%s)", m.get("tool_call_id", "?"))
                            continue
                    clean.append(m)
                messages = clean

                # 调用 LLM
                response_content = ""
                reasoning_content = ""
                tool_calls = []

                async for chunk in self._gateway.chat_stream(
                    messages=messages,
                    model=model,
                    tenant_id=task.tenant_id,
                    max_tokens=max_tokens,
                    temperature=temperature,
                    tools=tools,
                ):
                    # DeepSeek thinking mode 思考过程
                    if chunk.reasoning_content:
                        reasoning_content += chunk.reasoning_content
                        # 累积 reasoning_content，在 content 开始前按 ~80 字为单位 yield
                        if not response_content:
                            new_len = len(reasoning_content)
                            if new_len - _thinking_last_flushed >= 80 or any(c in chunk.reasoning_content for c in "。！？\n"):
                                yield AgentEvent(
                                    type="text",
                                    content=f"[thinking]{reasoning_content[_thinking_last_flushed:]}[/thinking]",
                                )
                                _thinking_last_flushed = new_len

                    # 文本片段
                    if chunk.content:
                        if not response_content and not tool_calls:
                            # 推理阶段的内容合并到 reasoning_content，不直接输出
                            reasoning_content += chunk.content
                        else:
                            response_content += chunk.content
                            yield AgentEvent(
                                type="text",
                                content=chunk.content,
                            )
                    
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
                    
                    # 错误响应
                    if chunk.finish_reason == "error" and not chunk.content and not chunk.tool_calls:
                        yield AgentEvent(
                            type="error",
                            error="LLM provider unavailable or not configured",
                        )
                        break
                
                # 如果有工具调用，执行工具
                if tool_calls:
                    # 先追加一条 assistant 消息，包含所有 tool_calls（OpenAI API 格式要求）
                    all_tool_calls = [
                        {"id": tc["id"], "function": {"name": tc["name"], "arguments": tc["arguments"]}}
                        for tc in tool_calls
                    ]
                    tc_msg_kwargs = {
                        "role": "assistant", "content": "",
                        "tool_calls": all_tool_calls,
                    }
                    messages.append(_normalize_msg(**tc_msg_kwargs))

                    for tc in tool_calls:
                        # 执行工具
                        logger.info("Executing tool %s (id=%s)", tc.get("name"), tc.get("id"))
                        tool_result = await self._execute_tool(tc, task)

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
                    msg_kwargs = {"role": "assistant", "content": response_content}
                    if reasoning_content:
                        msg_kwargs["reasoning_content"] = reasoning_content
                    messages.append(_normalize_msg(**msg_kwargs))
                elif reasoning_content:
                    # 仅有思考内容（无 content 输出时），将思考内容作为最终回答
                    msg_kwargs = {"role": "assistant", "content": reasoning_content}
                    messages.append(_normalize_msg(**msg_kwargs))
                    yield AgentEvent(
                        type="text",
                        content=reasoning_content,
                    )
                break
            
            # ── 保存累积消息到 session cache（含工具调用消息，保持前缀稳定）──
            if self._session_store and task.session_id:
                self._session_store.append(task.session_id, messages)
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
    
    def _get_core_tools(self) -> list[dict] | None:
        """返回核心工具列表（Token Economy），其余工具按需激活"""
        from app.tools.registry import registry as local_tool_registry
        all_tools = local_tool_registry.to_openai_tools()
        core = [t for t in all_tools if t.get("function", {}).get("name") in CORE_TOOL_NAMES]
        logger.info("Core tools enabled: %d (total registered: %d)", len(core), len(all_tools))
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
