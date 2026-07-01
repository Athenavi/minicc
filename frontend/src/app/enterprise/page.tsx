"use client";

import { useState } from "react";

export default function EnterpriseDashboard() {
  const [activeTab, setActiveTab] = useState("crm");
  const [result, setResult] = useState("");

  const runTool = async (name: string) => {
    setResult(`Running ${name}...`);
    try {
      const resp = await fetch("http://localhost:8000/api/submit", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message: `/tools ${name}`, session_id: "enterprise" }),
      });
      const data = await resp.json();
      setResult(data.output || `Executed: ${name}`);
    } catch (err: any) {
      setResult(`Error: ${err.message}`);
    }
  };

  const tabs = [
    { id: "crm", label: "🏢 CRM", tools: ["crm_contact_create", "crm_contact_search", "crm_lead_create", "crm_pipeline_create", "crm_pipeline_list", "crm_forecast"] },
    { id: "erp", label: "📦 ERP", tools: ["erp_supplier_create", "erp_inventory_add", "erp_inventory_check", "erp_order_create", "erp_invoice_create"] },
    { id: "collab", label: "👥 Collab", tools: ["collab_task_create", "collab_wiki_create", "collab_okr_create", "collab_message_send", "collab_meeting_summary"] },
    { id: "support", label: "🎫 Support", tools: ["support_ticket_create", "support_kb_search", "support_chatbot_reply", "marketing_campaign_create", "marketing_abtest"] },
    { id: "brain", label: "🧠 Brain", tools: ["brain_query", "brain_decision", "brain_predict", "brain_compliance"] },
  ];

  const currentTab = tabs.find((t) => t.id === activeTab);
  const widgets: Record<string, { label: string; value: string; color: string }[]> = {
    crm: [
      { label: "Total Contacts", value: "1,247", color: "bg-blue-500" },
      { label: "Active Deals", value: "42", color: "bg-green-500" },
      { label: "Pipeline Value", value: "$847K", color: "bg-purple-500" },
      { label: "Win Rate", value: "68%", color: "bg-amber-500" },
    ],
    erp: [
      { label: "Inventory Items", value: "3,891", color: "bg-teal-500" },
      { label: "Low Stock Alerts", value: "7", color: "bg-red-500" },
      { label: "Pending Orders", value: "23", color: "bg-orange-500" },
      { label: "Suppliers", value: "156", color: "bg-indigo-500" },
    ],
    collab: [
      { label: "Active Projects", value: "12", color: "bg-cyan-500" },
      { label: "Open Tasks", value: "87", color: "bg-rose-500" },
      { label: "Wiki Pages", value: "234", color: "bg-emerald-500" },
      { label: "Team Members", value: "28", color: "bg-violet-500" },
    ],
    support: [
      { label: "Open Tickets", value: "34", color: "bg-amber-500" },
      { label: "Avg Response", value: "2.3m", color: "bg-green-500" },
      { label: "Satisfaction", value: "94%", color: "bg-blue-500" },
      { label: "KB Articles", value: "156", color: "bg-purple-500" },
    ],
    brain: [
      { label: "Knowledge Nodes", value: "12.4K", color: "bg-indigo-500" },
      { label: "Decisions Made", value: "847", color: "bg-teal-500" },
      { label: "Predictions", value: "93% acc", color: "bg-green-500" },
      { label: "Compliance", value: "100%", color: "bg-emerald-500" },
    ],
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <div className="max-w-6xl mx-auto">
        <h1 className="text-2xl font-bold mb-1 text-gray-800 dark:text-gray-100">🏢 Enterprise OS</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mb-6">V0.6 — CRM · ERP · Collaboration · Support · Enterprise Brain</p>

        {/* KPI Widgets */}
        {widgets[activeTab] && (
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-6">
            {widgets[activeTab].map((w) => (
              <div key={w.label} className="bg-white dark:bg-gray-800 rounded-lg p-4 shadow-sm border border-gray-200 dark:border-gray-700">
                <div className={`w-2 h-8 rounded-full ${w.color} mb-2`} />
                <div className="text-2xl font-bold text-gray-800 dark:text-gray-100">{w.value}</div>
                <div className="text-xs text-gray-500 dark:text-gray-400 mt-1">{w.label}</div>
              </div>
            ))}
          </div>
        )}

        {/* Tabs */}
        <div className="flex gap-1 mb-6 bg-white dark:bg-gray-800 rounded-lg p-1 shadow-sm overflow-x-auto">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-2 rounded-md text-sm font-medium whitespace-nowrap ${
                activeTab === tab.id ? "bg-blue-600 text-white shadow" : "text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
              }`}
            >
              {tab.label} ({tab.tools.length})
            </button>
          ))}
        </div>

        {/* Tools */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 mb-6">
          {currentTab?.tools.map((name) => (
            <div key={name} className="bg-white dark:bg-gray-800 rounded-lg p-4 shadow-sm border border-gray-200 dark:border-gray-700 hover:shadow-md transition-shadow">
              <div className="flex items-center justify-between">
                <span className="text-sm font-mono text-gray-800 dark:text-gray-100">{name}</span>
                <button onClick={() => runTool(name)} className="px-3 py-1 bg-blue-600 text-white text-xs rounded hover:bg-blue-700 shrink-0 ml-2">Run</button>
              </div>
            </div>
          ))}
        </div>

        {/* Result */}
        {result && (
          <div className="bg-white dark:bg-gray-800 rounded-lg p-4 shadow-sm border border-gray-200 dark:border-gray-700">
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
