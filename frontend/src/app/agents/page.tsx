"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { apiUrl } from "@/lib/api";

interface AgentInfo {
  type: string;
  name: string;
  description: string;
}

interface Session {
  id: string;
  name: string;
  task: string;
  status: string;
  result?: string;
  created_at?: string;
}

const AGENT_ICONS: Record<string, string> = {
  code: "💻",
  knowledge: "📚",
  rpa: "🤖",
  tool: "🔧",
};

const AGENT_COLORS: Record<string, string> = {
  code: "border-l-blue-500",
  knowledge: "border-l-purple-500",
  rpa: "border-l-green-500",
  tool: "border-l-amber-500",
};

export default function AgentsPage() {
  const router = useRouter();
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [dispatchTask, setDispatchTask] = useState("");
  const [dispatchAgent, setDispatchAgent] = useState("auto");
  const [result, setResult] = useState("");
  const [executing, setExecuting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const token = localStorage.getItem("minicc_token");
    if (!token) { router.push("/login"); return; }
    fetchData();
  }, [router]);

  const fetchData = async () => {
    const token = localStorage.getItem("minicc_token");
    try {
      const [agentsRes] = await Promise.all([
        fetch(apiUrl("/v1/tools"), {
          headers: { Authorization: `Bearer ${token}` },
        }).catch(() => null),
      ]);
      // Agent info is from the tool definitions; for now use hardcoded agents
      setAgents([
        { type: "code", name: "Code Agent", description: "编写、修改、分析代码" },
        { type: "knowledge", name: "Knowledge Agent", description: "检索知识库和文档" },
        { type: "rpa", name: "RPA Agent", description: "控制浏览器和桌面应用" },
        { type: "tool", name: "Tool Agent", description: "调用 MCP 和外部 API" },
      ]);
    } catch {}
    setLoading(false);
  };

  const handleDispatch = async () => {
    if (!dispatchTask.trim()) return;
    setExecuting(true);
    setResult("");
    setError("");

    const token = localStorage.getItem("minicc_token");
    try {
      const res = await fetch(apiUrl("/v1/chat"), {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          session_id: "agent-console",
          message: dispatchAgent === "auto"
            ? `Dispatch to best agent: ${dispatchTask}`
            : `Dispatch to ${dispatchAgent} agent: ${dispatchTask}`,
        }),
      });
      const data = await res.json();
      if (data.success) {
        setResult(data.data.response);
      } else {
        setError(data.error || "Dispatch failed");
      }
    } catch (err: any) {
      setError(`Connection failed: ${err.message}`);
    } finally {
      setExecuting(false);
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
        <div className="max-w-5xl mx-auto animate-pulse space-y-4">
          <div className="h-8 bg-gray-200 dark:bg-gray-700 rounded w-48" />
          <div className="grid grid-cols-4 gap-3">
            {[1,2,3,4].map(i => <div key={i} className="h-28 bg-gray-200 dark:bg-gray-700 rounded-xl" />)}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <div className="max-w-5xl mx-auto">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-xl font-bold text-gray-800 dark:text-gray-100">🤖 Agent Console</h1>
            <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">Dispatch tasks to specialized AI agents</p>
          </div>
        </div>

        {/* Agent cards */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
          {agents.map((agent) => (
            <div key={agent.type}
              className={`bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700 border-l-4 ${AGENT_COLORS[agent.type] || "border-l-gray-400"} hover:shadow-md transition-shadow`}>
              <div className="text-2xl mb-2">{AGENT_ICONS[agent.type] || "●"}</div>
              <h3 className="text-sm font-semibold text-gray-800 dark:text-gray-100">{agent.name}</h3>
              <p className="text-xs text-gray-500 dark:text-gray-400 mt-1 line-clamp-2">{agent.description}</p>
            </div>
          ))}
        </div>

        {/* Dispatch panel */}
        <div className="bg-white dark:bg-gray-800 rounded-xl p-6 shadow-sm border border-gray-200 dark:border-gray-700 mb-6">
          <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-4">📤 Dispatch Task</h2>

          <div className="flex gap-3 mb-3">
            <select value={dispatchAgent} onChange={(e) => setDispatchAgent(e.target.value)}
              className="px-3 py-2 text-sm border rounded-lg dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 outline-none focus:ring-2 focus:ring-blue-500">
              <option value="auto">🤖 Auto-route</option>
              {agents.map((a) => (
                <option key={a.type} value={a.type}>{AGENT_ICONS[a.type]} {a.name}</option>
              ))}
            </select>
          </div>

          <div className="flex gap-2">
            <textarea
              value={dispatchTask}
              onChange={(e) => setDispatchTask(e.target.value)}
              className="flex-1 px-3 py-2 text-sm border rounded-lg dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 outline-none focus:ring-2 focus:ring-blue-500 resize-none"
              rows={2}
              placeholder="Describe the task to dispatch..."
              onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); handleDispatch(); } }}
            />
            <button onClick={handleDispatch} disabled={executing || !dispatchTask.trim()}
              className="px-6 py-2 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-all h-fit self-end">
              {executing ? "..." : "▶ Dispatch"}
            </button>
          </div>

          {/* Result */}
          {result && (
            <div className="mt-4 p-3 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg">
              <div className="flex items-center justify-between mb-1">
                <span className="text-[10px] font-medium text-green-600 dark:text-green-400 uppercase">Result</span>
                <button onClick={() => setResult("")} className="text-[10px] text-gray-400 hover:text-red-500">✕</button>
              </div>
              <pre className="text-xs text-green-800 dark:text-green-200 whitespace-pre-wrap font-sans">{result}</pre>
            </div>
          )}
          {error && (
            <div className="mt-4 p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
              <p className="text-xs text-red-600 dark:text-red-400">{error}</p>
            </div>
          )}
        </div>

        {/* Sessions */}
        <div className="bg-white dark:bg-gray-800 rounded-xl p-6 shadow-sm border border-gray-200 dark:border-gray-700">
          <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-4">🔄 Active Sessions</h2>
          <div className="text-center py-8 text-sm text-gray-400">
            <p className="text-2xl mb-2">⏳</p>
            <p>No active sessions</p>
            <p className="text-xs mt-1">Sessions appear when tasks are dispatched</p>
          </div>
        </div>
      </div>
    </div>
  );
}
