"use client";

import { useState, useEffect } from "react";
import { api } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Toaster } from "sonner";

export default function SystemPage() {
  const [metrics, setMetrics] = useState<any>(null);
  const [healthScores, setHealthScores] = useState<any[]>([]);
  const [traces, setTraces] = useState<any[]>([]);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const m = await api("/v1/metrics", { skipAuth: true });
        if (m?.data) setMetrics(m.data);

        const h = await api("/v1/system/health", { skipAuth: true });
        if (h?.data?.scores) setHealthScores(h.data.scores);

        const t = await api("/v1/system/traces", { skipAuth: true });
        if (t?.data?.traces) setTraces(t.data.traces);
      } catch {}
    };
    fetchData();
    const interval = setInterval(fetchData, 10000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <Toaster />
      <div className="max-w-7xl mx-auto space-y-6">
        <h1 className="text-xl font-bold">🔬 System Console</h1>

        <Tabs defaultValue="health">
          <TabsList>
            <TabsTrigger value="health">🏥 Health</TabsTrigger>
            <TabsTrigger value="observe">📊 Observe</TabsTrigger>
          </TabsList>

          <TabsContent value="health" className="space-y-6 mt-4">
            {/* Health scores from backend */}
            {healthScores.length > 0 && (
              <div className="grid grid-cols-5 gap-3">
                {healthScores.map((s: any) => (
                  <Card key={s.label}>
                    <CardContent className="p-4 text-center">
                      <div className="text-2xl font-bold">{s.score}%</div>
                      <Progress value={s.score} className={`mt-2 ${s.color || "bg-blue-500"}`} />
                      <p className="text-xs text-gray-500 mt-1">{s.label}</p>
                    </CardContent>
                  </Card>
                ))}
              </div>
            )}

            {/* Real-time metrics from backend */}
            {metrics && (
              <div className="grid grid-cols-5 gap-3">
                {[
                  { label: "Total Requests", value: metrics.requests_total, color: "text-blue-600" },
                  { label: "Active Now", value: metrics.requests_active, color: "text-green-600" },
                  { label: "LLM Calls", value: metrics.llm_calls, color: "text-purple-600" },
                  { label: "Tool Execs", value: metrics.tool_calls, color: "text-amber-600" },
                  { label: "Uptime", value: metrics.uptime_seconds ? `${Math.floor(metrics.uptime_seconds / 60)}m` : "—", color: "text-gray-600" },
                ].map((s) => (
                  <Card key={s.label}>
                    <CardContent className="p-4 text-center">
                      <div className={`text-2xl font-bold ${s.color}`}>{s.value ?? "—"}</div>
                      <p className="text-xs text-gray-500 mt-1">{s.label}</p>
                    </CardContent>
                  </Card>
                ))}
              </div>
            )}
          </TabsContent>

          <TabsContent value="observe" className="space-y-4 mt-4">
            {/* Traces from real tool_calls table */}
            <Card>
              <CardHeader><CardTitle className="text-sm">🔄 Recent Traces</CardTitle></CardHeader>
              <CardContent>
                <ScrollArea className="h-64">
                  {traces.length === 0 ? (
                    <p className="text-sm text-gray-400 text-center py-8">No traces recorded</p>
                  ) : (
                    <div className="space-y-1">
                      {traces.slice(0, 50).map((t: any) => (
                        <div key={t.id} className="flex items-center gap-3 px-3 py-1.5 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800 text-xs">
                          <div className={`w-2 h-2 rounded-full ${t.status === "ok" ? "bg-green-400" : t.status === "error" ? "bg-red-400" : "bg-gray-300"}`} />
                          <Badge variant="outline" className="text-[10px] font-mono w-20 truncate">{t.type}</Badge>
                          <span className="flex-1 truncate">{t.name}</span>
                          <span className="font-mono text-gray-400">{t.duration_ms ? `${t.duration_ms.toFixed(1)}ms` : "—"}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </ScrollArea>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
