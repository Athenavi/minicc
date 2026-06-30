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
  id: string; name: string; status: string;
  result?: string; isError?: boolean;
  requestId?: string; diffPreview?: string;
}

interface Conversation {
  id: string;
  title: string;
  messages: ChatMessage[];
  sessionId: string;
}

function genId() { return Math.random().toString(36).slice(2, 10); }
function Skeleton({ className }: { className?: string }) {
  return <div className={`animate-pulse bg-gray-200 dark:bg-gray-700 rounded ${className || ""}`} />;
}
function CollapsibleContent({ text, maxLen = 500 }: { text: string; maxLen?: number }) {
  const [expanded, setExpanded] = useState(false);
  const isLong = text.length > maxLen;
  const lineCount = text.split('\n').length;
  const displayText = isLong && !expanded ? text.slice(0, maxLen) + "\n\n... [" + lineCount + " lines, " + text.length + " chars]" : text;
  return (
    <div>
      <pre className="whitespace-pre-wrap font-sans text-sm m-0">{displayText}</pre>
      {isLong && <button onClick={() => setExpanded(!expanded)} className="mt-1 text-[10px] font-medium text-blue-500 hover:text-blue-700 underline">{expanded ? "▲ Show less" : "▼ Show all (" + lineCount + " lines)"}</button>}
    </div>
  );
}

