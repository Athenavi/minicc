"use client";

import { useState } from "react";
import { useToolRunner } from "@/hooks/useToolRunner";

interface ToolDef {
  name: string; description: string;
}

const MODULES = [
  {
    id: "collab", label: "Collaboration", icon: "👥", desc: "Tasks · Wiki · OKR · Chat · Meetings",
    color: "border-l-purple-500",
    tools: [
      { name: "collab_task_create", description: "Create a project task" },
      { name: "collab_wiki_create", description: "Create a wiki page" },
      { name: "collab_wiki_search", description: "Search wiki pages" },
      { name: "collab_okr_create", description: "Set an OKR goal" },
      { name: "collab_message_send", description: "Send a team message" },
      { name: "collab_meeting_summary", description: "Summarize meeting notes" },
    ],
  },
  {
    id: "brain", label: "Enterprise Brain", icon: "🧠", desc: "Query · Decision · Predict · Compliance",
    color: "border-l-red-500",
    tools: [
      { name: "brain_query", description: "Cross-module natural language query" },
      { name: "brain_decision", description: "AI-powered decision support" },
      { name: "brain_predict", description: "Predictive analytics" },
      { name: "brain_compliance", description: "Compliance check" },
    ],
  },
  {
    id: "support", label: "Customer Service", icon: "🎫", desc: "Tickets · Chatbot · KB · Campaigns",
    color: "border-l-amber-500",
    tools: [
      { name: "support_ticket_create", description: "Create a support ticket" },
      { name: "support_kb_search", description: "Search knowledge base" },
      { name: "support_chatbot_reply", description: "AI chatbot auto-reply" },
      { name: "marketing_campaign_create", description: "Create a campaign" },
      { name: "marketing_abtest", description: "Run A/B test" },
    ],
  },
];

export default function EnterprisePage() {
  const [activeModule, setActiveModule] = useState("collab");
  const [result, setResult] = useState("");
  const [executing, setExecuting] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const { runTool } = useToolRunner();

  const current = MODULES.find((m) => m.id === activeModule);
  const filteredTools = current?.tools.filter(
    (t) => !searchQuery || t.name.toLowerCase().includes(searchQuery.toLowerCase()) || t.description.toLowerCase().includes(searchQuery.toLowerCase())
  ) || [];

  const handleRun = async (name: string) => {
    setExecuting(name); setResult("");
    setResult(await runTool(name));
    setExecuting(null);
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <div className="max-w-6xl mx-auto">
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-xl font-bold text-gray-800 dark:text-gray-100">🏢 Enterprise OS</h1>
            <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">Collaboration · Brain · Customer Service</p>
          </div>
          <input className="w-48 px-3 py-2 text-sm border rounded-lg dark:bg-gray-800 dark:border-gray-600 dark:text-gray-100"
            placeholder="Search tools..." value={searchQuery} onChange={(e) => setSearchQuery(e.target.value)} />
        </div>

        {/* Module tabs */}
        <div className="flex gap-1 mb-6 bg-white dark:bg-gray-800 rounded-lg p-1 shadow-sm">
          {MODULES.map((mod) => (
            <button key={mod.id} onClick={() => { setActiveModule(mod.id); setSearchQuery(""); }}
              className={`flex-1 px-4 py-2.5 rounded-md text-sm font-medium transition-all ${
                activeModule === mod.id ? "bg-blue-600 text-white shadow" : "text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
              }`}>
              <span className="block">{mod.icon} {mod.label}</span>
              <span className="block text-[10px] opacity-70 font-normal mt-0.5">{mod.desc}</span>
            </button>
          ))}
        </div>

        {/* Tool grid */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {filteredTools.map((tool) => (
            <div key={tool.name} className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700 hover:shadow-md transition-all">
              <div className="flex items-start justify-between">
                <div className="flex-1 min-w-0">
                  <h3 className="font-medium text-sm text-gray-800 dark:text-gray-100">{tool.name}</h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">{tool.description}</p>
                </div>
                <button onClick={() => handleRun(tool.name)} disabled={executing === tool.name}
                  className="px-3 py-1.5 bg-blue-600 text-white text-xs font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-colors shrink-0 ml-2">
                  {executing === tool.name ? "⟳" : "▶ Run"}
                </button>
              </div>
            </div>
          ))}
          {filteredTools.length === 0 && (
            <div className="col-span-full text-center py-16">
              <p className="text-gray-400 text-lg mb-1">🔍 No tools found</p>
              <p className="text-gray-400 text-sm">Try a different module or search term</p>
            </div>
          )}
        </div>

        {/* Result */}
        {result && (
          <div className="mt-6 bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700">
            <div className="flex items-center justify-between mb-2">
              <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">📋 Result</h3>
              <button onClick={() => setResult("")} className="text-xs text-gray-400 hover:text-red-500">✕ Clear</button>
            </div>
            <pre className="text-xs font-mono bg-gray-50 dark:bg-gray-900 p-3 rounded-lg max-h-48 overflow-auto whitespace-pre-wrap">{result}</pre>
          </div>
        )}
      </div>
    </div>
  );
}
