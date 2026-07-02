"use client";

import { useState, useEffect } from "react";
import { api, apiUrl } from "@/lib/api";

// ─── Types ─────────────────────────────────────────────────────────────────

interface TraceSpan {
  id: string; name: string; type: string; duration_ms: number;
  status: string; started_at: string; events: any[];
  attributes: Record<string, any>; slow?: boolean;
}

interface EndpointStats {
  name: string; count: number; errors: number; error_rate: number;
  avg_ms: number; p50_ms: number; p95_ms: number; p99_ms: number;
}

interface SystemMetrics {
  total_spans: number; active_spans: number; error_spans: number;
  slow_spans: number; by_type?: Record<string, EndpointStats>;
}

// ─── Helper Components ─────────────────────────────────────────────────────

function MetricCard({ label, value, sub, color = "text-blue-600" }: { label: string; value: string | number; sub?: string; color?: string }) {
  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700">
      <p className="text-xs text-gray-500 dark:text-gray-400 mb-1">{label}</p>
      <p className={`text-2xl font-bold ${color}`}>{value}</p>
      {sub && <p className="text-xs text-gray-400 mt-1">{sub}</p>}
    </div>
  );
}

function ProgressBar({ value, max, color = "bg-blue-500", label }: { value: number; max: number; color?: string; label?: string }) {
  const pct = Math.min(100, (value / max) * 100);
  return (
    <div className="flex items-center gap-2">
      {label && <span className="text-xs text-gray-500 w-20 shrink-0">{label}</span>}
      <div className="flex-1 h-2 bg-gray-100 dark:bg-gray-700 rounded-full overflow-hidden">
        <div className={`h-full rounded-full transition-all duration-500 ${color}`} style={{ width: `${pct}%` }} />
      </div>
      <span className="text-xs text-gray-500 w-12 text-right">{value.toFixed(1)}ms</span>
    </div>
  );
}

// ─── Main Page ─────────────────────────────────────────────────────────────

