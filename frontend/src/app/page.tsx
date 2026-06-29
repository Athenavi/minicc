"use client";

import { useState, useCallback, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { MessageList } from "@/components/chat/MessageList";
import { SessionList } from "@/components/sidebar/SessionList";
import { useWebSocket } from "@/hooks/useWebSocket";
import { type Message, type AgentStatus } from "@/lib/types";
import { Send, Square, Menu } from "lucide-react";

export default function Home() {
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [agentStatus, setAgentStatus] = useState<AgentStatus>("idle");
  const [sessionId] = useState(() => crypto.randomUUID());
  const inputRef = useRef<HTMLInputElement>(null);

  const handleMessage = useCallback((data: Record<string, unknown>) => {
    if (data.type === "message_chunk") {
      // Phase 1: handle streaming chunks
    }
  }, []);

  const { status: wsStatus, sendJSON } = useWebSocket({
    sessionId,
    onMessage: handleMessage,
  });

  const handleSend = useCallback(() => {
    const text = input.trim();
    if (!text) return;

    const userMsg: Message = {
      role: "user",
      content: text,
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, userMsg]);
    setInput("");
    setAgentStatus("thinking");

    sendJSON({ type: "user_message", payload: { content: text } });
  }, [input, sendJSON]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const handleStop = useCallback(() => {
    sendJSON({ type: "cancel" });
    setAgentStatus("idle");
  }, [sendJSON]);

  return (
    <div className="flex h-screen overflow-hidden">
      {/* Sidebar */}
      {sidebarOpen && (
        <SessionList
          sessions={[]}
          onSelect={() => {}}
          onNew={() => {
            setMessages([]);
            setAgentStatus("idle");
          }}
          onDelete={() => {}}
          className="w-72 shrink-0 hidden md:flex"
        />
      )}

      {/* Main chat area */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Header */}
        <header className="flex items-center gap-2 px-4 h-12 border-b shrink-0">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="md:hidden"
          >
            <Menu className="w-4 h-4" />
          </Button>
          <h1 className="text-sm font-semibold">MiniCC</h1>
          <div className="flex items-center gap-2 ml-auto">
            <span className={`w-2 h-2 rounded-full ${
              wsStatus === "connected" ? "bg-green-500" : "bg-yellow-500"
            }`} />
            <span className="text-xs text-muted-foreground">
              {wsStatus === "connected" ? "已连接" : "连接中..."}
            </span>
          </div>
        </header>

        {/* Messages */}
        <MessageList messages={messages} className="flex-1" />

        {/* Input area */}
        <div className="border-t p-4 shrink-0">
          <div className="flex gap-2 max-w-4xl mx-auto">
            <Input
              ref={inputRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="输入消息... (Shift+Enter 换行)"
              disabled={agentStatus === "thinking" || agentStatus === "executing"}
              className="flex-1"
            />
            {agentStatus === "idle" ? (
              <Button onClick={handleSend} size="icon" disabled={!input.trim()}>
                <Send className="w-4 h-4" />
              </Button>
            ) : (
              <Button onClick={handleStop} variant="destructive" size="icon">
                <Square className="w-4 h-4" />
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
