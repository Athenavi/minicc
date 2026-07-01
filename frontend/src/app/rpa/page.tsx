"use client";

import { useState, useEffect } from "react";
import { useToolRunner } from "@/hooks/useToolRunner";

interface ToolCard {
  name: string;
  description: string;
  category: string;
}

export default function RPADashboard() {
  const [tools, setTools] = useState<ToolCard[]>([]);
  const [activeTab, setActiveTab] = useState("browser");
  const [result, setResult] = useState("");
  const { runTool } = useToolRunner();

  useEffect(() => {
    fetch("http://localhost:8000/api/tools")
      .then((r) => r.json())
      .then((data) => {
        const filtered = data.tools.filter(
          (t: any) => t.category === "WEB" || t.category === "SHELL" || t.name.startsWith("desktop_") || t.name.startsWith("excel_") || t.name.startsWith("word_") || t.name.startsWith("email_")
        );
        setTools(filtered);
      })
      .catch(() => {});
  }, []);

  const handleRun = async (name: string, input: Record<string, any> = {}) => {
    setResult("Running...");
    const out = await runTool(name, input);
    setResult(out);
  };

  const tabs = [
    { id: "browser", label: "🌐 Browser", desc: "Navigate, click, fill forms, screenshots" },
    { id: "desktop", label: "🖥️ Desktop", desc: "Mouse, keyboard, OCR, windows, clipboard" },
    { id: "office", label: "📊 Office", desc: "Excel, Word, Email" },
    { id: "shell", label: "🐚 Shell", desc: "Run commands, scripts" },
  ];

  const filtered = tools.filter((t) => {
    if (activeTab === "browser") return t.name.startsWith("browser_") || t.name.startsWith("web_");
    if (activeTab === "desktop") return t.name.startsWith("desktop_") || t.name.startsWith("ocr_") || t.name.startsWith("clipboard_") || t.name.startsWith("window_");
    if (activeTab === "office") return t.name.startsWith("excel_") || t.name.startsWith("word_") || t.name.startsWith("email_");
    if (activeTab === "shell") return t.name === "bash";
    return false;
  });

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <div className="max-w-6xl mx-auto">
        <h1 className="text-2xl font-bold mb-2 text-gray-800 dark:text-gray-100">🤖 RPA Control Center</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mb-6">V0.2 — Browser Automation · Desktop Control · Office Tools</p>

        <div className="flex gap-1 mb-6 bg-white dark:bg-gray-800 rounded-lg p-1 shadow-sm">
          {tabs.map((tab) => (
            <button key={tab.id} onClick={() => setActiveTab(tab.id)}
              className={`flex-1 px-4 py-2.5 rounded-md text-sm font-medium transition-all ${
                activeTab === tab.id ? "bg-blue-600 text-white shadow" : "text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
              }`}
            >
              <span className="block">{tab.label}</span>
              <span className="block text-xs opacity-70 font-normal mt-0.5">{tab.desc}</span>
            </button>
          ))}
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-6">
          {filtered.map((tool) => (
            <div key={tool.name} className="bg-white dark:bg-gray-800 rounded-lg p-4 shadow-sm border border-gray-200 dark:border-gray-700">
              <div className="flex items-start justify-between">
                <div className="flex-1 min-w-0">
                  <h3 className="font-medium text-sm text-gray-800 dark:text-gray-100">{tool.name}</h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400 mt-1 line-clamp-2">{tool.description}</p>
                </div>
                <button onClick={() => handleRun(tool.name)} className="px-3 py-1 bg-blue-600 text-white text-xs rounded hover:bg-blue-700 shrink-0 ml-2">Run</button>
              </div>
            </div>
          ))}
          {filtered.length === 0 && (
            <div className="col-span-full text-center py-12 text-gray-400"><p>No tools found</p></div>
          )}
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
