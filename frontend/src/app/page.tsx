"use client";

import { useCallback, useEffect, useRef, useState } from "react";

type ConnStatus = "connecting" | "connected" | "disconnected";

interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "system" | "tool";
  content: string;
  toolCalls?: ToolCallState[];
}

interface ToolCallState {
  id: string;
  name: string;
  status: string;
  result?: string;
  isError?: boolean;
  requestId?: string;
  diffPreview?: string;
}

function genId() { return Math.random().toString(36).slice(2, 10); }

function Skeleton({ className }: { className?: string }) {
  return <div className={`animate-pulse bg-gray-200 dark:bg-gray-700 rounded ${className || ""}`} />;
}

// Collapsible content for long messages (Reasonix-style)
function CollapsibleContent({ text, maxLen = 500, className }: { text: string; maxLen?: number; className?: string }) {
  const [expanded, setExpanded] = useState(false);
  const isLong = text.length > maxLen;
  const lineCount = text.split('\n').length;
  const displayText = isLong && !expanded ? text.slice(0, maxLen) + "\n\n... [" + lineCount + " lines, " + text.length + " chars]" : text;
  return (
    <div className={className}>
      <pre className="whitespace-pre-wrap font-sans text-sm m-0">{displayText}</pre>
      {isLong && (
        <button onClick={() => setExpanded(!expanded)}
          className="mt-1 text-[10px] font-medium text-blue-500 hover:text-blue-700 underline">
          {expanded ? "▲ Show less" : "▼ Show all (" + lineCount + " lines)"}
        </button>
      )}
    </div>
  );
}

