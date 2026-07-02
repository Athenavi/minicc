/** MiniCC 前后端共享类型定义 — 与 backend/app/models/ 保持同步 */

// -- 消息模型 --

export type Role = "system" | "user" | "assistant" | "tool";

export interface ContentBlock {
  type: "text" | "image" | "file" | "tool_use" | "tool_result";
  text?: string;
  source?: Record<string, unknown>;
  id?: string;
  name?: string;
  input?: Record<string, unknown>;
  tool_use_id?: string;
  content?: ContentBlock[];
}

export interface Message {
  role: Role;
  content: string | ContentBlock[];
  created_at: string;
  model?: string;
}

// -- 工具调用 --

export type ToolCallType =
  | "function" | "bash" | "file_read" | "file_write"
  | "file_edit" | "search" | "web_fetch" | "lsp" | "mcp";

export type ToolCallStatus =
  | "pending" | "approved" | "rejected" | "running" | "completed" | "failed";

export interface ToolCall {
  id: string;
  type: ToolCallType;
  name: string;
  input: Record<string, unknown>;
  status: ToolCallStatus;
}

export interface ToolResult {
  tool_call_id: string;
  output: string;
  is_error: boolean;
  metadata: Record<string, unknown>;
}

// -- 权限 --

export type PermissionLevel = "read" | "write" | "execute";
export type PermissionStatus = "pending" | "approved" | "rejected" | "always_allowed";

export interface PermissionRequest {
  id: string;
  tool_name: string;
  tool_input: Record<string, unknown>;
  level: PermissionLevel;
  reason: string;
  diff_preview?: string;
  status: PermissionStatus;
}

// -- WebSocket 协议 --

export type AgentStatus = "thinking" | "executing" | "waiting_approval" | "idle";

/** WebSocket 消息信封 */
export interface WSMessage {
  type: string;
  id: string;
  timestamp: string;
  payload: Record<string, unknown>;
}
