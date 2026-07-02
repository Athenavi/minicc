"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { MarkdownRenderer } from "@/components/chat/MarkdownRenderer";
import { api, apiUrl } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Textarea } from "@/components/ui/textarea";
import { Toaster, toast } from "sonner";
import { Send, Square, Plus, MessageSquare } from "lucide-react";

interface ChatMessage { id: string; role: string; content: string; toolCalls?: any[]; }
interface Conversation { id: string; title: string; messages: ChatMessage[]; sessionId: string; }

function genId() { return Math.random().toString(36).slice(2, 10); }

export default function Home() {
  const [conversations, setConversations] = useState<Conversation[]>([{ id: genId(), title: "Chat 1", messages: [], sessionId: genId() + genId() }]);
  const [activeIdx, setActiveIdx] = useState(0);
  const [input, setInput] = useState("");
  const [connStatus, setConnStatus] = useState<"connecting" | "connected" | "disconnected">("disconnected");
  const [streamingMsg, setStreamingMsg] = useState<ChatMessage | null>(null);
  const [isGenerating, setIsGenerating] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);

  // SSE event handler — updates streaming message as events arrive
  const handleEventRef = useRef<(data: any) => void>((data) => {
    if (data.kind === "text" && data.data) {
      setStreamingMsg((prev) => prev ? { ...prev, content: prev.content + data.data } : prev);
    }
    if (data.kind === "tool_dispatch") {
      setStreamingMsg((prev) => prev ? { ...prev, content: (prev.content || "") + `\n\n_🔧 Using tool: ${data.data?.tool_name || "unknown"}_\n` } : prev);
    }
    if (data.kind === "turn_done" || data.kind === "error") {
      setIsGenerating(false);
      setStreamingMsg((prev) => {
        if (prev) {
          // Finalize: move streaming message to conversation
          setConversations((c) => c.map((conv, i) =>
            i === activeIdxRef.current ? { ...conv, messages: [...conv.messages, prev] } : conv
          ));
        }
        return null;
      });
    }
  });

  // Keep ref to activeIdx for use in SSE handler closure
  const activeIdxRef = useRef(activeIdx);
  activeIdxRef.current = activeIdx;

  // Auto-scroll to bottom
  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: "smooth" }); }, [activeConv.messages, streamingMsg]);

  const activeConv = conversations[activeIdx];

  // SSE
  useEffect(() => {
    let es: EventSource | null = null;
    function connect() {
      setConnStatus("connecting");
      es = new EventSource(apiUrl("/events"));
      es.onopen = () => setConnStatus("connected");
      es.onmessage = (e) => {
        if (e.data.startsWith(": ping")) return;
        try { handleEventRef.current(JSON.parse(e.data)); } catch {}
      };
      es.onerror = () => { setConnStatus("disconnected"); es?.close(); setTimeout(connect, 3000); };
    }
    connect();
    return () => es?.close();
  }, []);

  const sendMessage = useCallback(async () => {
    const text = input.trim();
    if (!text || isGenerating) return;
    setInput("");
    setIsGenerating(true);

    const userMsg: ChatMessage = { id: genId(), role: "user", content: text };
    const assistantMsg: ChatMessage = { id: genId(), role: "assistant", content: "" };
    setStreamingMsg(assistantMsg);
    setConversations((prev) => prev.map((c, i) => i === activeIdx ? { ...c, messages: [...c.messages, userMsg] } : c));

    try {
      await api("/submit", {
        method: "POST",
        body: JSON.stringify({ content: text, session_id: activeConv.sessionId }),
      });
    } catch { setIsGenerating(false); }
  }, [input, isGenerating, activeIdx, activeConv.sessionId]);

  const newChat = () => {
    setConversations((prev) => [...prev, { id: genId(), title: `Chat ${prev.length + 1}`, messages: [], sessionId: genId() + genId() }]);
    setActiveIdx(conversations.length);
  };

  const statusColor = connStatus === "connected" ? "bg-green-500" : connStatus === "connecting" ? "bg-amber-500" : "bg-red-500";

  return (
    <div className="flex h-[calc(100vh-48px)] bg-gray-50 dark:bg-gray-900">
      <Toaster />
      {/* Sidebar */}
      <div className="w-56 bg-white dark:bg-gray-800 border-r dark:border-gray-700 flex flex-col">
        <div className="p-3 border-b dark:border-gray-700">
          <Button variant="outline" size="sm" className="w-full gap-1" onClick={newChat}>
            <Plus className="h-3.5 w-3.5" /> New Chat
          </Button>
        </div>
        <ScrollArea className="flex-1">
          {conversations.map((conv, i) => (
            <div key={conv.id} onClick={() => setActiveIdx(i)}
              className={`flex items-center gap-2 px-3 py-2 cursor-pointer text-xs border-l-2 transition-colors ${
                i === activeIdx ? "bg-blue-50 dark:bg-blue-950 border-l-blue-500" : "border-l-transparent hover:bg-gray-50 dark:hover:bg-gray-800"
              }`}>
              <MessageSquare className="h-3.5 w-3.5 text-gray-400 shrink-0" />
              <span className="truncate text-gray-700 dark:text-gray-300">{conv.title}</span>
            </div>
          ))}
        </ScrollArea>
        <div className="p-3 border-t dark:border-gray-700 flex items-center gap-2 text-xs text-gray-500">
          <div className={`w-2 h-2 rounded-full ${statusColor}`} />
          {connStatus}
        </div>
      </div>

      {/* Main */}
      <div className="flex-1 flex flex-col">
        <ScrollArea className="flex-1 p-4">
          <div className="max-w-3xl mx-auto space-y-4">
            {activeConv.messages.length === 0 && !streamingMsg && (
              <div className="text-center py-20 text-gray-400">
                <div className="text-5xl mb-4">⚡</div>
                <p className="text-lg font-medium text-gray-600 dark:text-gray-300">MiniCC V2</p>
                <p className="text-sm mt-1">Start a conversation</p>
              </div>
            )}
            {activeConv.messages.map((msg) => (
              <Card key={msg.id} className={`p-4 ${msg.role === "user" ? "bg-blue-50 dark:bg-blue-950 border-blue-200 dark:border-blue-800" : ""}`}>
                <p className="text-[10px] font-medium text-gray-400 mb-1 uppercase">{msg.role}</p>
                <div className="text-sm prose prose-sm dark:prose-invert max-w-none">
                  {msg.content ? <MarkdownRenderer content={msg.content} /> : <span className="text-gray-300 italic">pending...</span>}
                </div>
              </Card>
            ))}
            {streamingMsg && (
              <Card className="p-4 bg-gray-50 dark:bg-gray-800/50">
                <p className="text-[10px] font-medium text-gray-400 mb-1 uppercase">assistant</p>
                <div className="text-sm"><MarkdownRenderer content={streamingMsg.content || "▊"} /></div>
              </Card>
            )}
            <div ref={bottomRef} />
          </div>
        </ScrollArea>

        {/* Input */}
        <div className="border-t dark:border-gray-700 p-4 bg-white dark:bg-gray-800">
          <div className="max-w-3xl mx-auto flex gap-2">
            <Textarea value={input} onChange={(e) => setInput(e.target.value)}
              placeholder="Type a message..." rows={1} className="min-h-[40px]"
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
    </div>
  );
}