export default function Home() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [connStatus, setConnStatus] = useState<ConnStatus>("disconnected");
  const [sessionId] = useState(() => genId() + genId());
  const [streamingMsg, setStreamingMsg] = useState<ChatMessage | null>(null);
  const [isGenerating, setIsGenerating] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [approvals, setApprovals] = useState<Record<string, any>>({});
  const bottomRef = useRef<HTMLDivElement>(null);
  const prevGenRef = useRef(false);

  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: "smooth" }); }, [messages, streamingMsg]);

  // SSE connection (Reasonix-style)
  useEffect(() => {
    let eventSource: EventSource | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout>;

    function connectSSE() {
      setConnStatus("connecting");
      eventSource = new EventSource(`http://localhost:8000/events`);

      eventSource.onopen = () => setConnStatus("connected");

      eventSource.onmessage = (e) => {
        if (e.data.startsWith(": ping")) return; // keepalive
        try {
          const data = JSON.parse(e.data);
          handleSSEEvent(data);
        } catch { /* ignore */ }
      };

      eventSource.onerror = () => {
        setConnStatus("disconnected");
        eventSource?.close();
        reconnectTimer = setTimeout(connectSSE, 3000);
      };
    }

    connectSSE();
    return () => { eventSource?.close(); clearTimeout(reconnectTimer); };
  }, []);

  // Finalize streaming message when generation stops
  useEffect(() => {
    if (prevGenRef.current && !isGenerating && streamingMsg) {
      setMessages((prev) => [...prev, streamingMsg]);
      setStreamingMsg(null);
    }
    prevGenRef.current = isGenerating;
  }, [isGenerating, streamingMsg]);

  // SSE event handler
  const handleSSEEvent = useCallback((data: any) => {
    const kind = data.kind;

    switch (kind) {
      case "connected":
        console.log("SSE connected");
        break;

      case "text":
        setStreamingMsg((prev) => {
          const text = data.text || "";
          const next = prev || { id: genId(), role: "assistant", content: "" };
          return { ...next, content: next.content + text };
        });
        break;

      case "reasoning":
        // Show thinking indicator
        break;

      case "tool_dispatch":
        setStreamingMsg((prev) => {
          const tc: ToolCallState = {
            id: data.id || "",
            name: data.name || "",
            status: "running",
          };
          const existing = prev?.toolCalls || [];
          return prev ? { ...prev, toolCalls: [...existing, tc] } : prev!;
        });
        break;

      case "tool_progress":
        // Update tool with streaming output
        setStreamingMsg((prev) => {
          if (!prev) return prev;
          const updated = (prev.toolCalls || []).map((tc) =>
            tc.id === data.id ? { ...tc, result: (tc.result || "") + (data.output || "") } : tc
          );
          return { ...prev, toolCalls: updated };
        });
        break;

      case "tool_result":
        setStreamingMsg((prev) => {
          if (!prev) return prev;
          const updated = (prev.toolCalls || []).map((tc) =>
            tc.id === data.id
              ? { ...tc, status: data.is_error ? "failed" : "completed", result: data.output, isError: data.is_error }
              : tc
          );
          return { ...prev, toolCalls: updated };
        });
        break;

      case "approval_request":
        setApprovals((prev) => ({
          ...prev,
          [data.request_id]: { ...data, status: "pending" },
        }));
        setStreamingMsg((prev) => {
          if (!prev) return prev;
          const updated = (prev.toolCalls || []).map((tc) =>
            tc.name === data.tool_name
              ? { ...tc, status: "pending", requestId: data.request_id, diffPreview: data.diff_preview }
              : tc
          );
          return { ...prev, toolCalls: updated };
        });
        break;

      case "message":
        // Final message with full text — frontend can re-render
        if (data.text) {
          setStreamingMsg((prev) => {
            if (prev) return { ...prev, content: data.text };
            return null;
          });
        }
        break;

      case "turn_done":
        setIsGenerating(false);
        setApprovals({});
        break;

      case "notice":
        setMessages((prev) => [...prev, { id: genId(), role: "system", content: data.message || "" }]);
        break;

      case "error":
        setMessages((prev) => [...prev, { id: genId(), role: "system", content: `❌ Error: ${data.message || "Unknown"}` }]);
        setIsGenerating(false);
        break;
    }
  }, []);

  // Send message via HTTP POST (Reasonix-style)
  const handleSend = useCallback(async () => {
    const text = input.trim();
    if (!text) return;
    setInput("");
    setIsGenerating(true);
    setMessages((prev) => [...prev, { id: genId(), role: "user", content: text }]);
    try {
      await fetch("http://localhost:8000/submit", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content: text, session_id: sessionId }),
      });
    } catch (err) {
      setMessages((prev) => [...prev, { id: genId(), role: "system", content: `❌ Failed to send: ${err}` }]);
      setIsGenerating(false);
    }
  }, [input, sessionId]);

  // Cancel via HTTP POST
  const handleCancel = useCallback(async () => {
    try {
      await fetch("http://localhost:8000/cancel", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ session_id: sessionId }),
      });
    } catch { /* ignore */ }
    setIsGenerating(false);
  }, [sessionId]);

  // Approve via HTTP POST
  const handleApproval = useCallback(async (requestId: string, action: string) => {
    try {
      await fetch("http://localhost:8000/approve", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ session_id: sessionId, request_id: requestId, action }),
      });
    } catch { /* ignore */ }
    setApprovals((prev) => ({ ...prev, [requestId]: { ...prev[requestId], status: action } }));
    setStreamingMsg((prev) => {
      if (!prev) return prev;
      const updated = (prev.toolCalls || []).map((tc) =>
        tc.requestId === requestId
          ? { ...tc, status: action === "approve" || action === "always_allow" ? "approved" : "rejected" }
          : tc
      );
      return { ...prev, toolCalls: updated };
    });
  }, [sessionId]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); handleSend(); }
  };

  const renderMessage = (msg: ChatMessage, isStreaming?: boolean) => {
    const isUser = msg.role === "user";
    const isSystem = msg.role === "system";
    return (
      <div key={msg.id} className={`flex gap-3 ${isUser ? "flex-row-reverse" : ""} ${isStreaming ? "opacity-80" : ""}`}>
        <div className={`w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold shrink-0 mt-1 ${
          isUser ? "bg-blue-600 text-white" : isSystem ? "bg-gray-400 dark:bg-gray-600 text-white" : "bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300"
        }`}>
          {isUser ? "U" : isSystem ? "⚙" : "M"}
        </div>
        <div className={`max-w-[75%] space-y-2`}>
          {msg.content && (
            <div className={`rounded-xl px-4 py-2.5 text-sm ${
              isUser ? "bg-blue-600 text-white" : isSystem ? "bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 text-xs" : "bg-white dark:bg-gray-800 border dark:border-gray-700 dark:text-gray-100"
            }`}>
              <CollapsibleContent text={msg.content} className={isUser ? "text-white" : ""} />
              {isStreaming && <span className="animate-pulse">▍</span>}
            </div>
          )}
          {msg.toolCalls?.map((tc) => (
            <div key={tc.id} className={`border rounded-lg overflow-hidden text-sm ${
              tc.status === "rejected" || tc.status === "failed" ? "border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-950" :
              tc.status === "completed" ? "border-green-300 dark:border-green-700 bg-green-50 dark:bg-green-950" :
              "border-yellow-300 dark:border-yellow-700 bg-yellow-50 dark:bg-yellow-950"
            }`}>
              <div className="flex items-center gap-2 px-3 py-1.5 border-b dark:border-gray-700 bg-white dark:bg-gray-800 text-xs font-medium dark:text-gray-100">
                <span>🔧 {tc.name}</span>
                <span className={`ml-auto px-2 py-0.5 rounded-full text-[10px] font-bold ${
                  tc.status === "completed" ? "bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300" :
                  tc.status === "failed" || tc.status === "rejected" ? "bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-300" :
                  "bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-300"
                }`}>{tc.status}</span>
              </div>
              {tc.diffPreview && <pre className="p-2 text-xs font-mono overflow-x-auto max-h-40 bg-white dark:bg-gray-800 border-b dark:border-gray-700">{tc.diffPreview}</pre>}
              {tc.result && <pre className={`p-2 text-xs font-mono max-h-40 overflow-auto ${tc.isError ? "text-red-600" : "text-gray-700 dark:text-gray-300"}`}>{tc.result.slice(0, 1000)}</pre>}
              {tc.status === "pending" && tc.requestId && (
                <div className="flex gap-2 p-2 bg-white dark:bg-gray-800 border-t dark:border-gray-700">
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
      {sidebarOpen && <div className="fixed inset-0 bg-black/30 z-10 md:hidden" onClick={() => setSidebarOpen(false)} />}
      <div className={`${sidebarOpen ? "translate-x-0" : "-translate-x-full"} md:translate-x-0 fixed md:static z-20 md:z-auto w-64 bg-white dark:bg-gray-800 border-r dark:border-gray-700 flex flex-col shrink-0 transition-transform duration-200 h-full`}>
        <div className="flex items-center justify-between p-3 border-b dark:border-gray-700">
          <button onClick={() => { setMessages([]); setStreamingMsg(null); }} className="flex-1 py-2 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 mr-2">+ New Chat</button>
          <button onClick={() => setSidebarOpen(false)} className="md:hidden p-1 text-gray-400 hover:text-gray-600">✕</button>
        </div>
        <div className="flex-1 overflow-y-auto p-2 space-y-1 text-xs text-gray-500 dark:text-gray-400">
          {messages.filter(m => m.role !== "system").slice(-10).map(m => (
            <div key={m.id} className="px-2 py-1.5 rounded hover:bg-gray-100 dark:hover:bg-gray-700 truncate">{m.content.slice(0, 60)}</div>
          ))}
          {messages.length === 0 && <div className="text-center py-8 text-gray-400 dark:text-gray-500">No conversations yet</div>}
        </div>
        <div className="p-3 border-t dark:border-gray-700 text-[10px] text-gray-400 dark:text-gray-500 text-center">MiniCC v0.1</div>
      </div>

      <div className="flex-1 flex flex-col min-w-0">
        <header className="flex items-center gap-3 px-4 h-12 bg-white dark:bg-gray-800 border-b dark:border-gray-700 shrink-0">
          <button onClick={() => setSidebarOpen(true)} className="md:hidden p-1 text-gray-500 hover:text-gray-700 dark:text-gray-400">☰</button>
          <h1 className="text-sm font-semibold dark:text-gray-100">MiniCC</h1>
          <span className={`w-2 h-2 rounded-full ${connStatus === "connected" ? "bg-green-500" : connStatus === "connecting" ? "bg-yellow-500" : "bg-red-500"}`} />
          <span className="text-xs text-gray-400 dark:text-gray-500">{connStatus}</span>
        </header>

        <div className="flex-1 overflow-y-auto p-4 space-y-4 dark:bg-gray-900">
          {messages.map((m) => renderMessage(m))}
          {streamingMsg && renderMessage(streamingMsg, true)}
          {isGenerating && !streamingMsg && (
            <div className="flex gap-3">
              <div className="w-8 h-8 rounded-full bg-gray-200 dark:bg-gray-700 flex items-center justify-center text-xs font-bold shrink-0 mt-1">M</div>
              <div className="space-y-2 flex-1 max-w-[75%]">
                <div className="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
                  <span className="w-2 h-2 bg-blue-500 rounded-full animate-pulse" />Thinking...
                </div>
                <Skeleton className="h-4 w-3/4" /><Skeleton className="h-4 w-1/2" />
              </div>
            </div>
          )}
          <div ref={bottomRef} />
        </div>

        {isGenerating && (
          <div className="px-4 py-1.5 bg-blue-50 dark:bg-blue-950 border-t dark:border-blue-800 text-xs text-blue-600 dark:text-blue-400 flex items-center gap-2 shrink-0">
            <span className="w-1.5 h-1.5 bg-blue-500 rounded-full animate-pulse" />
            {streamingMsg?.toolCalls?.some(tc => tc.status === "pending") ? "Waiting for approval..." : "Generating..."}
            <button onClick={handleCancel} className="ml-auto text-red-500 hover:text-red-700 text-[10px] font-medium">Stop</button>
          </div>
        )}

        <div className="border-t dark:border-gray-700 bg-white dark:bg-gray-800 p-4 shrink-0">
          <div className="flex gap-2 max-w-4xl mx-auto">
            <textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Type a message... (/help for commands)"
              disabled={isGenerating}
              rows={Math.min(Math.max(input.split('\n').length, 1), 8)}
              className="flex-1 px-4 py-2 border dark:border-gray-600 rounded-lg text-sm dark:bg-gray-700 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 resize-none font-mono leading-6"
              style={{ minHeight: '42px', maxHeight: '200px' }}
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
