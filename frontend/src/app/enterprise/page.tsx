"use client";

import { useState } from "react";
import { useToolRunner } from "@/hooks/useToolRunner";

export default function EnterpriseDashboard() {
  const [activeTab, setActiveTab] = useState("crm");
  const [result, setResult] = useState("");
  const { runTool } = useToolRunner();

  const handleRun = async (name: string, input: Record<string, any> = {}) => {
    setResult("Running...");
    setResult(await runTool(name, input));
  };

  const tabs = [
    { id: "crm", label: "🏢 CRM", tools: ["crm_contact_create", "crm_contact_search", "crm_lead_create", "crm_pipeline_create", "crm_pipeline_list", "crm_forecast"] },
    { id: "erp", label: "📦 ERP", tools: ["erp_supplier_create", "erp_inventory_add", "erp_inventory_check", "erp_order_create", "erp_invoice_create"] },
    { id: "collab", label: "👥 Collab", tools: ["collab_task_create", "collab_wiki_create", "collab_okr_create", "collab_message_send", "collab_meeting_summary"] },
    { id: "support", label: "🎫 Support", tools: ["support_ticket_create", "support_kb_search", "support_chatbot_reply", "marketing_campaign_create", "marketing_abtest"] },
    { id: "brain", label: "🧠 Brain", tools: ["brain_query", "brain_decision", "brain_predict", "brain_compliance"] },
  ];

  const currentTab = tabs.find((t) => t.id === activeTab);

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <div className="max-w-6xl mx-auto">
        <h1 className="text-2xl font-bold mb-1 text-gray-800 dark:text-gray-100">🏢 Enterprise OS</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mb-6">V0.6 — CRM · ERP · Collaboration · Support · Enterprise Brain</p>

        <div className="flex gap-1 mb-6 bg-white dark:bg-gray-800 rounded-lg p-1 shadow-sm overflow-x-auto">
          {tabs.map((tab) => (
            <button key={tab.id} onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-2 rounded-md text-sm font-medium whitespace-nowrap ${
                activeTab === tab.id ? "bg-blue-600 text-white shadow" : "text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
              }`}>{tab.label} ({tab.tools.length})</button>
          ))}
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-6">
          {currentTab?.tools.map((name) => (
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
