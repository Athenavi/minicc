"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { MarkdownRenderer } from "@/components/chat/MarkdownRenderer";
import { MonacoEditor } from "@/components/editor/MonacoEditor";
import { api, apiUrl } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Toaster, toast } from "sonner";
import {
  Send, Square, Plus, MessageSquare, LogIn, Check, X, AlertTriangle,
  Bot, FileCode, ListTodo, Building2, PanelRightClose, PanelRightOpen,
  Minimize2, Maximize2, Loader2
} from "lucide-react";

// ── Types ──
interface ChatMessage { id: string; role: string; content: string; }
interface Conversation { id: string; title: string; messages: ChatMessage[]; sessionId: string; }
interface PermissionReq { task_id: string; tool_name: string; task_name: string; session_id: string; }
interface TaskItem { id: string; type: string; status: string; payload?: any; error?: string; created_at: string; }
interface AgentItem { type: string; name: string; description: string; }
interface FileNode { name: string; path: string; type: "file" | "dir"; children?: FileNode[]; }

let idCounter = 0;
function genId() { return (++idCounter).toString(36) + Math.random().toString(36).slice(2, 5); }

export default function WorkspacePage() {
  // ── Conversations ──
  const [conversations, setConversations] = useState<Conversation[]>([{ id: genId(), title: "Chat 1", messages: [], sessionId: genId() + genId() }]);
  const [activeIdx, setActiveIdx] = useState(0);
  const activeIdxRef = useRef(activeIdx);
  activeIdxRef.current = activeIdx;
  const activeConv = conversations[activeIdx] || conversations[0] || { id: "", title: "Chat", messages: [], sessionId: "" };

  // ── Chat ──
  const [input, setInput] = useState("");
  const [streamingMsg, setStreamingMsg] = useState<ChatMessage | null>(null);
  const [isGenerating, setIsGenerating] = useState(false);
  const streamingContentRef = useRef("");
  const bottomRef = useRef<HTMLDivElement>(null);

  // ── Connection ──
  const [connStatus, setConnStatus] = useState<"connecting" | "connected" | "disconnected">("disconnected");
  const [isLoggedIn, setIsLoggedIn] = useState(false);
  const [clientId] = useState(() => genId());

  // ── Mode & Permissions ──
  const [mode, setModeState] = useState("auto");
  const [permRequest, setPermRequest] = useState<PermissionReq | null>(null);

  // ── UI toggles ──
  const [rightPanel, setRightPanel] = useState(true);
  const [outputPanel, setOutputPanel] = useState(false);
  const [outputContent, setOutputContent] = useState("");
  const [outputLang, setOutputLang] = useState("plaintext");
  const [outputPath, setOutputPath] = useState("");

  // ── Right panel data ──
  const [agents, setAgents] = useState<AgentItem[]>([]);
  const [agentStatuses, setAgentStatuses] = useState<Record<string, string>>({});
  const [tasks, setTasks] = useState<TaskItem[]>([]);
  const [files, setFiles] = useState<FileNode[]>([]);
  const [mediaAssets, setMediaAssets] = useState<any[]>([]);
  const [mediaFilter, setMediaFilter] = useState("all");
  const [expandedDirs, setExpandedDirs] = useState<Set<string>>(new Set(["."]));

  // ── SSE handler ──
  const handleEventRef = useRef<(data: any) => void>((data) => {
    if (data.type === "text" && data.data) {
      const chunk = typeof data.data === "string" ? data.data : (data.data.content || "");
      streamingContentRef.current += chunk;
      setStreamingMsg((p) => p ? { ...p, content: p.content + chunk } : p);
    }
    if (data.type === "tool_use") {
      const name = data.data?.tool_name || "tool";
      const args = data.data?.arguments || "";
      streamingContentRef.current += `\n\n_🔧 **${name}** executing_\n`;
      setStreamingMsg((p) => p ? { ...p, content: p.content + `\n\n_🔧 **${name}** executing_\n` } : p);
      // Show inline code block with args if present
      if (args && args !== "{}") {
        const displayArgs = typeof args === "string" ? args : JSON.stringify(args, null, 2);
        streamingContentRef.current += `\`\`\`json\n${displayArgs}\n\`\`\`\n`;
        setStreamingMsg((p) => p ? { ...p, content: p.content + `\`\`\`json\n${displayArgs}\n\`\`\`\n` } : p);
      }
    }
    if (data.type === "tool_result") {
      const name = data.data?.tool_name || "tool";
      const output = data.data?.output || "";
      const error = data.data?.error;
      const duration = data.data?.duration_ms;
      const status = error ? "❌" : "✅";
      const statusText = error ? `Error: ${error}` : `Done (${duration || "?"}ms)`;
      streamingContentRef.current += `_${status} ${name}: ${statusText}_\n\n`;
      setStreamingMsg((p) => p ? { ...p, content: p.content + `_${status} ${name}: ${statusText}_\n\n` } : p);
    }
    if (data.type === "turn_done" || data.type === "error") {
      setIsGenerating(false);
      const finalContent = streamingContentRef.current;
      streamingContentRef.current = "";
      setStreamingMsg(null);
      setConversations((c) => c.map((conv, i) =>
        i === activeIdxRef.current
          ? { ...conv, messages: [...conv.messages, { id: genId(), role: "assistant", content: finalContent }] }
          : conv
      ));
      fetchTasks();
      // If AI generated code, show it in output panel
      const codeMatch = finalContent.match(/```(\w+)?\n([\s\S]*?)```/);
      if (codeMatch) {
        setOutputContent(codeMatch[2]);
        setOutputLang(codeMatch[1] || "plaintext");
        setOutputPath("Generated Code");
        setOutputPanel(true);
      }
    }
    if (data.type === "permission_request") {
      setPermRequest({
        task_id: data.data?.task_id || "", tool_name: data.data?.tool_name || "",
        task_name: data.data?.task_name || "", session_id: data.data?.session_id || "",
      });
    }
    if (data.type === "permission_result") {
      setPermRequest(null);
      const ok = data.data?.approved === "true";
      setStreamingMsg((p) => p ? { ...p, content: p.content + (ok ? "\n\n_✅ Approved_\n" : "\n\n_❌ Rejected_\n") } : p);
    }
    if (data.type === "agent_status") {
      const st = data.data?.status || "";
      const agentType = data.data?.agent_type || "";
      const task = data.data?.task || "";
      const result = data.data?.result || "";
      // Update agent status in the panel
      setAgentStatuses((prev) => ({ ...prev, [agentType]: st }));
      // Refresh tasks when agent completes
      if (st === "completed" || st === "failed") { fetchTasks(); }
      // Show inline status
      const statusMsg = st === "running" ? `_🤖 **${agentType}** agent: ${task}_\n` :
        st === "completed" ? `_✅ **${agentType}** agent completed_\n` :
        st === "failed" ? `_❌ **${agentType}** agent failed: ${result}_\n` : "";
      if (statusMsg) {
        streamingContentRef.current += `\n\n${statusMsg}`;
        setStreamingMsg((p) => p ? { ...p, content: p.content + `\n\n${statusMsg}` } : p);
      }
    }
  });

  // ── Data fetchers ──
  const fetchConvs = useCallback(async () => {
    try {
      const d = await api("/v1/conversations", { skipAuth: true });
      if (Array.isArray(d?.data) && d.data.length > 0) {
        setConversations(d.data.map((c: any) => ({ id: c.id, title: c.title || "Chat", messages: [], sessionId: c.id })));
        setIsLoggedIn(true);
      }
    } catch {}
  }, []);

  const fetchTasks = useCallback(async () => {
    try {
      const d = await api("/v1/tasks", { skipAuth: true });
      if (Array.isArray(d?.data)) setTasks(d.data.slice(0, 30));
    } catch {}
  }, []);

  const fetchAgents = useCallback(async () => {
    try {
      const d = await api("/v1/agents", { skipAuth: true });
      if (Array.isArray(d?.data)) setAgents(d.data);
    } catch {}
  }, []);

  const fetchFiles = useCallback(async () => {
    try {
      const r = await fetch(apiUrl("/api/editor/files"));
      const d = await r.json();
      if (d.data?.files) setFiles(d.data.files);
    } catch {}
  }, []);

  const fetchMedia = useCallback(async (category?: string) => {
    try {
      const params = category && category !== "all" ? `?category=${category}` : "";
      const d = await api(`/v1/media${params}`, { skipAuth: true });
      if (Array.isArray(d?.data)) setMediaAssets(d.data);
    } catch {}
  }, []);

  useEffect(() => { fetchConvs(); fetchTasks(); fetchAgents(); fetchFiles(); fetchMedia(); }, []);
  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: "smooth" }); }, [activeConv.messages, streamingMsg]);

  // ── SSE setup ──
  useEffect(() => {
    let es: EventSource | null = null;
    const connect = () => {
      setConnStatus("connecting");
      es = new EventSource(apiUrl("/events?client_id=" + clientId));
      es.onopen = () => setConnStatus("connected");
      es.onmessage = (e) => {
        if (e.data.startsWith(": ping")) return;
        try { handleEventRef.current(JSON.parse(e.data)); } catch {}
      };
      es.onerror = () => { setConnStatus("disconnected"); es?.close(); setTimeout(connect, 3000); };
    };
    connect();
    return () => es?.close();
  }, []);

  // ── Actions ──
  const sendMessage = useCallback(async () => {
    const text = input.trim();
    if (!text || isGenerating) return;
    setInput("");
    setIsGenerating(true);
    const userMsg: ChatMessage = { id: genId(), role: "user", content: text };
    const asstMsg: ChatMessage = { id: genId(), role: "assistant", content: "" };
    setStreamingMsg(asstMsg);
    setConversations((prev) => prev.map((c, i) => i === activeIdx ? { ...c, messages: [...c.messages, userMsg] } : c));
    try { await api("/submit", { method: "POST", body: JSON.stringify({ content: text, session_id: activeConv.sessionId }) }); }
    catch { setIsGenerating(false); }
  }, [input, isGenerating, activeIdx, activeConv.sessionId]);

  const newChat = async () => {
    const conv: Conversation = { id: genId(), title: `Chat ${conversations.length + 1}`, messages: [], sessionId: genId() + genId() };
    if (isLoggedIn) {
      try { await api("/v1/conversations", { method: "POST", body: JSON.stringify({ id: conv.sessionId, title: conv.title }) }); } catch {}
    }
    setConversations((prev) => [...prev, conv]);
    setActiveIdx(conversations.length);
  };

  const switchConv = async (idx: number) => {
    setActiveIdx(idx);
    const conv = conversations[idx];
    if (isLoggedIn && conv.messages.length === 0 && conv.sessionId) {
      try {
        const d = await api(`/v1/conversations/${conv.sessionId}`, { skipAuth: true });
        const msgs = (d?.data?.messages || []).map((m: any) => ({ id: m.id, role: m.role, content: m.content }));
        if (msgs.length > 0) setConversations((prev) => prev.map((c, i) => i === idx ? { ...c, messages: msgs } : c));
      } catch {}
    }
  };

  const switchMode = async (m: string) => {
    setModeState(m);
    try { await api("/mode", { method: "POST", body: JSON.stringify({ mode: m, session_id: activeConv.sessionId }) }); } catch {}
  };
  const doApprove = async (tid: string) => { try { await api("/approve", { method: "POST", body: JSON.stringify({ task_id: tid }) }); } catch {} };
  const doReject = async (tid: string) => { try { await api("/reject", { method: "POST", body: JSON.stringify({ task_id: tid }) }); } catch {} };

  const deleteConversation = async (idx: number) => {
    const conv = conversations[idx];
    if (!conv) return;
    try { await api(`/v1/conversations/${conv.sessionId}`, { method: "DELETE", skipAuth: true }); } catch {}
    const newConvs = conversations.filter((_, i) => i !== idx);
    if (newConvs.length === 0) {
      // Create a fresh default conversation
      const fresh: Conversation = { id: genId(), title: "Chat 1", messages: [], sessionId: genId() + genId() };
      setConversations([fresh]);
      setActiveIdx(0);
    } else {
      setConversations(newConvs);
      if (activeIdx >= idx && activeIdx > 0) setActiveIdx(activeIdx - 1);
    }
  };

  const [renamingIdx, setRenamingIdx] = useState(-1);
  const [renameValue, setRenameValue] = useState("");
  const startRename = (idx: number) => { setRenamingIdx(idx); setRenameValue(conversations[idx]?.title || ""); };
  const commitRename = async (idx: number) => {
    const conv = conversations[idx];
    if (!conv || !renameValue.trim()) { setRenamingIdx(-1); return; }
    const newTitle = renameValue.trim();
    setConversations((prev) => prev.map((c, i) => i === idx ? { ...c, title: newTitle } : c));
    setRenamingIdx(-1);
    try { await api("/v1/conversations", { method: "POST", body: JSON.stringify({ id: conv.sessionId, title: newTitle }) }); } catch {}
  };

  const openFile = async (path: string) => {
    try {
      const r = await fetch(apiUrl(`/api/editor/read?path=${encodeURIComponent(path)}`));
      const d = await r.json();
      const content = d.data?.content || d.content || "";
      setOutputContent(content);
      setOutputLang(path.split(".").pop() || "plaintext");
      setOutputPath(path);
      setOutputPanel(true);
    } catch {}
  };

  const dispatchToAgent = async (agentType: string, taskDesc?: string) => {
    const task = taskDesc || input.trim();
    if (!task) return;
    setInput("");
    setAgentStatuses((prev) => ({ ...prev, [agentType]: "running" }));
    try {
      await api("/v1/agents/dispatch", {
        method: "POST",
        body: JSON.stringify({
          session_id: activeConv.sessionId,
          agent_type: agentType,
          task: task,
          user_id: isLoggedIn ? "user" : "",
        }),
      });
    } catch {}
  };

  const runEnterpriseTool = async (name: string, toolLabel: string) => {
    try {
      const r = await api("/v1/tools/execute", { method: "POST", body: JSON.stringify({ name, input: {} }) });
      toast.success(`${toolLabel}: ${r?.data?.output?.slice(0, 80) || "done"}`);
      fetchTasks();
    } catch (e: any) { toast.error(e.message); }
  };

  // ── File tree renderer ──
  const renderTree = (nodes: FileNode[], depth = 0): React.ReactNode => {
    return nodes.map((node) => (
      <div key={node.path}>
        <div className="flex items-center gap-1 px-2 py-0.5 cursor-pointer text-xs rounded hover:bg-gray-200 dark:hover:bg-gray-700"
          style={{ paddingLeft: `${depth * 14 + 4}px` }}
          onClick={() => {
            if (node.type === "dir") {
              setExpandedDirs((p) => { const n = new Set(p); n.has(node.path) ? n.delete(node.path) : n.add(node.path); return n; });
            } else { openFile(node.path); }
          }}>
          <span>{node.type === "dir" ? (expandedDirs.has(node.path) ? "📂" : "📁") : "📄"}</span>
          <span className="truncate">{node.name}</span>
        </div>
        {node.type === "dir" && expandedDirs.has(node.path) && node.children && renderTree(node.children, depth + 1)}
      </div>
    ));
  };

  // ── Status helpers ──
  const taskDot = (s: string) =>
    s === "running" ? "bg-blue-400 animate-pulse" : s === "completed" ? "bg-green-400" : s === "failed" ? "bg-red-400" : "bg-gray-300";

  const modeCls: Record<string, string> = {
    ask: "bg-amber-100 text-amber-700 dark:bg-amber-900/50 dark:text-amber-300",
    auto: "bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300",
    yolo: "bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300",
  };

  const rightTabs = [
    { id: "agents", label: "Agents", icon: Bot },
    { id: "media", label: "Media", icon: FileCode },
    { id: "tasks", label: "Tasks", icon: ListTodo },
    { id: "tools", label: "Tools", icon: Building2 },
  ];

  const toolShortcuts = [
    { name: "collab_task_create", icon: "📋", label: "Task" },
    { name: "collab_wiki_create", icon: "📝", label: "Wiki" },
    { name: "collab_okr_create", icon: "🎯", label: "OKR" },
    { name: "brain_query", icon: "🔍", label: "Query" },
    { name: "brain_decision", icon: "🧠", label: "Decide" },
    { name: "support_ticket_create", icon: "🎫", label: "Ticket" },
    { name: "marketing_campaign_create", icon: "📢", label: "Campaign" },
  ];

  return (
    <div className="flex flex-col h-screen bg-gray-50 dark:bg-gray-900">
      <Toaster />

      {/* ══ Header ══ */}
      <header className="h-10 bg-white dark:bg-gray-800 border-b dark:border-gray-700 flex items-center px-3 gap-2 shrink-0">
        <span className="text-sm font-bold text-blue-600 dark:text-blue-400">⚡ MiniCC V2</span>
        <div className="flex gap-0.5 bg-gray-100 dark:bg-gray-700 rounded-lg p-0.5">
          {["ask", "auto", "yolo"].map((m) => (
            <button key={m} onClick={() => switchMode(m)}
              className={`px-2.5 py-0.5 text-[10px] font-semibold rounded-md transition-all ${
                mode === m ? modeCls[m] : "text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
              }`}>
              {m === "ask" ? "❓Ask" : m === "auto" ? "▶Auto" : "🔥YOLO"}
            </button>
          ))}
        </div>
        <div className="flex-1" />
        <button onClick={() => setRightPanel(!rightPanel)} className="text-gray-400 hover:text-gray-600 p-1">
          {rightPanel ? <PanelRightClose className="h-4 w-4" /> : <PanelRightOpen className="h-4 w-4" />}
        </button>
        {isLoggedIn ? (
          <span className="text-[10px] text-green-600">Logged in</span>
        ) : (
          <a href="/login" className="text-[10px] text-blue-500 hover:text-blue-600 flex items-center gap-1">
            <LogIn className="h-3 w-3" /> Log in
          </a>
        )}
      </header>

      {/* ══ Body ══ */}
      <div className="flex flex-1 overflow-hidden">

        {/* ── Left: Conversations ── */}
        <div className="w-48 bg-white dark:bg-gray-800 border-r dark:border-gray-700 flex flex-col shrink-0">
          <div className="p-2 border-b dark:border-gray-700">
            <Button variant="outline" size="sm" className="w-full gap-1 text-xs" onClick={newChat}>
              <Plus className="h-3 w-3" /> New Chat
            </Button>
          </div>
          <ScrollArea className="flex-1">
            {conversations.map((conv, i) => (
              <div key={conv.id}
                className={`group flex items-center gap-1 px-2.5 py-1.5 cursor-pointer text-xs border-l-2 transition-colors ${
                  i === activeIdx ? "bg-blue-50 dark:bg-blue-950 border-l-blue-500" : "border-l-transparent hover:bg-gray-50 dark:hover:bg-gray-800"
                }`}>
                <div className="flex-1 min-w-0" onClick={() => switchConv(i)} onDoubleClick={() => startRename(i)}>
                  {renamingIdx === i ? (
                    <input autoFocus className="w-full text-xs px-1 py-0.5 border rounded dark:bg-gray-700 dark:border-gray-600 outline-none"
                      value={renameValue} onChange={(e) => setRenameValue(e.target.value)}
                      onBlur={() => commitRename(i)}
                      onKeyDown={(e) => { if (e.key === "Enter") commitRename(i); if (e.key === "Escape") setRenamingIdx(-1); }}
                      onClick={(e) => e.stopPropagation()} />
                  ) : (
                    <span className="truncate text-gray-700 dark:text-gray-300 flex items-center gap-1">
                      <MessageSquare className="h-3 w-3 text-gray-400 shrink-0" />
                      {conv.title}
                    </span>
                  )}
                </div>
                <button onClick={(e) => { e.stopPropagation(); deleteConversation(i); }}
                  className="shrink-0 opacity-0 group-hover:opacity-100 text-gray-400 hover:text-red-500 transition-all p-0.5">
                  ✕
                </button>
              </div>
            ))}
          </ScrollArea>
          <div className="p-2 border-t dark:border-gray-700 flex items-center gap-2 text-[10px] text-gray-400">
            <div className={`w-1.5 h-1.5 rounded-full ${connStatus === "connected" ? "bg-green-500" : connStatus === "connecting" ? "bg-amber-500" : "bg-red-500"}`} />
            <span>{connStatus}</span>
          </div>
        </div>

        {/* ── Center: Chat + Output ── */}
        <div className="flex-1 flex flex-col min-w-0">
          {/* Chat */}
          <div className="flex-1 overflow-auto p-3">
            <div className="max-w-4xl mx-auto space-y-3">
              {activeConv.messages.length === 0 && !streamingMsg && (
                <div className="text-center py-20 text-gray-400">
                  <div className="text-5xl mb-3">⚡</div>
                  <p className="text-base font-medium text-gray-600 dark:text-gray-300">MiniCC V2</p>
                  <p className="text-xs mt-1">Enterprise AI Agent Platform</p>
                </div>
              )}
              {activeConv.messages.map((msg) => (
                <Card key={msg.id} className={`p-3 ${msg.role === "user" ? "bg-blue-50 dark:bg-blue-950 border-blue-200 dark:border-blue-800" : ""}`}>
                  <p className="text-[10px] font-medium text-gray-400 mb-1 uppercase">{msg.role}</p>
                  <div className="text-sm prose prose-sm dark:prose-invert max-w-none">
                    {msg.content ? <MarkdownRenderer content={msg.content} /> : <span className="text-gray-300 italic">pending...</span>}
                  </div>
                </Card>
              ))}
              {streamingMsg && (
                <Card className="p-3 bg-gray-50 dark:bg-gray-800/50">
                  <p className="text-[10px] font-medium text-gray-400 mb-1 uppercase">assistant</p>
                  <div className="text-sm"><MarkdownRenderer content={streamingMsg.content || "▊"} /></div>
                </Card>
              )}
              <div ref={bottomRef} />
            </div>
          </div>

          {/* Output panel */}
          {outputPanel && (
            <div className="border-t dark:border-gray-700 flex flex-col" style={{ height: "35%" }}>
              <div className="h-7 bg-gray-100 dark:bg-gray-800 flex items-center px-3 gap-2 shrink-0 border-b dark:border-gray-700">
                <FileCode className="h-3.5 w-3.5 text-gray-500" />
                <span className="text-xs font-medium text-gray-600 dark:text-gray-300 truncate">{outputPath || "Preview"}</span>
                <Badge variant="outline" className="text-[10px]">{outputLang}</Badge>
                <div className="flex-1" />
                <button onClick={() => setOutputPanel(false)} className="text-gray-400 hover:text-gray-600 p-0.5"><Minimize2 className="h-3 w-3" /></button>
              </div>
              <div className="flex-1 overflow-hidden">
                <MonacoEditor key={outputPath} value={outputContent} path={outputPath} language={outputLang} onChange={() => {}} readOnly />
              </div>
            </div>
          )}

          {/* Input */}
          <div className="border-t dark:border-gray-700 p-3 bg-white dark:bg-gray-800">
            <div className="max-w-4xl mx-auto flex gap-2">
              <Textarea value={input} onChange={(e) => setInput(e.target.value)}
                placeholder="Ask AI to code, analyze, search, or automate..." rows={1} className="min-h-[36px] text-sm"
                onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); sendMessage(); } }} />
              {isGenerating ? (
                <Button variant="destructive" size="icon" onClick={() => fetch(apiUrl("/cancel"), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ session_id: activeConv.sessionId }) })}>
                  <Square className="h-4 w-4" />
                </Button>
              ) : (
                <Button onClick={sendMessage} disabled={!input.trim()}>
                  <Send className="h-4 w-4" />
                </Button>
              )}
            </div>
          </div>
        </div>

        {/* ── Right Panel ── */}
        {rightPanel && (
          <div className="w-72 bg-white dark:bg-gray-800 border-l dark:border-gray-700 flex flex-col shrink-0">
            <Tabs defaultValue="agents" className="flex-1 flex flex-col">
              <TabsList className="px-2 pt-2 justify-start gap-0.5">
                {rightTabs.map((t) => (
                  <TabsTrigger key={t.id} value={t.id} className="text-[10px] px-2 py-1">
                    <t.icon className="h-3 w-3 mr-1" />{t.label}
                  </TabsTrigger>
                ))}
              </TabsList>

              {/* Agents tab — real data from /v1/agents */}
              <TabsContent value="agents" className="flex-1 p-2 space-y-2 overflow-auto mt-0">
                {agents.length === 0 ? (
                  <div className="text-xs text-gray-400 text-center py-8"><Loader2 className="h-4 w-4 animate-spin mx-auto mb-2" />Loading agents...</div>
                ) : (
                  agents.map((a) => {
                    const st = agentStatuses[a.type] || "idle";
                    const stColor = st === "running" ? "bg-blue-100 text-blue-700 animate-pulse" :
                      st === "completed" ? "bg-green-100 text-green-700" :
                      st === "failed" ? "bg-red-100 text-red-700" : "bg-gray-100 text-gray-500";
                    return (
                      <Card key={a.type} className="p-3 hover:shadow-sm transition-shadow cursor-pointer" onClick={() => dispatchToAgent(a.type)}>
                        <div className="flex items-center justify-between mb-1">
                          <span className="text-sm font-medium">{a.type === "code" ? "💻" : a.type === "knowledge" ? "📚" : a.type === "rpa" ? "🤖" : "🔧"} {a.name}</span>
                          <span className={`text-[10px] px-1.5 py-0.5 rounded-full ${stColor}`}>{st}</span>
                        </div>
                        <p className="text-[10px] text-gray-500">{a.description}</p>
                      </Card>
                    );
                  })
                )}
              </TabsContent>

              {/* Media Library tab */}
              <TabsContent value="media" className="flex-1 flex flex-col overflow-auto mt-0">
                {/* Category filters */}
                <div className="flex gap-1 p-2 flex-wrap border-b dark:border-gray-700">
                  {[
                    { id: "all", label: "All" },
                    { id: "writing", label: "Writing" },
                    { id: "image", label: "Images" },
                    { id: "translation", label: "Translation" },
                    { id: "office", label: "Office" },
                    { id: "code", label: "Code" },
                  ].map((c) => (
                    <button key={c.id} onClick={() => { setMediaFilter(c.id); fetchMedia(c.id); }}
                      className={`text-[10px] px-2 py-0.5 rounded-full transition-colors ${
                        mediaFilter === c.id ? "bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300" : "text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-700"
                      }`}>
                      {c.label}
                    </button>
                  ))}
                </div>
                {/* Media grid */}
                <div className="flex-1 overflow-auto p-2">
                  {mediaAssets.length === 0 ? (
                    <div className="text-xs text-gray-400 text-center py-8">
                      <p className="text-lg mb-1">📦</p>
                      <p>No media yet</p>
                      <p className="text-[10px] mt-1">AI-generated content appears here</p>
                    </div>
                  ) : (
                    <div className="grid grid-cols-2 gap-2">
                      {mediaAssets.map((asset) => (
                        <div key={asset.id} className="p-2 rounded-lg border dark:border-gray-700 hover:shadow-sm cursor-pointer transition-shadow"
                          onClick={() => {
                            if (asset.type === "image" && asset.file_path) {
                              setOutputContent(`![${asset.name}](/${asset.file_path})`);
                              setOutputLang("markdown");
                            } else {
                              setOutputContent(asset.content || "");
                              setOutputLang(asset.type === "code" ? "plaintext" : "markdown");
                            }
                            setOutputPath(asset.name);
                            setOutputPanel(true);
                          }}>
                          <div className="text-[10px] text-gray-400 mb-1 flex items-center gap-1">
                            <span>{asset.type === "image" ? "🖼" : asset.type === "text" ? "📝" : asset.type === "code" ? "💻" : "📄"}</span>
                            <span className="font-medium text-gray-600 dark:text-gray-300 truncate">{asset.name}</span>
                          </div>
                          {asset.type === "image" && asset.file_path ? (
                            <img src={`/${asset.file_path}`} alt={asset.name} className="w-full h-20 object-cover rounded" />
                          ) : (
                            <p className="text-[10px] text-gray-500 line-clamp-3">{asset.content?.slice(0, 120) || "No preview"}</p>
                          )}
                          <div className="flex items-center gap-1 mt-1">
                            <span className="text-[9px] text-gray-400">{asset.category || asset.type}</span>
                            <span className="text-[9px] text-gray-400 ml-auto">{Math.ceil(asset.size / 100) / 10}k</span>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </TabsContent>

              {/* Tasks tab — real data from /v1/tasks */}
              <TabsContent value="tasks" className="flex-1 overflow-auto p-2 mt-0">
                {tasks.length === 0 ? (
                  <div className="text-xs text-gray-400 text-center py-8">
                    <p className="text-lg mb-1">⚡</p>
                    <p>No tasks yet</p>
                  </div>
                ) : (
                  <div className="space-y-1">
                    {tasks.map((t) => (
                      <div key={t.id} className="flex items-center gap-2 px-2 py-1.5 text-xs rounded hover:bg-gray-50 dark:hover:bg-gray-700">
                        <div className={`w-1.5 h-1.5 rounded-full ${taskDot(t.status)}`} />
                        <span className="flex-1 truncate">{t.payload?.task || t.type}</span>
                        <Badge variant="outline" className={`text-[10px] ${
                          t.status === "running" ? "text-blue-600" : t.status === "completed" ? "text-green-600" : t.status === "failed" ? "text-red-600" : "text-gray-500"
                        }`}>{t.status}</Badge>
                      </div>
                    ))}
                  </div>
                )}
              </TabsContent>

              {/* Tools tab — real via /v1/tools/execute */}
              <TabsContent value="tools" className="flex-1 overflow-auto p-2 mt-0">
                <div className="grid grid-cols-2 gap-2">
                  {toolShortcuts.map((tool) => (
                    <button key={tool.name} onClick={() => runEnterpriseTool(tool.name, tool.label)}
                      className="flex flex-col items-center gap-1 p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors text-center">
                      <span className="text-lg">{tool.icon}</span>
                      <span className="text-[10px] text-gray-600 dark:text-gray-400">{tool.label}</span>
                    </button>
                  ))}
                </div>
              </TabsContent>
            </Tabs>
          </div>
        )}
      </div>

      {/* ══ Permission Dialog ══ */}
      {permRequest && (
        <div className="fixed inset-0 bg-black/30 z-50 flex items-center justify-center">
          <Card className="w-96 p-6 shadow-2xl" onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center gap-2 text-amber-600 mb-3">
              <AlertTriangle className="h-5 w-5" />
              <span className="font-semibold text-sm">Permission Required</span>
            </div>
            <p className="text-sm text-gray-600 dark:text-gray-300 mb-1">The AI wants to execute:</p>
            <p className="text-sm font-mono bg-gray-100 dark:bg-gray-800 rounded p-2 mb-4">{permRequest.task_name}</p>
            <div className="flex gap-2 justify-end">
              <Button variant="outline" size="sm" onClick={() => doReject(permRequest.task_id)}>
                <X className="h-3 w-3 mr-1" /> Reject
              </Button>
              <Button size="sm" onClick={() => doApprove(permRequest.task_id)}>
                <Check className="h-3 w-3 mr-1" /> Approve
              </Button>
            </div>
          </Card>
        </div>
      )}
    </div>
  );
}
