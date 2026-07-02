"use client";

import { useState, useEffect } from "react";
import { api, apiUrl } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Toaster, toast } from "sonner";
import { Bot, BookOpen, Monitor, Wrench, Send } from "lucide-react";

const AGENTS = [
  { type: "code", name: "Code Agent", desc: "编写、修改、分析代码", icon: Bot, color: "border-l-blue-500" },
  { type: "knowledge", name: "Knowledge Agent", desc: "检索知识库和文档", icon: BookOpen, color: "border-l-purple-500" },
  { type: "rpa", name: "RPA Agent", desc: "控制浏览器和桌面应用", icon: Monitor, color: "border-l-green-500" },
  { type: "tool", name: "Tool Agent", desc: "调用 MCP 和外部 API", icon: Wrench, color: "border-l-amber-500" },
];

export default function AgentsPage() {
  const [task, setTask] = useState("");
  const [agentType, setAgentType] = useState("auto");
  const [result, setResult] = useState("");
  const [loading, setLoading] = useState(false);

  const handleDispatch = async () => {
    if (!task.trim()) return;
    setLoading(true);
    setResult("");
    try {
      const d = await api("/v1/chat", {
        method: "POST",
        body: JSON.stringify({ session_id: "agents", message: agentType === "auto" ? `Dispatch: ${task}` : `Dispatch to ${agentType}: ${task}` }),
      });
      setResult(d.data?.response || "Done");
      toast.success("Task dispatched");
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <Toaster richColors />
      <div className="max-w-5xl mx-auto space-y-6">
        <h1 className="text-xl font-bold">🤖 Agent Console</h1>

        {/* Agent cards */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          {AGENTS.map((a) => (
            <Card key={a.type} className={`border-l-4 ${a.color}`}>
              <CardContent className="p-4">
                <a.icon className="h-6 w-6 mb-2 text-gray-600 dark:text-gray-300" />
                <CardTitle className="text-sm">{a.name}</CardTitle>
                <p className="text-xs text-gray-500 mt-1">{a.desc}</p>
              </CardContent>
            </Card>
          ))}
        </div>

        {/* Dispatch */}
        <Card>
          <CardContent className="p-4 space-y-3">
            <div className="flex gap-2">
              <select value={agentType} onChange={(e) => setAgentType(e.target.value)}
                className="px-3 py-2 text-sm border rounded-lg dark:bg-gray-800 dark:border-gray-700 outline-none">
                <option value="auto">🤖 Auto-route</option>
                {AGENTS.map((a) => <option key={a.type} value={a.type}>{a.name}</option>)}
              </select>
            </div>
            <div className="flex gap-2">
              <Textarea value={task} onChange={(e) => setTask(e.target.value)} placeholder="Describe the task..." rows={2}
                onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); handleDispatch(); } }} />
              <Button onClick={handleDispatch} disabled={loading || !task.trim()} className="self-end">
                <Send className="h-4 w-4 mr-1" /> {loading ? "..." : "Dispatch"}
              </Button>
            </div>
            {result && (
              <div className="p-3 bg-green-50 dark:bg-green-950 rounded-lg border">
                <pre className="text-xs whitespace-pre-wrap font-sans text-green-800 dark:text-green-200">{result}</pre>
              </div>
            )}
          </CardContent>
        </Card>

        {/* Sessions */}
        <Card>
          <CardContent className="p-8 text-center text-sm text-gray-400">
            <p className="text-2xl mb-2">⏳</p>
            <p>No active sessions</p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
