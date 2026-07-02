"use client";

import { useState, useEffect, useCallback } from "react";
import { useToolRunner } from "@/hooks/useToolRunner";

interface ToolCard {
  name: string;
  description: string;
  category: string;
}

interface ParamField {
  name: string;
  type: string;
  required: boolean;
  description: string;
  default?: any;
}

interface ToolDetail extends ToolCard {
  parameters?: ParamField[];
}

// ─── Category Config ───────────────────────────────────────────────────────

const CATEGORIES = [
  {
    id: "browser",
    label: "🌐 Browser",
    desc: "Navigation, clicks, forms, screenshots, cookies",
    filter: (t: ToolCard) => t.name.startsWith("browser_") || t.name.startsWith("web_") || t.name.startsWith("ai_"),
    tools: [] as ToolCard[],
  },
  {
    id: "extract",
    label: "📄 Data Extraction",
    desc: "Tables, lists, text, JSON-LD, metadata",
    filter: (t: ToolCard) => t.name.startsWith("web_extract_"),
    tools: [] as ToolCard[],
  },
  {
    id: "desktop",
    label: "🖥️ Desktop",
    desc: "Mouse, keyboard, OCR, windows, clipboard",
    filter: (t: ToolCard) => t.name.startsWith("desktop_") || t.name.startsWith("ocr_") || t.name.startsWith("clipboard_") || t.name.startsWith("window_"),
    tools: [] as ToolCard[],
  },
  {
    id: "office",
    label: "📊 Office",
    desc: "Excel, Word, Email automation",
    filter: (t: ToolCard) => t.name.startsWith("excel_") || t.name.startsWith("word_") || t.name.startsWith("email_"),
    tools: [] as ToolCard[],
  },
  {
    id: "database",
    label: "🗄️ Database",
    desc: "SQL queries, connections, migrations",
    filter: (t: ToolCard) => t.name.startsWith("db_") || t.name.startsWith("database_"),
    tools: [] as ToolCard[],
  },
  {
    id: "api",
    label: "🔌 API",
    desc: "REST, GraphQL, webhook integration",
    filter: (t: ToolCard) => t.name.startsWith("api_") || t.name.startsWith("rest_"),
    tools: [] as ToolCard[],
  },
  {
    id: "workflow",
    label: "⚡ Workflow",
    desc: "Create, run, schedule automation workflows",
    filter: (t: ToolCard) => t.name.startsWith("workflow_") || t.name.startsWith("scheduler_"),
    tools: [] as ToolCard[],
  },
];

// ─── Stats Card ────────────────────────────────────────────────────────────

function StatCard({ label, value, color }: { label: string; value: number | string; color: string }) {
  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg p-4 shadow-sm border border-gray-200 dark:border-gray-700">
      <p className="text-xs text-gray-500 dark:text-gray-400">{label}</p>
      <p className={`text-2xl font-bold mt-1 ${color}`}>{value}</p>
    </div>
  );
}

// ─── Tool Parameter Input ──────────────────────────────────────────────────

function ParamInput({ param, value, onChange }: { param: ParamField; value: string; onChange: (v: string) => void }) {
  const isMultiline = param.type === "text" || param.description.length > 60;
  return (
    <div className="mb-2">
      <label className="block text-xs text-gray-500 dark:text-gray-400 mb-0.5">
        {param.name}
        {param.required && <span className="text-red-400 ml-0.5">*</span>}
      </label>
      {isMultiline ? (
        <textarea
          className="w-full p-1.5 text-xs border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 font-mono"
          rows={2}
          placeholder={param.description}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      ) : (
        <input
          className="w-full p-1.5 text-xs border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 font-mono"
          placeholder={param.description}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      )}
    </div>
  );
}

// ─── Main Dashboard ────────────────────────────────────────────────────────

