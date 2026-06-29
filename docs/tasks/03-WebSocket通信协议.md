# 任务 03：WebSocket 通信协议与心跳机制

> **所属阶段**：Phase 0 - 基础骨架与协议定义
> **对应模块**：模块 1 (前端通信层) + 后端 WebSocket 端点
> **预估工时**：2-3 天
> **依赖**：任务 01 (Pydantic 模型)、任务 02 (前端骨架)

---

## 1. 任务目标

建立前端与后端之间的 WebSocket 长连接通信协议，定义所有消息类型的 JSON Schema，实现稳定的心跳检测和自动重连。

## 2. 详细子任务

### 2.1 协议消息类型定义

前后端间所有消息遵循统一的 JSON 信封格式：

```typescript
// 所有消息的顶层信封
{
  "type": string,      // 消息类型标识
  "id": string,        // 消息唯一 ID (UUID)
  "timestamp": string,  // ISO 8601 时间戳
  "payload": object    // 具体消息内容
}
```

定义以下消息类型（均在 `protocol.py` 和前端 `types.ts` 中同步实现）：

#### 客户端 → 服务端 (C2S)

| type | payload | 说明 |
|:-----|:--------|:-----|
| `user_message` | `{ content: string }` | 用户发送消息 |
| `approval_action` | `{ request_id: string, action: "approve" \| "reject" \| "always_allow" }` | 审批响应 |
| `cancel` | `{}` | 中断当前生成 |
| `ping` | `{}` | WebSocket 心跳 Ping |
| `session_resume` | `{ session_id: string }` | 恢复历史会话 |

#### 服务端 → 客户端 (S2C)

| type | payload | 说明 |
|:-----|:--------|:-----|
| `message_chunk` | `{ index: number, content: ContentBlock, done: boolean }` | 流式消息块 |
| `message_complete` | `{ message_id: string }` | 一次完整消息结束 |
| `tool_call_start` | `{ call_id, name, input, level }` | 工具调用开始 |
| `tool_call_result` | `{ call_id, output, is_error }` | 工具执行结果 |
| `permission_required` | `{ request_id, tool_name, tool_input, level, diff_preview?, reason? }` | 需要用户审批 |
| `permission_result` | `{ request_id, action }` | 审批结果回执 |
| `status_update` | `{ status: "thinking" \| "executing" \| "waiting_approval" \| "idle", message?: string }` | Agent 状态变更 |
| `error` | `{ code: string, message: string }` | 服务端错误 |
| `session_info` | `{ session_id, created_at, message_count }` | 会话信息 |
| `pong` | `{}` | 心跳回复 |

### 2.2 后端 WebSocket 端点实现

#### 端点：`GET /ws/{session_id}`

- [ ] 连接升级后立即发送 `session_info`
- [ ] 为每个连接创建独立的消息队列（`asyncio.Queue`）
- [ ] 实现双协程架构：

```python
# 两个并行的异步协程
async def reader(websocket, queue):   # 读：接收客户端消息
async def writer(websocket, queue):   # 写：从队列取消息发送

# 用 TaskGroup 管理
async with asyncio.TaskGroup() as tg:
    tg.create_task(reader(ws, queue))
    tg.create_task(writer(ws, queue))
```

- [ ] `reader` 协程：
  - 解析 JSON 消息，根据 `type` 路由
  - `ping` → 回复 `pong`
  - `user_message` → 转发给 QueryEngine (Phase 1)
  - `approval_action` → 解锁 Permission 等待
  - `cancel` → 设置取消标记
- [ ] `writer` 协程：
  - 从 Queue 取消息，序列化为 JSON 发送
  - 处理发送异常（连接断开时优雅退出）
- [ ] 连接断开处理：
  - 设置会话为 `disconnected` 状态
  - 如果在执行工具，根据配置决定继续或中止

### 2.3 心跳机制

- [ ] 服务端：每 30s 检查连接活跃度，60s 无消息发送 `ping`（或靠 TCP keepalive）
- [ ] 客户端 (`useWebSocket`)：
  - 每 25s 发送 `ping`
  - 60s 内未收到 `pong` 或任何消息 → 触发重连
  - 重连策略：`1s → 2s → 4s → 8s → 16s → 30s (max)`
  - 重连时自动发送 `session_resume`

### 2.4 前端 type 定义 (`frontend/src/lib/types.ts`)

- [ ] 所有 S2C 消息类型的 TypeScript interface
- [ ] 所有 C2S 消息类型的 TypeScript interface
- [ ] 枚举 `AgentStatus`, `PermissionLevel`, `ToolCallStatus`

### 2.5 前端 `useWebSocket` 钩子完整实现

- [ ] 连接管理（connect / disconnect / reconnect）
- [ ] 消息路由：按 `type` 分发到不同回调处理器
- [ ] 类型安全的 `send()` 和 `sendJSON()` 方法
- [ ] 连接状态暴露给 React 组件
- [ ] 自动重连逻辑
- [ ] 网络恢复检测（`window.addEventListener('online', ...)`）

### 2.6 端到端联调测试

- [ ] 使用 `backend` + `frontend` 同时启动
- [ ] 测试流程：前端连接 → 收到 `session_info` → 发送 `ping` → 收到 `pong`
- [ ] 模拟后端发送 `message_chunk` → 前端显示流式文本
- [ ] 测试连接断开后自动重连

## 3. 验收标准

| # | 检查项 | 验证方式 |
|:-|:-|:-|
| 1 | WebSocket 连接成功建立并收到 `session_info` | 浏览器 DevTools Network 面板 |
| 2 | 心跳 Ping/Pong 每 30s 正常收发 | 日志 / DevTools |
| 3 | 断开后客户端按指数退避重连 | 关闭后端进程观察前端重连日志 |
| 4 | `message_chunk` 可在前端逐块渲染 | 手动发送测试消息验证 |
| 5 | 所有消息类型 JSON Schema 前后端一致 | 对照检查 |
| 6 | `cancel` 消息可被后端正确接收 | 日志 |
| 7 | 同时启动前后端无 CORS/跨域问题 | 端到端测试 |

## 4. 参考资源

- [FastAPI WebSocket 文档](https://fastapi.tiangolo.com/advanced/websockets/)
- [MDN WebSocket API](https://developer.mozilla.org/en-US/docs/Web/API/WebSocket)
- 规划文档 §5 核心数据流、§6 启动命令骨架
- 参考 claw-code 的 `g011-acp-json-rpc-status-contract.md` 契约设计理念

## 5. 注意事项

- WebSocket 消息必须使用 JSON，不得使用自定义二进制协议
- 前后端必须使用同一套 `types.ts` / Python dataclass 作为唯一真相源 (SSOT)
- 建议在后端维护一个 `MessageType` 枚举，防止拼写错误
- 大消息（如文件内容）需考虑分片或压缩
- 异常断开时不能泄露内存——需清理连接相关的资源
