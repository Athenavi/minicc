"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { Message, ToolCall, ToolCallStatus, PermissionLevel } from "@/lib/types";

type WSStatus = "connecting" | "connected" | "disconnected";

interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "system" | "tool";
  content: string;
  toolCalls?: ToolCallState[];
  timestamp: string;
}

interface ToolCallState {
  id: string;
  name: string;
  input: Record<string, unknown>;
  status: ToolCallStatus;
  level?: PermissionLevel;
  diffPreview?: string;
  result?: string;
  isError?: boolean;
  requestId?: string;
}

function genId() { return Math.random().toString(36).slice(2, 10); }

export default function Home() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [wsStatus, setWsStatus] = useState<WSStatus>("disconnected");
  const [sessionId] = useState(() => genId() + genId());
  const [streamingMsg, setStreamingMsg] = useState<ChatMessage | null>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectCount = useRef(0);
  const [isGenerating, setIsGenerating] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const handleEventRef = useRef<(data: any) => void>(() => {});

  // Auto-scroll
  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: "smooth" }); }, [messages, streamingMsg]);

  // WebSocket connection
  const connect = useCallback(() => {
    const url = `ws://localhost:8000/ws/${sessionId}`;
    setWsStatus("connecting");
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => { setWsStatus("connected"); reconnectCount.current = 0; };
    ws.onclose = () => {
      setWsStatus("disconnected");
      const delay = Math.min(1000 * 2 ** reconnectCount.current, 30000);
      reconnectCount.current += 1;
      setTimeout(connect, delay);
    };
    ws.onerror = () => ws.close();
    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        handleEventRef.current(data);
      } catch { /* ignore */ }
    };
  }, [sessionId]);

  useEffect(() => { connect(); return () => wsRef.current?.close(); }, [connect]);

  const send = useCallback((data: Record<string, unknown>) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(data));
    }
  }, []);

  // Event handler — stored in ref to avoid stale closures in WebSocket
  const handleEvent = useCallback((data: any) => {
    const type = data.type;
    const payload = data.payload || {};

    switch (type) {
      case "session_info":
        break;

      case "text_chunk":
        setStreamingMsg((prev) => {
          const text = payload.text || "";
          if (!prev) {
            return { id: genId(), role: "assistant", content: text, timestamp: new Date().toISOString() };
          }
          return { ...prev, content: prev.content + text };
        });
        break;

      case "tool_call_start":
        setStreamingMsg((prev) => {
          const tc: ToolCallState = {
            id: payload.call_id,
            name: payload.name,
            input: payload.input || {},
            status: payload.level === "read" ? "approved" : "pending",
            level: payload.level,
          };
          const existing = prev?.toolCalls || [];
          return prev ? { ...prev, toolCalls: [...existing, tc] } : prev!;
        });
        break;

      case "tool_call_result":
        setStreamingMsg((prev) => {
          if (!prev) return prev;
          const existing = prev.toolCalls || [];
          const updated = existing.map((tc) =>
            tc.id === payload.call_id
              ? { ...tc, status: payload.is_error ? "failed" as const : "completed" as const, result: payload.output, isError: payload.is_error }
              : tc
          );
          return { ...prev, toolCalls: updated };
        });
        break;

      case "permission_required":
        setStreamingMsg((prev) => {
          if (!prev) return prev;
          const existing = prev.toolCalls || [];
          const updated = existing.map((tc) =>
            tc.name === payload.tool_name && tc.status === "pending"
              ? { ...tc, status: "pending" as const, requestId: payload.request_id, level: payload.level, diffPreview: payload.diff_preview }
              : tc
          );
          return { ...prev, toolCalls: updated };
        });
        break;

      case "message_complete":
        setIsGenerating(false);
        break;

      case "command_result":
        setMessages((prev) => [...prev, { id: genId(), role: "system", content: payload.output || "", timestamp: new Date().toISOString() }]);
        break;

      case "compaction":
        // Context was compressed — show a subtle indicator
        setMessages((prev) => [...prev, { id: genId(), role: "system", content: `📦 Context compressed (freed ~${payload.tokens_freed || 0} tokens)`, timestamp: new Date().toISOString() }]);
        break;

      case "error":
        setMessages((prev) => [...prev, { id: genId(), role: "system", content: `❌ Error: ${payload.message || "Unknown error"}`, timestamp: new Date().toISOString() }]);
        setIsGenerating(false);
        break;

      case "pong":
        break;
    }
  }, []);

  // Send user message
  const handleSend = useCallback(() => {
    const text = input.trim();
    if (!text) return;
    setInput("");
    setIsGenerating(true);
    // Add message locally for instant feedback, then send to backend
    setMessages((prev) => [...prev, { id: genId(), role: "user", content: text, timestamp: new Date().toISOString() }]);
    send({ type: "user_message", payload: { content: text } });
  }, [input, send]);

  // Sync handleEvent to ref so WebSocket always gets latest version
  useEffect(() => { handleEventRef.current = handleEvent; }, [handleEvent]);

  // Finalize streaming message when generation stops
  const prevGenerating = useRef(false);
  useEffect(() => {
    if (prevGenerating.current && !isGenerating && streamingMsg) {
      setMessages((prev) => [...prev, streamingMsg]);
      setStreamingMsg(null);
    }
    prevGenerating.current = isGenerating;
  }, [isGenerating, streamingMsg]);

  // Approval actions
  const handleApproval = useCallback((requestId: string, action: "approve" | "reject" | "always_allow") => {
    send({ type: "approval_action", payload: { request_id: requestId, action } });
    // Update local state
    setStreamingMsg((prev) => {
      if (!prev) return prev;
      const updated = (prev.toolCalls || []).map((tc) =>
        tc.requestId === requestId
          ? { ...tc, status: (action === "approve" || action === "always_allow" ? "approved" : "rejected") as ToolCallStatus }
          : tc
      );
      return { ...prev, toolCalls: updated };
    });
  }, [send]);

  const handleCancel = useCallback(() => {
    send({ type: "cancel", payload: {} });
    setIsGenerating(false);
  }, [send]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if ((e.key === "Enter" && !e.shiftKey) || (e.key === "Enter" && e.ctrlKey)) { e.preventDefault(); handleSend(); }
  };

  // Loading skeleton
  const Skeleton = ({ className }: { className?: string }) => (
    <div className={`animate-pulse bg-gray-200 dark:bg-gray-700 rounded ${className || ""}`} />
  );

  // Render a single message
  const renderMessage = (msg: ChatMessage, isStreaming?: boolean) => {
    const isUser = msg.role === "user";
    const isSystem = msg.role === "system";
    const isTool = msg.role === "tool";

    return (
      <div key={msg.id} className={`flex gap-3 ${isUser ? "flex-row-reverse" : ""} ${isStreaming ? "opacity-80" : ""}`}>
        {/* Avatar */}
        <div className={`w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold shrink-0 mt-1 ${
          isUser ? "bg-blue-600 text-white" : isSystem ? "bg-gray-400 dark:bg-gray-600 text-white" : "bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300"
        }`}>
          {isUser ? "U" : isSystem ? "⚙" : "M"}
        </div>

        {/* Content */}
        <div className={`max-w-[75%] space-y-2 ${isUser ? "items-end" : ""}`}>
          {/* Text */}
          {msg.content && (
            <div className={`rounded-xl px-4 py-2.5 text-sm whitespace-pre-wrap ${
              isUser ? "bg-blue-600 text-white" : isSystem ? "bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 text-xs" : "bg-white dark:bg-gray-800 border dark:border-gray-700 dark:text-gray-100"
            }`}>
              {msg.content}
              {isStreaming && <span className="animate-pulse">▍</span>}
            </div>
          )}

          {/* Tool calls */}
          {msg.toolCalls?.map((tc) => (
            <div key={tc.id} className={`border rounded-lg overflow-hidden text-sm ${
              tc.status === "rejected" || tc.status === "failed" ? "border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-950" :
              tc.status === "running" ? "border-blue-300 dark:border-blue-700 bg-blue-50 dark:bg-blue-950" :
              tc.status === "completed" ? "border-green-300 dark:border-green-700 bg-green-50 dark:bg-green-950" :
              "border-yellow-300 dark:border-yellow-700 bg-yellow-50 dark:bg-yellow-950"
            }`}>
              {/* Header */}
              <div className="flex items-center gap-2 px-3 py-1.5 border-b dark:border-gray-700 bg-white dark:bg-gray-800 text-xs font-medium dark:text-gray-100">
                <span>🔧 {tc.name}</span>
                <span className={`ml-auto px-2 py-0.5 rounded-full text-[10px] font-bold ${
                  tc.status === "completed" ? "bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300" :
                  tc.status === "failed" || tc.status === "rejected" ? "bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-300" :
                  tc.status === "running" ? "bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300" :
                  "bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-300"
                }`}>{tc.status}</span>
              </div>

              {/* Diff preview */}
              {tc.diffPreview && (
                <pre className="p-2 text-xs font-mono overflow-x-auto max-h-40 bg-white border-b">{tc.diffPreview}</pre>
              )}

              {/* Result */}
              {tc.result && (
                <pre className={`p-2 text-xs font-mono max-h-40 overflow-auto ${tc.isError ? "text-red-600" : "text-gray-700"}`}>
                  {tc.result.slice(0, 1000)}
                </pre>
              )}

              {/* Approval buttons */}
              {tc.status === "pending" && tc.requestId && (
                <div className="flex gap-2 p-2 bg-white border-t">
                  <button onClick={() => handleApproval(tc.requestId!, "approve")} className="flex-1 px-3 py-1.5 text-xs font-medium rounded bg-green-600 text-white hover:bg-green-700">✅ Approve</button>
                  <button onClick={() => handleApproval(tc.requestId!, "reject")} className="px-3 py-1.5 text-xs font-medium rounded bg-red-600 text-white hover:bg-red-700">❌ Reject</button>
                  <button onClick={() => handleApproval(tc.requestId!, "always_allow")} className="px-3 py-1.5 text-xs font-medium rounded bg-gray-200 text-gray-700 hover:bg-gray-300 border">⭐ Always</button>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    );
  };

  return (
    <div className="flex h-screen bg-gray-100 dark:bg-gray-900">
      {/* Sidebar overlay for mobile */}
      {sidebarOpen && <div className="fixed inset-0 bg-black/30 z-10 md:hidden" onClick={() => setSidebarOpen(false)} />}

      {/* Sidebar */}
      <div className={`${sidebarOpen ? "translate-x-0" : "-translate-x-full"} md:translate-x-0 fixed md:static z-20 md:z-auto w-64 bg-white dark:bg-gray-800 border-r dark:border-gray-700 flex flex-col shrink-0 transition-transform duration-200 h-full`}>
        <div className="flex items-center justify-between p-3 border-b dark:border-gray-700">
          <button onClick={() => { setMessages([]); setStreamingMsg(null); }} className="flex-1 py-2 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 mr-2">
            + New Chat
          </button>
          <button onClick={() => setSidebarOpen(false)} className="md:hidden p-1 text-gray-400 hover:text-gray-600">
            ✕
          </button>
        </div>
        <div className="flex-1 overflow-y-auto p-2 space-y-1 text-xs text-gray-500 dark:text-gray-400">
          {messages.filter(m => m.role !== "system").slice(-10).map(m => (
            <div key={m.id} className="px-2 py-1.5 rounded hover:bg-gray-100 dark:hover:bg-gray-700 truncate">{m.content.slice(0, 60)}</div>
          ))}
          {messages.length === 0 && <div className="text-center py-8 text-gray-400 dark:text-gray-500">No conversations yet</div>}
        </div>
        <div className="p-3 border-t dark:border-gray-700 text-[10px] text-gray-400 dark:text-gray-500 text-center">MiniCC v0.1</div>
      </div>

      {/* Main */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Header */}
        <header className="flex items-center gap-3 px-4 h-12 bg-white dark:bg-gray-800 border-b dark:border-gray-700 shrink-0">
          <button onClick={() => setSidebarOpen(true)} className="md:hidden p-1 text-gray-500 hover:text-gray-700 dark:text-gray-400">
            ☰
          </button>
          <h1 className="text-sm font-semibold dark:text-gray-100">MiniCC</h1>
          <span className={`w-2 h-2 rounded-full ${wsStatus === "connected" ? "bg-green-500" : wsStatus === "connecting" ? "bg-yellow-500" : "bg-red-500"}`} />
          <span className="text-xs text-gray-400 dark:text-gray-500">{wsStatus}</span>
        </header>

        {/* Messages */}
        <div className="flex-1 overflow-y-auto p-4 space-y-4 dark:bg-gray-900">
          {messages.map((m) => renderMessage(m))}
          {streamingMsg && renderMessage(streamingMsg, true)}
          {isGenerating && !streamingMsg && (
            <div className="flex gap-3">
              <div className="w-8 h-8 rounded-full bg-gray-200 dark:bg-gray-700 flex items-center justify-center text-xs font-bold shrink-0 mt-1">M</div>
              <div className="space-y-2 flex-1 max-w-[75%]">
                <div className="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
                  <span className="w-2 h-2 bg-blue-500 rounded-full animate-pulse" />
                  Thinking...
                </div>
                <Skeleton className="h-4 w-3/4" />
                <Skeleton className="h-4 w-1/2" />
              </div>
            </div>
          )}
          {streamingMsg && streamingMsg.toolCalls?.some(tc => tc.status === "pending" || tc.status === "approved") && (
            <div className="flex gap-3">
              <div className="w-8 h-8 rounded-full bg-gray-200 dark:bg-gray-700 flex items-center justify-center text-xs font-bold shrink-0 mt-1">⚙</div>
              <div className="text-sm text-gray-500 dark:text-gray-400 py-2">
                <span className="w-2 h-2 bg-blue-500 rounded-full animate-pulse inline-block mr-2" />
                Waiting for approval...
              </div>
            </div>
          )}
          {messages.length === 0 && !streamingMsg && !isGenerating && (
            <div className="flex items-center justify-center h-full text-gray-400 dark:text-gray-500 text-sm">
              <div className="text-center space-y-2">
                <p className="text-lg font-medium text-gray-300 dark:text-gray-600">MiniCC</p>
                <p>Send a message to start a conversation</p>
                <p className="text-xs">Type /help for available commands</p>
              </div>
            </div>
          )}
          <div ref={bottomRef} />
        </div>

        {/* Status bar */}
        {isGenerating && (
          <div className="px-4 py-1.5 bg-blue-50 dark:bg-blue-950 border-t dark:border-blue-800 text-xs text-blue-600 dark:text-blue-400 flex items-center gap-2 shrink-0">
            <span className="w-1.5 h-1.5 bg-blue-500 rounded-full animate-pulse" />
            {streamingMsg?.toolCalls?.some(tc => tc.status === "pending")
              ? "Waiting for approval..."
              : streamingMsg?.toolCalls?.some(tc => tc.status === "running" || tc.status === "approved")
              ? "Executing tool..."
              : "Generating response..."}
            <button onClick={handleCancel} className="ml-auto text-red-500 hover:text-red-700 text-[10px] font-medium">Stop</button>
          </div>
        )}

        {/* Input */}
        <div className="border-t dark:border-gray-700 bg-white dark:bg-gray-800 p-4 shrink-0">
          <div className="flex gap-2 max-w-4xl mx-auto">
            <input
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Type a message... (/help for commands)"
              disabled={isGenerating}
              className="flex-1 px-4 py-2 border dark:border-gray-600 rounded-lg text-sm dark:bg-gray-700 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
            />
            {isGenerating ? (
              <button onClick={handleCancel} className="px-4 py-2 bg-red-600 text-white rounded-lg text-sm font-medium hover:bg-red-700">⏹ Stop</button>
            ) : (
              <button onClick={handleSend} disabled={!input.trim()} className="px-4 py-2 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 disabled:opacity-50">Send</button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