export default function RPADashboard() {
  const [tools, setTools] = useState<ToolCard[]>([]);
  const [activeTab, setActiveTab] = useState("browser");
  const [result, setResult] = useState("");
  const [executing, setExecuting] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [showParams, setShowParams] = useState<string | null>(null);
  const [paramValues, setParamValues] = useState<Record<string, string>>({});
  const { runTool } = useToolRunner();

  // Show only last 10 lines of result
  const displayResult = result.split("\n").slice(-10).join("\n");

  // Load tools from API
  useEffect(() => {
    fetch("http://localhost:8000/api/tools")
      .then((r) => r.json())
      .then((data) => {
        const allTools: ToolCard[] = (data.tools || data || []).filter(
          (t: any) => t.category === "WEB" || t.category === "SHELL" || t.name.startsWith("desktop_") ||
            t.name.startsWith("excel_") || t.name.startsWith("word_") || t.name.startsWith("email_") ||
            t.name.startsWith("db_") || t.name.startsWith("api_") || t.name.startsWith("workflow_") ||
            t.name.startsWith("scheduler_") || t.name.startsWith("recorder_") || t.name.startsWith("ai_")
        );
        setTools(allTools);
      })
      .catch(() => {});
  }, []);

  const activeCategory = CATEGORIES.find((c) => c.id === activeTab);
  const filteredTools = tools.filter((t) => {
    if (!activeCategory) return false;
    if (!activeCategory.filter(t)) return false;
    if (searchQuery && !t.name.toLowerCase().includes(searchQuery.toLowerCase()) &&
        !t.description.toLowerCase().includes(searchQuery.toLowerCase())) return false;
    return true;
  });

  // Count tools per category
  const categoryCounts = CATEGORIES.map((c) => ({
    ...c,
    count: tools.filter(c.filter).length,
  }));

  const stats = [
    { label: "Total Tools", value: tools.length, color: "text-blue-600 dark:text-blue-400" },
    { label: "Browser", value: tools.filter((t) => t.name.startsWith("browser_")).length, color: "text-green-600 dark:text-green-400" },
    { label: "Desktop", value: tools.filter((t) => t.name.startsWith("desktop_") || t.name.startsWith("ocr_")).length, color: "text-purple-600 dark:text-purple-400" },
    { label: "Office", value: tools.filter((t) => t.name.startsWith("excel_") || t.name.startsWith("word_") || t.name.startsWith("email_")).length, color: "text-amber-600 dark:text-amber-400" },
  ];

  const handleRun = useCallback(async (name: string, input?: Record<string, any>) => {
    setExecuting(name);
    setResult("");
    const out = await runTool(name, input || {});
    setResult(out);
    setExecuting(null);
    setShowParams(null);
  }, [runTool]);

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <div className="max-w-7xl mx-auto">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-2xl font-bold text-gray-800 dark:text-gray-100">🤖 RPA Control Center</h1>
            <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">V0.2 — Enterprise RPA Platform | {tools.length} tools available</p>
          </div>
          <div className="relative">
            <input
              className="w-64 px-4 py-2 text-sm border rounded-lg dark:bg-gray-800 dark:border-gray-600 dark:text-gray-100 pl-9"
              placeholder="Search tools..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
            <span className="absolute left-3 top-2.5 text-gray-400 text-sm">🔍</span>
          </div>
        </div>

        {/* Stats */}
        <div className="grid grid-cols-4 gap-4 mb-6">
          {stats.map((s) => <StatCard key={s.label} {...s} />)}
        </div>

        {/* Category Tabs */}
        <div className="flex gap-1 mb-6 bg-white dark:bg-gray-800 rounded-lg p-1 shadow-sm overflow-x-auto">
          {categoryCounts.map((cat) => (
            <button key={cat.id} onClick={() => setActiveTab(cat.id)}
              className={`flex-1 px-4 py-2.5 rounded-md text-sm font-medium transition-all whitespace-nowrap ${
                activeTab === cat.id
                  ? "bg-blue-600 text-white shadow"
                  : "text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
              }`}
            >
              <span className="block">{cat.label}</span>
              <span className="block text-xs opacity-70 font-normal mt-0.5">{cat.desc}</span>
              <span className={`inline-block text-xs mt-1 px-1.5 py-0.5 rounded ${
                activeTab === cat.id ? "bg-white/20" : "bg-gray-100 dark:bg-gray-700 text-gray-500"
              }`}>
                {cat.count} tools
              </span>
            </button>
          ))}
        </div>

        {/* Tool Grid */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 mb-6">
          {filteredTools.map((tool) => (
            <div key={tool.name} className="bg-white dark:bg-gray-800 rounded-lg p-4 shadow-sm border border-gray-200 dark:border-gray-700 hover:shadow-md transition-shadow">
              <div className="flex items-start justify-between">
                <div className="flex-1 min-w-0">
                  <h3 className="font-medium text-sm text-gray-800 dark:text-gray-100">{tool.name}</h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400 mt-1 line-clamp-2">{tool.description}</p>
                </div>
              </div>
              <div className="flex items-center gap-2 mt-3">
                <button
                  onClick={() => handleRun(tool.name)}
                  disabled={executing === tool.name}
                  className="flex-1 px-3 py-1.5 bg-blue-600 text-white text-xs font-medium rounded hover:bg-blue-700 disabled:opacity-50 transition-colors"
                >
                  {executing === tool.name ? "⟳ Running..." : "▶ Run"}
                </button>
              </div>
            </div>
          ))}
          {filteredTools.length === 0 && (
            <div className="col-span-full text-center py-16">
              <p className="text-gray-400 text-lg mb-2">🔍 No tools found</p>
              <p className="text-gray-400 text-sm">Try a different category or search term</p>
            </div>
          )}
        </div>

        {/* Result Panel */}
        {result && (
          <div className="bg-white dark:bg-gray-800 rounded-lg p-4 shadow-sm border border-gray-200 dark:border-gray-700">
            <div className="flex items-center justify-between mb-2">
              <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">📋 Execution Result</h3>
              <div className="flex gap-2">
                <button onClick={() => { navigator.clipboard.writeText(result); }} className="text-xs text-gray-400 hover:text-gray-600">📋 Copy</button>
                <button onClick={() => setResult("")} className="text-xs text-gray-400 hover:text-red-500">✕ Clear</button>
              </div>
            </div>
            <pre className="text-xs font-mono bg-gray-50 dark:bg-gray-900 p-3 rounded max-h-48 overflow-auto whitespace-pre-wrap">{displayResult}</pre>
          </div>
        )}
      </div>
    </div>
  );
}