export default function SystemDashboard() {
  const [result, setResult] = useState("");
  const [activeSection, setActiveSection] = useState<string>("health");
  const [metrics, setMetrics] = useState<SystemMetrics | null>(null);
  const [traces, setTraces] = useState<TraceSpan[]>([]);
  const [endpoints, setEndpoints] = useState<EndpointStats[]>([]);
  const [loading, setLoading] = useState(true);
  const [healthScores, setHealthScores] = useState([
    { label: "Architecture", score: 85, color: "bg-green-500" },
    { label: "Performance", score: 72, color: "bg-amber-500" },
    { label: "Security", score: 95, color: "bg-green-500" },
    { label: "Coverage", score: 68, color: "bg-amber-500" },
    { label: "Debt", score: 4.2, color: "bg-blue-500", suffix: "%" },
  ]);

  // Fetch trace metrics on mount
  useEffect(() => {
    const fetchMetrics = async () => {
      try {
        const [metricsRes, tracesRes] = await Promise.all([
          await api("/metrics").catch(() => null),
          await api("/metrics/traces").catch(() => null),
        ]);
        if (metricsRes?.ok) {
          const data = await metricsRes.json();
          setMetrics(data);
          if (data.by_type) {
            setEndpoints(Object.values(data.by_type));
          }
        }
        if (tracesRes?.ok) {
          const data = await tracesRes.json();
          setTraces(data.traces || []);
        }
      } catch {}
      setLoading(false);
    };
    fetchMetrics();
    const interval = setInterval(fetchMetrics, 10000);
    return () => clearInterval(interval);
  }, []);

  const runTool = async (name: string) => {
    setResult(`Running ${name}...`);
    try {
      const resp = await fetch(apiUrl("/api/submit"), {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message: `/tools ${name}`, session_id: "system" }),
      });
      const data = await resp.json();
      setResult(data.output || `Done: ${name}`);
    } catch (err: any) { setResult(`Error: ${err.message}`); }
  };

  const sections = [
    { id: "health", title: "🔬 Self-Awareness", desc: "V0.7 — Monitor, Analyze, Profile",
      tools: ["self_monitor", "self_arch_analyze", "self_profiler", "self_improve_plan", "self_health"],
      color: "border-l-blue-500" },
    { id: "heal", title: "💊 Self-Healing", desc: "V0.7 — Auto-fix, Optimize, Refactor",
      tools: ["self_heal_bug", "self_optimize", "self_refactor", "self_deps", "self_test_augment"],
      color: "border-l-green-500" },
    { id: "learn", title: "📚 Continuous Learning", desc: "V0.7 — Feedback, Knowledge, Experience",
      tools: ["learn_from_feedback", "learn_knowledge", "learn_experience"],
      color: "border-l-purple-500" },
    { id: "evolve", title: "🧬 Self-Evolution", desc: "V0.7 — Design, Implement, Marketplace",
      tools: ["evolve_design", "evolve_implement", "evolve_register", "evolve_marketplace"],
      color: "border-l-amber-500" },
    { id: "constitution", title: "⚖️ AI Constitution", desc: "V0.7 — Check, Violations",
      tools: ["constitution_check", "constitution_violations"],
      color: "border-l-red-500" },
  ];

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <div className="max-w-7xl mx-auto">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-2xl font-bold text-gray-800 dark:text-gray-100">🔬 System Console</h1>
            <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">V0.7-V0.8 — AI Self-Monitoring · Evolution · Observability</p>
          </div>
          <div className="flex gap-2">
            {["health", "observe", "tools"].map((tab) => (
              <button key={tab} onClick={() => setActiveSection(tab)}
                className={`px-4 py-2 text-sm font-medium rounded-lg transition-all ${
                  activeSection === tab ? "bg-blue-600 text-white shadow" : "bg-white dark:bg-gray-800 text-gray-600 dark:text-gray-300 hover:bg-gray-100"
                }`}>
                {tab === "health" ? "🏥 Health" : tab === "observe" ? "📊 Observe" : "🔧 Tools"}
              </button>
            ))}
          </div>
        </div>

        {activeSection === "health" && (
          <>
            {/* Health Scores */}
            <div className="grid grid-cols-5 gap-3 mb-6">
              {healthScores.map((m) => (
                <div key={m.label} className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700 text-center">
                  <div className="text-2xl font-bold text-gray-800 dark:text-gray-100">{m.score}{m.suffix || ""}</div>
                  <div className={`h-1.5 rounded-full ${m.color} mt-2 opacity-70`} style={{ width: `${typeof m.score === 'number' ? Math.min(100, m.score) : 70}%` }} />
                  <div className="text-xs text-gray-500 dark:text-gray-400 mt-1">{m.label}</div>
                </div>
              ))}
            </div>

            {/* Section Tools */}
            <div className="space-y-4">
              {sections.map((section) => (
                <div key={section.id} className={`bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border-l-4 ${section.color} border border-gray-200 dark:border-gray-700`}>
                  <div className="flex items-center justify-between mb-3">
                    <div>
                      <h3 className="font-semibold text-sm text-gray-800 dark:text-gray-100">{section.title}</h3>
                      <p className="text-xs text-gray-500 dark:text-gray-400">{section.desc}</p>
                    </div>
                    <button onClick={() => section.tools.forEach(runTool)} className="px-3 py-1.5 bg-blue-600 text-white text-xs font-medium rounded-lg hover:bg-blue-700">
                      ▶ Run All
                    </button>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {section.tools.map((name) => (
                      <button key={name} onClick={() => runTool(name)}
                        className="px-3 py-1.5 bg-gray-100 dark:bg-gray-700 text-xs font-mono rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 transition-colors">
                        {name}
                      </button>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </>
        )}

        {activeSection === "observe" && (
          <>
            {/* Metrics Overview */}
            <div className="grid grid-cols-5 gap-3 mb-6">
              <MetricCard label="Total Spans" value={metrics?.total_spans || 0} color="text-blue-600" />
              <MetricCard label="Active" value={metrics?.active_spans || 0} color="text-green-600" sub="In-flight spans" />
              <MetricCard label="Errors" value={metrics?.error_spans || 0} color="text-red-600" sub={metrics?.total_spans ? `${((metrics.error_spans / metrics.total_spans) * 100).toFixed(1)}% error rate` : ""} />
              <MetricCard label="Slow Traces" value={metrics?.slow_spans || 0} color="text-amber-600" sub=">5s duration" />
              <MetricCard label="Health" value={metrics && metrics.error_spans === 0 ? "Good" : "Warning"} color={metrics?.error_spans === 0 ? "text-green-600" : "text-amber-600"} />
            </div>

            {/* Endpoint Latency */}
            <div className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700 mb-6">
              <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-4">📡 Endpoint Latency (p50 / p95 / p99)</h3>
              <div className="space-y-3">
                {endpoints.length === 0 && <p className="text-sm text-gray-400 text-center py-4">No endpoint data yet. Make some API calls to see metrics.</p>}
                {endpoints.map((ep) => (
                  <div key={ep.name} className="border-b border-gray-100 dark:border-gray-700 pb-3">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-xs font-mono text-gray-700 dark:text-gray-300">{ep.name}</span>
                      <span className="text-xs text-gray-400">{ep.count} calls · {ep.error_rate}% err</span>
                    </div>
                    <ProgressBar value={ep.p50_ms} max={Math.max(ep.p99_ms, 100)} color="bg-blue-500" label="p50" />
                    <ProgressBar value={ep.p95_ms} max={Math.max(ep.p99_ms, 100)} color="bg-amber-500" label="p95" />
                    <ProgressBar value={ep.p99_ms} max={Math.max(ep.p99_ms, 100)} color="bg-red-500" label="p99" />
                  </div>
                ))}
              </div>
            </div>

            {/* Recent Traces */}
            <div className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700">
              <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">🔄 Recent Traces</h3>
              <div className="space-y-1 max-h-64 overflow-y-auto">
                {traces.length === 0 && <p className="text-sm text-gray-400 text-center py-4">No traces recorded yet.</p>}
                {traces.slice(0, 50).map((t) => (
                  <div key={t.id} className="flex items-center gap-3 px-3 py-1.5 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700/50 text-xs">
                    <span className={`w-2 h-2 rounded-full ${t.status === "ok" ? "bg-green-400" : t.status === "error" ? "bg-red-400" : "bg-gray-300"}`} />
                    <span className="font-mono text-gray-600 dark:text-gray-400 w-20">{t.type}</span>
                    <span className="text-gray-800 dark:text-gray-200 flex-1 truncate">{t.name}</span>
                    <span className={`font-mono ${t.slow ? "text-red-500 font-semibold" : "text-gray-400"}`}>
                      {t.duration_ms.toFixed(1)}ms
                    </span>
                  </div>
                ))}
              </div>
            </div>
          </>
        )}

        {activeSection === "tools" && (
          <div className="space-y-4">
            {sections.map((section) => (
              <div key={section.id} className={`bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border-l-4 ${section.color} border border-gray-200 dark:border-gray-700`}>
                <div className="flex items-center justify-between mb-2">
                  <h3 className="font-semibold text-sm text-gray-800 dark:text-gray-100">{section.title}</h3>
                </div>
                <div className="flex flex-wrap gap-2">
                  {section.tools.map((name) => (
                    <button key={name} onClick={() => runTool(name)}
                      className="px-3 py-1.5 bg-gray-100 dark:bg-gray-700 text-xs font-mono rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 transition-colors">
                      {name}
                    </button>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Result */}
        {result && (
          <div className="mt-6 bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700">
            <div className="flex items-center justify-between mb-2">
              <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">📋 Result</h3>
              <button onClick={() => setResult("")} className="text-xs text-gray-400 hover:text-gray-600">Clear</button>
            </div>
            <pre className="text-xs font-mono bg-gray-50 dark:bg-gray-900 p-3 rounded-lg max-h-48 overflow-auto whitespace-pre-wrap">{result}</pre>
          </div>
        )}
      </div>
    </div>
  );
}