export default function Home() {
  const [conversations, setConversations] = useState<Conversation[]>([{ id: genId(), title: "Chat 1", messages: [], sessionId: genId() + genId() }]);
  const [activeIdx, setActiveIdx] = useState(0);
  const [input, setInput] = useState("");
  const [connStatus, setConnStatus] = useState<ConnStatus>("disconnected");
  const [streamingMsg, setStreamingMsg] = useState<ChatMessage | null>(null);
  const [isGenerating, setIsGenerating] = useState(false);
  const [execMode, setExecMode] = useState("ask");
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);
  const prevGenRef = useRef(false);
  const handleEventRef = useRef<(data: any) => void>(() => {});
  const activeConv = conversations[activeIdx];

  // SSE connection (per session)
  useEffect(() => {
    let eventSource: EventSource | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout>;
    function connectSSE() {
      setConnStatus("connecting");
      eventSource = new EventSource("http://localhost:8000/events");
      eventSource.onopen = () => setConnStatus("connected");
      eventSource.onmessage = (e) => {
        if (e.data.startsWith(": ping")) return;
        try { handleEventRef.current(JSON.parse(e.data)); } catch { }
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

  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: "smooth" }); }, [activeConv.messages, streamingMsg]);

  useEffect(() => {
    if (prevGenRef.current && !isGenerating && streamingMsg) {
      setConversations((prev) => prev.map((c, i) =>
        i === activeIdx ? { ...c, messages: [...c.messages, streamingMsg] } : c
      ));
      setStreamingMsg(null);
    }
    prevGenRef.current = isGenerating;
  }, [isGenerating, streamingMsg, activeIdx]);

  const updateMessages = useCallback((fn: (msgs: ChatMessage[]) => ChatMessage[]) => {
    setConversations((prev) => prev.map((c, i) => i === activeIdx ? { ...c, messages: fn(c.messages) } : c));
  }, [activeIdx]);

  const handleSSEEvent = useCallback((data: any) => {
    const kind = data.kind;
    switch (kind) {
      case "text":
        setStreamingMsg((prev) => {
          const text = data.text || "";
          const next = prev || { id: genId(), role: "assistant", content: "" };
          return { ...next, content: next.content + text };
        });
        break;
      case "tool_dispatch":
        setStreamingMsg((prev) => {
          if (!prev) return prev;
          const tc: ToolCallState = { id: data.id || "", name: data.name || "", status: "pending" };
          return { ...prev, toolCalls: [...(prev.toolCalls || []), tc] };
        });
        break;
      case "tool_result":
        setStreamingMsg((prev) => prev ? {
          ...prev,
          toolCalls: (prev.toolCalls || []).map((tc) =>
            tc.id === data.id ? { ...tc, status: data.is_error ? "failed" : "completed", result: data.output, isError: data.is_error } : tc
          ),
        } : prev);
        break;
      case "approval_request":
        setStreamingMsg((prev) => prev ? {
          ...prev,
          toolCalls: (prev.toolCalls || []).map((tc) =>
            tc.name === data.tool_name ? { ...tc, status: "pending", requestId: data.request_id, diffPreview: data.diff_preview } : tc
          ),
        } : prev);
        break;
      case "message":
        if (data.text) setStreamingMsg((prev) => prev ? { ...prev, content: data.text } : prev);
        break;
      case "turn_done":
        setIsGenerating(false);
        break;
      case "notice":
        updateMessages((msgs) => [...msgs, { id: genId(), role: "system", content: data.message || "" }]);
        break;
      case "error":
        updateMessages((msgs) => [...msgs, { id: genId(), role: "system", content: "❌ Error: " + (data.message || "Unknown") }]);
        setIsGenerating(false);
        break;
    }
  }, [updateMessages]);

  useEffect(() => { handleEventRef.current = handleSSEEvent; }, [handleSSEEvent]);

  const handleSend = useCallback(async () => {
    const text = input.trim();
    if (!text) return;
    setInput("");
    setIsGenerating(true);
    const sid = activeConv.sessionId;
    updateMessages((msgs) => [...msgs, { id: genId(), role: "user", content: text }]);
    try {
      await fetch("http://localhost:8000/submit", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ content: text, session_id: sid }) });
    } catch (err) {
      updateMessages((msgs) => [...msgs, { id: genId(), role: "system", content: "❌ Failed: " + err }]);
      setIsGenerating(false);
    }
  }, [input, activeConv, updateMessages]);

  const handleCancel = useCallback(async () => {
    await fetch("http://localhost:8000/cancel", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ session_id: activeConv.sessionId }) });
    setIsGenerating(false);
  }, [activeConv]);

  const handleApproval = useCallback(async (requestId: string, action: string) => {
    await fetch("http://localhost:8000/approve", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ session_id: activeConv.sessionId, request_id: requestId, action }) });
    setStreamingMsg((prev) => prev ? { ...prev, toolCalls: (prev.toolCalls || []).map((tc) => tc.requestId === requestId ? { ...tc, status: action === "approve" || action === "always_allow" ? "approved" : "rejected" } : tc) } : prev);
  }, [activeConv]);

  const newConversation = useCallback(() => {
    if (isGenerating) return;
    setConversations((prev) => [...prev, { id: genId(), title: "Chat " + (prev.length + 1), messages: [], sessionId: genId() + genId() }]);
    setActiveIdx((prev) => prev + 1);
    setStreamingMsg(null);
  }, [isGenerating]);

  const switchConversation = useCallback((idx: number) => {
    if (isGenerating) return;
    setActiveIdx(idx);
    setStreamingMsg(null);
  }, [isGenerating]);

  const deleteConversation = useCallback((idx: number) => {
    if (conversations.length <= 1) return;
    setConversations((prev) => prev.filter((_, i) => i !== idx));
    if (activeIdx >= idx && activeIdx > 0) setActiveIdx((prev) => prev - 1);
  }, [conversations.length, activeIdx]);

  const renderMessage = (msg: ChatMessage, isStreaming?: boolean) => {
    const isUser = msg.role === "user";
    const isSystem = msg.role === "system";
    return (
      <div key={msg.id} className={`flex gap-3 ${isUser ? "flex-row-reverse" : ""} ${isStreaming ? "opacity-80" : ""}`}>
        <div className={`w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold shrink-0 mt-1 ${isUser ? "bg-blue-600 text-white" : isSystem ? "bg-gray-400 dark:bg-gray-600 text-white" : "bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300"}`}>{isUser ? "U" : isSystem ? "⚙" : "M"}</div>
        <div className="max-w-[75%] space-y-2">
          {msg.content && (
            <div className={`rounded-xl px-4 py-2.5 text-sm ${isUser ? "bg-blue-600 text-white" : isSystem ? "bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 text-xs" : "bg-white dark:bg-gray-800 border dark:border-gray-700 dark:text-gray-100"}`}>
              <CollapsibleContent text={msg.content} />
              {isStreaming && <span className="animate-pulse">▍</span>}
            </div>
          )}
          {msg.toolCalls?.map((tc) => (
            <div key={tc.id} className={`border rounded-lg overflow-hidden text-sm ${tc.status === "rejected" || tc.status === "failed" ? "border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-950" : tc.status === "completed" ? "border-green-300 dark:border-green-700 bg-green-50 dark:bg-green-950" : "border-yellow-300 dark:border-yellow-700 bg-yellow-50 dark:bg-yellow-950"}`}>
              <div className="flex items-center gap-2 px-3 py-1.5 border-b dark:border-gray-700 bg-white dark:bg-gray-800 text-xs font-medium dark:text-gray-100">
                <span>🔧 {tc.name}</span>
                <span className={`ml-auto px-2 py-0.5 rounded-full text-[10px] font-bold ${tc.status === "completed" ? "bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300" : tc.status === "failed" || tc.status === "rejected" ? "bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-300" : "bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-300"}`}>{tc.status}</span>
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
    <div className="flex flex-col h-screen bg-gray-100 dark:bg-gray-900">
      {/* Top header with mode + tabs */}
      <div className="bg-white dark:bg-gray-800 border-b dark:border-gray-700 shrink-0">
        <div className="flex items-center gap-2 px-3 h-10">
          <button onClick={() => setSidebarOpen(!sidebarOpen)} className="md:hidden p-1 text-gray-500">☰</button>
          <h1 className="text-sm font-semibold dark:text-gray-100 mr-3">MiniCC</h1>
          <span className={`w-2 h-2 rounded-full ${connStatus === "connected" ? "bg-green-500" : connStatus === "connecting" ? "bg-yellow-500" : "bg-red-500"}`} />
          {/* Conversation tabs — horizontal bar */}
          <div className="flex gap-1 flex-1 min-w-0 overflow-x-auto no-scrollbar ml-2" style={{scrollbarWidth:'none'}}>
            {conversations.map((conv, i) => (
              <div key={conv.id} className={`flex items-center gap-1 px-2.5 py-1 rounded-md text-xs cursor-pointer whitespace-nowrap shrink-0 max-w-[150px] transition-colors ${i === activeIdx ? "bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300 font-medium" : "text-gray-500 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700"}`}
                onClick={() => switchConversation(i)}>
                <span className="truncate">{conv.title}</span>
                {conversations.length > 1 && <button onClick={(e) => { e.stopPropagation(); deleteConversation(i); }} className="ml-0.5 text-gray-400 hover:text-red-500 text-[10px] leading-none">✕</button>}
              </div>
            ))}
            <button onClick={newConversation} disabled={isGenerating} className="px-2 py-1 text-xs text-gray-400 hover:text-blue-500 hover:bg-blue-50 dark:hover:bg-blue-950 rounded shrink-0 disabled:opacity-50">+</button>
          </div>
          {/* Mode toggles */}
          <div className="flex items-center gap-1 ml-auto shrink-0">
            {["ask", "auto", "yolo"].map((m) => (
              <button key={m} onClick={async () => { setExecMode(m); await fetch("http://localhost:8000/mode", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ session_id: activeConv.sessionId, mode: m }) }); }}
                className={`px-2 py-0.5 text-[10px] font-medium rounded border transition-colors ${execMode === m ? "bg-blue-600 text-white border-blue-600" : "bg-transparent text-gray-500 dark:text-gray-400 border-gray-300 dark:border-gray-600 hover:bg-gray-100 dark:hover:bg-gray-700"}`}>{m === "ask" ? "💬 Ask" : m === "auto" ? "⚡ Auto" : "🔥 YOLO"}</button>
            ))}
          </div>
        </div>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4 dark:bg-gray-900">
        {activeConv.messages.map((m) => renderMessage(m))}
        {streamingMsg && renderMessage(streamingMsg, true)}
        {isGenerating && !streamingMsg && (
          <div className="flex gap-3">
            <div className="w-8 h-8 rounded-full bg-gray-200 dark:bg-gray-700 flex items-center justify-center text-xs font-bold shrink-0 mt-1">M</div>
            <div className="space-y-2 flex-1 max-w-[75%]">
              <div className="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400"><span className="w-2 h-2 bg-blue-500 rounded-full animate-pulse" />Thinking...</div>
              <Skeleton className="h-4 w-3/4" /><Skeleton className="h-4 w-1/2" />
            </div>
          </div>
        )}
        {activeConv.messages.length === 0 && !streamingMsg && !isGenerating && (
          <div className="flex items-center justify-center h-full text-gray-400 dark:text-gray-500 text-sm">
            <div className="text-center space-y-2"><p className="text-lg font-medium text-gray-300 dark:text-gray-600">MiniCC</p><p>Send a message to start</p><p className="text-xs">+ New Chat for multi-tab conversations</p></div>
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Status bar */}
      {isGenerating && (
        <div className="px-4 py-1.5 bg-blue-50 dark:bg-blue-950 border-t dark:border-blue-800 text-xs text-blue-600 dark:text-blue-400 flex items-center gap-2 shrink-0">
          <span className="w-1.5 h-1.5 bg-blue-500 rounded-full animate-pulse" />
          {streamingMsg?.toolCalls?.some(tc => tc.status === "pending") ? "Waiting for approval..." : "Generating..."}
          <button onClick={handleCancel} className="ml-auto text-red-500 hover:text-red-700 text-[10px] font-medium">Stop</button>
        </div>
      )}

      {/* Input */}
      <div className="border-t dark:border-gray-700 bg-white dark:bg-gray-800 p-4 shrink-0">
        <div className="flex gap-2 max-w-4xl mx-auto">
          <textarea value={input} onChange={(e) => setInput(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); handleSend(); } }}
            placeholder="Type a message... (/help for commands)" disabled={isGenerating}
            rows={Math.min(Math.max(input.split('\n').length, 1), 8)}
            className="flex-1 px-4 py-2 border dark:border-gray-600 rounded-lg text-sm dark:bg-gray-700 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 resize-none font-mono leading-6"
            style={{ minHeight: '42px', maxHeight: '200px' }} />
          {isGenerating ? (
            <button onClick={handleCancel} className="px-4 py-2 bg-red-600 text-white rounded-lg text-sm font-medium hover:bg-red-700">⏹ Stop</button>
          ) : (
            <button onClick={handleSend} disabled={!input.trim()} className="px-4 py-2 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 disabled:opacity-50">Send</button>
          )}
        </div>
      </div>
    </div>
  );
}
