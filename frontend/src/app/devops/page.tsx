"use client";

import { useState, useEffect } from "react";
import { useToolRunner } from "@/hooks/useToolRunner";

interface PipelineStage {
  name: string; status: "pending" | "running" | "success" | "failed" | "skipped";
  duration?: string; logs?: string[];
}

interface DeployEnv {
  name: string; version: string; status: string; url: string; updated: string;
}

export default function DevOpsDashboard() {
  const [activeTab, setActiveTab] = useState("pm");
  const [result, setResult] = useState("");
  const [executing, setExecuting] = useState<string | null>(null);
  const [pipeline, setPipeline] = useState<PipelineStage[]>([
    { name: "Lint & Format", status: "success", duration: "12s" },
    { name: "Unit Tests", status: "success", duration: "45s" },
    { name: "Integration Tests", status: "running", duration: "1m 20s" },
    { name: "Build Docker", status: "pending" },
    { name: "Deploy Staging", status: "pending" },
    { name: "E2E Tests", status: "pending" },
    { name: "Deploy Production", status: "pending" },
  ]);
  const [environments, setEnvironments] = useState<DeployEnv[]>([
    { name: "Development", version: "v0.7.2-rc1", status: "✅", url: "http://dev.minicc.local", updated: "2h ago" },
    { name: "Staging", version: "v0.7.1", status: "✅", url: "http://staging.minicc.local", updated: "1d ago" },
    { name: "Production", version: "v0.7.0", status: "✅", url: "https://minicc.ai", updated: "3d ago" },
  ]);

  const { runTool } = useToolRunner();

  const handleRun = async (name: string) => {
    setExecuting(name);
    setResult("");
    const out = await runTool(name);
    setResult(out);
    setExecuting(null);
  };

  const tabs = [
    { id: "pm", label: "📋 Planning", desc: "PRD → Tech Design → Tasks → Validation",
      tools: ["prd_generate", "tech_design", "task_decompose", "requirement_validate"] },
    { id: "dev", label: "👷 Development", desc: "Architecture → Code → Test → Review → Auto-fix",
      tools: ["architect_agent", "coding_agent", "test_agent", "review_agent", "auto_fix", "merge_pr"] },
    { id: "cicd", label: "🚀 CI/CD", desc: "CI Config → Build → Deploy → DB Migration",
      tools: ["ci_generate", "build_service", "deploy_service", "db_migrate"] },
    { id: "ops", label: "🔧 Operations", desc: "Monitor → Alert → Self-heal → Reports",
      tools: ["monitor_status", "error_analyze", "self_heal", "cost_analyze", "daily_report"] },
    { id: "agent", label: "🤖 Long-term Agent", desc: "Goals → Execute → Report → Decisions",
      tools: ["goal_set", "goal_status", "agent_execute", "agent_report"] },
  ];

  const tab = tabs.find((t) => t.id === activeTab);

  const statusIcon = (s: string) => {
    switch (s) {
      case "success": return "✅"; case "failed": return "❌";
      case "running": return "⟳"; case "pending": return "⏳";
      case "skipped": return "⏭"; default: return "⬜";
    }
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <div className="max-w-7xl mx-auto">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-2xl font-bold text-gray-800 dark:text-gray-100">🚀 DevOps Platform</h1>
            <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">V0.5 — AI-powered Development Lifecycle · CI/CD · Operations</p>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-xs text-gray-400">System Health:</span>
            <span className="flex items-center gap-1 text-xs text-green-600 bg-green-50 dark:bg-green-900/30 px-2 py-1 rounded-full">
              <span className="w-1.5 h-1.5 bg-green-500 rounded-full" /> All Systems Operational
            </span>
          </div>
        </div>

        {/* CI/CD Pipeline */}
        {activeTab === "cicd" && (
          <div className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700 mb-6">
            <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">🔄 CI/CD Pipeline</h3>
            <div className="flex items-center gap-2 overflow-x-auto pb-2">
              {pipeline.map((stage) => (
                <div key={stage.name} className={`flex flex-col items-center min-w-[100px] p-3 rounded-lg border ${
                  stage.status === "running" ? "border-blue-400 bg-blue-50 dark:bg-blue-900/20" :
                  stage.status === "success" ? "border-green-300 bg-green-50 dark:bg-green-900/20" :
                  stage.status === "failed" ? "border-red-300 bg-red-50" :
                  "border-gray-200 dark:border-gray-600"
                }`}>
                  <span className="text-lg mb-1">{statusIcon(stage.status)}</span>
                  <span className="text-xs font-medium text-gray-700 dark:text-gray-300 text-center">{stage.name}</span>
                  {stage.duration && <span className="text-[10px] text-gray-400 mt-0.5">{stage.duration}</span>}
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Environments */}
        {activeTab === "ops" && (
          <div className="grid grid-cols-3 gap-3 mb-6">
            {environments.map((env) => (
              <div key={env.name} className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700">
                <div className="flex items-center justify-between mb-2">
                  <h4 className="text-sm font-semibold text-gray-700 dark:text-gray-300">{env.name}</h4>
                  <span className="text-lg">{env.status}</span>
                </div>
                <p className="text-xs font-mono text-gray-500">{env.version}</p>
                <p className="text-xs text-gray-400 mt-1">{env.url}</p>
                <p className="text-[10px] text-gray-400 mt-1">Updated {env.updated}</p>
              </div>
            ))}
          </div>
        )}

        {/* Tab Bar */}
        <div className="flex gap-1 mb-6 bg-white dark:bg-gray-800 rounded-lg p-1 shadow-sm overflow-x-auto">
          {tabs.map((t) => (
            <button key={t.id} onClick={() => setActiveTab(t.id)}
              className={`flex-1 px-4 py-2.5 rounded-md text-sm font-medium transition-all whitespace-nowrap ${
                activeTab === t.id ? "bg-blue-600 text-white shadow" : "text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
              }`}>
              <span className="block">{t.label}</span>
              <span className="block text-[10px] opacity-70 font-normal mt-0.5">{t.desc}</span>
            </button>
          ))}
        </div>

        {/* Tools Grid */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 mb-6">
          {tab?.tools.map((name) => (
            <div key={name} className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700 hover:shadow-md transition-shadow">
              <div className="flex items-center justify-between">
                <div>
                  <span className="text-sm font-mono text-gray-800 dark:text-gray-100">{name}</span>
                  <p className="text-xs text-gray-400 mt-0.5">Click to execute</p>
                </div>
                <button onClick={() => handleRun(name)} disabled={executing === name}
                  className="px-3 py-1.5 bg-blue-600 text-white text-xs font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-colors shrink-0 ml-2">
                  {executing === name ? "⟳" : "▶ Run"}
                </button>
              </div>
            </div>
          ))}
        </div>

        {/* Result */}
        {result && (
          <div className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700">
            <div className="flex items-center justify-between mb-2">
              <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">📋 Result</h3>
              <div className="flex gap-2">
                <button onClick={() => navigator.clipboard.writeText(result)} className="text-xs text-gray-400 hover:text-gray-600">📋 Copy</button>
                <button onClick={() => setResult("")} className="text-xs text-gray-400 hover:text-red-500">✕ Clear</button>
              </div>
            </div>
            <pre className="text-xs font-mono bg-gray-50 dark:bg-gray-900 p-3 rounded-lg max-h-48 overflow-auto whitespace-pre-wrap">{result}</pre>
          </div>
        )}
      </div>
    </div>
  );
}
