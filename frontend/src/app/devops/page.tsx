"use client";

import { useState } from "react";
import { useToolRunner } from "@/hooks/useToolRunner";

export default function DevOpsDashboard() {
  const [activeTab, setActiveTab] = useState("pm");
  const [result, setResult] = useState("");
  const { runTool } = useToolRunner();

  const handleRun = async (name: string) => {
    setResult("Running...");
    setResult(await runTool(name));
  };

  const tabs = [
    { id: "pm", label: "📋 PM", tools: ["prd_generate", "tech_design", "task_decompose", "requirement_validate"] },
    { id: "dev", label: "👷 Dev", tools: ["architect_agent", "coding_agent", "test_agent", "review_agent"] },
    { id: "cicd", label: "🚀 CI/CD", tools: ["ci_generate", "deploy_service", "monitor_setup"] },
    { id: "ops", label: "🔧 Ops", tools: ["error_analyze", "self_heal", "goal_set", "goal_status"] },
  ];

  const tab = tabs.find((t) => t.id === activeTab);

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <div className="max-w-6xl mx-auto">
        <h1 className="text-2xl font-bold mb-1 text-gray-800 dark:text-gray-100">🚀 DevOps Platform</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mb-6">V0.5 — AI-powered development lifecycle</p>
        <div className="flex gap-1 mb-6 bg-white dark:bg-gray-800 rounded-lg p-1 shadow-sm overflow-x-auto">
          {tabs.map((t) => (
            <button key={t.id} onClick={() => setActiveTab(t.id)}
              className={`flex-1 px-4 py-2.5 rounded-md text-sm font-medium whitespace-nowrap ${
                activeTab === t.id ? "bg-blue-600 text-white shadow" : "text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
              }`}>{t.label} ({t.tools.length})</button>
          ))}
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-6">
          {tab?.tools.map((name) => (
            <div key={name} className="bg-white dark:bg-gray-800 rounded-lg p-4 shadow-sm border border-gray-200 dark:border-gray-700">
              <div className="flex items-center justify-between">
                <span className="text-sm font-mono text-gray-800 dark:text-gray-100">{name}</span>
                <button onClick={() => handleRun(name)} className="px-3 py-1 bg-blue-600 text-white text-xs rounded hover:bg-blue-700 shrink-0 ml-2">Run</button>
              </div>
            </div>
          ))}
        </div>
        {result && (
          <div className="bg-white dark:bg-gray-800 rounded-lg p-4 shadow-sm border border-gray-200">
            <div className="flex items-center justify-between mb-2">
              <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">Result</h3>
              <button onClick={() => setResult("")} className="text-xs text-gray-400 hover:text-gray-600">Clear</button>
            </div>
            <pre className="text-xs font-mono bg-gray-50 dark:bg-gray-900 p-3 rounded max-h-60 overflow-auto whitespace-pre-wrap">{result}</pre>
          </div>
        )}
      </div>
    </div>
  );
}
