# 任务 05：查询引擎 QueryEngine（主循环编排层）

> **所属阶段**：Phase 1 - 会话主循环与上下文装配 (The Core Loop)
> **对应模块**：模块 2 (Loop/Agent) — MiniCC 的灵魂
> **预估工时**：4-5 天
> **依赖**：任务 01 (Pydantic 模型)、任务 03 (WebSocket)、任务 04 (ContextBuilder)

---

## 1. 任务目标

实现 MiniCC 的核心主循环 QueryEngine——这是整个项目的灵魂组件。它负责接收用户输入、组装上下文、驱动 LLM 调用、处理工具调用中间结果、维护会话状态，直到任务完成。

**核心参考**：Claude Code 的 `QueryEngine.ts` 设计哲学——“一个会话一个 QueryEngine，它不是请求处理器，而是会话级运行时的任务编排器。”

## 2. 详细子任务

### 2.1 QueryEngine 类设计

- [ ] 文件：`backend/app/engine/query_engine.py`

```python
class QueryEngine:
    """
    会话级主循环编排器。
    对应 Claude Code 的 QueryEngine.ts — 管理的是"会话"，不是"一次请求"。
    
    核心职责：
    - 接收用户输入
    - 调用 ContextBuilder 装配上下文
    - 驱动 LLM 调用（流式）
    - 拦截并路由工具调用
    - 将工具结果回流到消息历史
    - 管理会话级别的状态缓存
    """
    
    def __init__(
        self,
        session_id: str,
        workspace_dir: str,
        tool_registry: ToolRegistry,
        context_builder: ContextBuilder,
        permission_handler: PermissionHandler,  # Phase 2 注入
        message_queue: asyncio.Queue,           # WebSocket 输出队列
    ):
        self.session_id = session_id
        self.mutable_messages: list[Message] = []   # 跨轮次消息历史
        self.abort_event = asyncio.Event()           # 中断信号
        self.permission_denials: dict[str, bool] = {} # 权限记忆
        self.total_usage: dict = {}                  # Token 用量统计
        
    async def submit_message(self, content: str) -> AsyncGenerator[SDKMessage, None]:
        """一次完整的任务提交——对应 Claude Code 的 submitMessage()"""
        ...
```

### 2.2 LLM Provider 抽象层

- [ ] 文件：`backend/app/engine/llm_provider.py`
- [ ] `class LLMProvider(ABC)`:
  - `send_message(messages, tools, system_prompt) -> AsyncGenerator[StreamEvent]`
  - 支持流式和非流式两种模式
- [ ] `class AnthropicProvider(LLMProvider)`:
  - 使用 Anthropic Python SDK
  - 支持 Messages API 流式
  - 支持 tool_use content block
  - 支持 thinking/扩展思考（Claude 4 特性）
- [ ] `class OpenAIProvider(LLMProvider)`:
  - 兼容 OpenAI SDK
  - 支持 function calling
  - 可对接本地 Ollama（OpenAI 兼容模式）
- [ ] `class StreamEvent(BaseModel)`:
  - `type`: Literal["text", "tool_use", "tool_result", "end", "error"]
  - `data`: dict

### 2.3 主循环逻辑（`submit_message` 核心流程）

```
submit_message(content):
  1. 将用户消息追加到 mutable_messages
  2. 调用 ContextBuilder.build_context() 获取 System Prompt
  3. 从 ToolRegistry 获取 tools 定义（LLM 格式）
  4. 进入主循环:
    4a. 调用 LLM (流式)
    4b. 对每个事件:
        - text → 通过 Queue 发送 message_chunk
        - tool_use → 挂起，进入工具调用流程
    4c. 如果 LLM 返回 tool_use:
        - 调用 PermissionSystem (Phase 2)
        - 等待权限结果（可中断）
        - 执行工具
        - 将 tool_result 追加为消息
        - 回到步骤 4a（继续循环）
    4d. 如果 LLM 返回文本结束 → 完成
  5. 发送 message_complete
  6. 更新 usage 统计
```

### 2.4 提示词模板设计

- [ ] System Prompt Template（集成 ContextBuilder 输出 + 工具定义 + 行为规则）
- [ ] 内置系统级指令：
  - 工具调用格式规范
  - 审批流程须知
  - 输出格式要求
  - 安全边界提示
- [ ] 可扩展点：允许通过 `.minicc/rules.md` 追加自定义系统指令

### 2.5 流式消息转发

- [ ] 将 LLM 的流式输出实时转发到 WebSocket Queue
- [ ] 文本块按 token 粒度或句子粒度分块（避免单字刷屏）
- [ ] 支持 tool_use 块的流式渲染（显示正在调用的工具名称）
- [ ] 异常处理：LLM 调用失败时发送 `error` 消息

### 2.6 错误处理与边界情况

- [ ] LLM API 调用失败 → 重试策略（指数退避，最多 3 次）
- [ ] 上下文超长 → 触发 Context Window 管理（Phase 5 实现）
- [ ] 工具调用超时 → 标记为失败，继续下一轮
- [ ] 用户中途取消 → 设置 `abort_event`，协程协作取消
- [ ] API Key 无效 → 明确的错误提示

### 2.7 单元测试与集成测试

- [ ] Mock LLM Provider 测试主循环流程
- [ ] 测试带工具调用的完整回合
- [ ] 测试中断取消
- [ ] 测试 LLM 故障恢复

## 3. 验收标准

| # | 检查项 | 验证方式 |
|:-|:-|:-|
| 1 | 用户消息 → LLM 响应 → 前端渲染的完整链路跑通 | 端到端测试 |
| 2 | LLM 流式文本实时出现在前端 | 观察前端逐字渲染 |
| 3 | 工具调用被正确拦截并触发权限流程 | 集成测试 (Phase 2 联调) |
| 4 | 多次消息的会话历史正确累积 | 测试 `mutable_messages` |
| 5 | API 调用失败时正确重试 3 次后报错 | mock 测试 |
| 6 | 用户取消时主循环 1s 内停止 | 集成测试 |
| 7 | 支持切换 Anthropic / OpenAI / Ollama 三种 provider | 配置切换测试 |

## 4. 参考资源

- [Claude Code QueryEngine 源码解析](https://www.xuanyuancode.com/learn-claude-code/tutorials/cc5) — 核心参考
- [Anthropic Messages API](https://docs.anthropic.com/en/api/messages)
- [OpenAI Chat Completions API](https://platform.openai.com/docs/api-reference/chat)
- 规划文档 §3 Phase 1 任务 1.2、§5 核心数据流
- claw-code 的 `query_engine` 模块设计理念

## 5. 注意事项

- **一个会话一个 QueryEngine 实例**，不要每次请求重新创建
- `mutable_messages` 是跨轮次状态，注意深拷贝 vs 引用的问题
- 流式输出的 `message_chunk` 必须包含 `index` 字段，前端据此拼接
- 工具调用的中间结果必须**立即回流**到消息历史，否则 LLM 无法感知
- 主循环中不要阻塞 WebSocket 的读写协程
- 参考 Claude Code 的设计：`submitMessage()` 是 AsyncGenerator，每次 yield 一个 `SDKMessage`
