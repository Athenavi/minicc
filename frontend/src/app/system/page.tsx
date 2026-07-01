"use client";

import { useState } from "react";

export default function SystemDashboard() {
  const [result, setResult] = useState("");

  const runTool = async (name: string) => {
    setResult(`Running ${name}...`);
    try {
      const resp = await fetch("http://localhost:8000/api/submit", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message: `/tools ${name}`, session_id: "system" }),
      });
      const data = await resp.json();
      setResult(data.output || `Done: ${name}`);
    } catch (err: any) { setResult(`Error: ${err.message}`); }
  };

  const sections = [
    {
      title: "🔬 Self-Awareness", desc: "V0.7 — Monitor, Analyze, Profile",
      tools: ["self_monitor", "self_arch_analyze", "self_profiler", "self_improve_plan", "self_health"],
      color: "border-l-blue-500",
    },
    {
      title: "💊 Self-Healing", desc: "V0.7 — Auto-fix, Optimize, Refactor",
      tools: ["self_heal_bug", "self_optimize", "self_refactor", "self_deps", "self_test_augment", "self_status"],
      color: "border-l-green-500",
    },
    {
      title: "📚 Continuous Learning", desc: "V0.7 — Feedback, Knowledge, Experience",
      tools: ["learn_from_feedback", "learn_knowledge", "learn_experience"],
      color: "border-l-purple-500",
    },
    {
      title: "🧬 Self-Evolution", desc: "V0.7 — Design, Implement, Marketplace",
      tools: ["evolve_design", "evolve_implement", "evolve_register", "evolve_marketplace", "evolve_transfer"],
      color: "border-l-amber-500",
    },
    {
      title: "⚖️ AI Constitution", desc: "V0.7 — Check, Violations",
      tools: ["constitution_check", "constitution_violations"],
      color: "border-l-red-500",
    },
    {
      title: "🏛️ AI Civilization", desc: "V0.8 — Citizens, DAO, Economy, Culture, Diplomacy",
      tools: ["ai_citizen_create", "ai_citizen_list", "dao_propose", "dao_list", "economy_mint", "culture_art", "diplomacy_treaty"],
      color: "border-l-cyan-500",
    },
  ];

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <div className="max-w-6xl mx-auto">
        <h1 className="text-2xl font-bold mb-1 text-gray-800 dark:text-gray-100">🔬 System Console</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mb-6">V0.7-V0.8 — Self-monitoring · Evolution · Civilization</p>

        {/* Health Overview */}
        <div className="grid grid-cols-5 gap-3 mb-6">
          {[
            { label: "Architecture", score: 85, color: "bg-green-500" },
            { label: "Performance", score: 72, color: "bg-amber-500" },
            { label: "Security", score: 95, color: "bg-green-500" },
            { label: "Coverage", score: 68, color: "bg-amber-500" },
            { label: "Debt", score: 4.2, color: "bg-blue-500", suffix: "%" },
          ].map((m) => (
            <div key={m.label} className="bg-white dark:bg-gray-800 rounded-lg p-4 shadow-sm border border-gray-200 dark:border-gray-700 text-center">
              <div className="text-2xl font-bold text-gray-800 dark:text-gray-100">{m.score}{m.suffix || ""}</div>
              <div className={`h-1.5 rounded-full ${m.color} mt-2 opacity-70`} style={{ width: `${typeof m.score === 'number' ? Math.min(100, m.score) : 70}%` }} />
              <div className="text-xs text-gray-500 dark:text-gray-400 mt-1">{m.label}</div>
            </div>
          ))}
        </div>

        {/* Sections */}
        <div className="space-y-4">
          {sections.map((section) => (
            <div key={section.title} className={`bg-white dark:bg-gray-800 rounded-lg p-4 shadow-sm border-l-4 ${section.color} border border-gray-200 dark:border-gray-700`}>
              <div className="flex items-center justify-between mb-2">
                <div>
                  <h3 className="font-semibold text-sm text-gray-800 dark:text-gray-100">{section.title}</h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400">{section.desc}</p>
                </div>
                <button onClick={() => section.tools.forEach(runTool)} className="px-3 py-1 bg-blue-600 text-white text-xs rounded hover:bg-blue-700">
                  Run All
                </button>
              </div>
              <div className="flex flex-wrap gap-2">
                {section.tools.map((name) => (
                  <button key={name} onClick={() => runTool(name)}
                    className="px-3 py-1.5 bg-gray-100 dark:bg-gray-700 text-xs font-mono rounded hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 transition-colors"
                  >
                    {name}
                  </button>
                ))}
              </div>
            </div>
          ))}
        </div>

        {result && (
          <div className="mt-6 bg-white dark:bg-gray-800 rounded-lg p-4 shadow-sm border border-gray-200 dark:border-gray-700">
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
