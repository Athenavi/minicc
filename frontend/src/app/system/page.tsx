"use client";

import { useState, useEffect } from "react";
import { api, apiUrl } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { Toaster, toast } from "sonner";

export default function SystemPage() {
  const [metrics, setMetrics] = useState<any>(null);
  const [traces, setTraces] = useState<any[]>([]);
  const [activeTab, setActiveTab] = useState("health");

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [m, t] = await Promise.all([
          api("/v1/metrics", { skipAuth: true }).catch(() => null),
          api("/metrics/traces", { skipAuth: true }).catch(() => null),
        ]);
        if (m) setMetrics(m.data || m);
        if (t) setTraces(t.traces || []);
      } catch {}
    };
    fetchData();
    const interval = setInterval(fetchData, 10000);
    return () => clearInterval(interval);
  }, []);

  const healthScores = [
    { label: "Architecture", score: 85, color: "bg-green-500" },
    { label: "Performance", score: 72, color: "bg-amber-500" },
    { label: "Security", score: 95, color: "bg-green-500" },
    { label: "Coverage", score: 68, color: "bg-amber-500" },
    { label: "Debt", score: 96, color: "bg-blue-500" },
  ];

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <Toaster />
      <div className="max-w-7xl mx-auto space-y-6">
        <h1 className="text-xl font-bold">🔬 System Console</h1>

        <Tabs defaultValue="health" onValueChange={setActiveTab}>
          <TabsList>
            <TabsTrigger value="health">🏥 Health</TabsTrigger>
            <TabsTrigger value="observe">📊 Observe</TabsTrigger>
          </TabsList>

          <TabsContent value="health" className="space-y-6 mt-4">
            {/* Health scores */}
            <div className="grid grid-cols-5 gap-3">
              {healthScores.map((m) => (
                <Card key={m.label}>
                  <CardContent className="p-4 text-center">
                    <div className="text-2xl font-bold">{m.score}%</div>
                    <Progress value={m.score} className={`mt-2 ${m.color}`} />
                    <p className="text-xs text-gray-500 mt-1">{m.label}</p>
                  </CardContent>
                </Card>
              ))}
            </div>

            {/* Metrics cards */}
            {metrics && (
              <div className="grid grid-cols-5 gap-3">
                {[
                  { label: "Total Spans", value: metrics.total_spans, color: "text-blue-600" },
                  { label: "Active", value: metrics.active_spans, color: "text-green-600" },
                  { label: "Errors", value: metrics.error_spans, color: "text-red-600" },
                  { label: "Slow Traces", value: metrics.slow_spans, color: "text-amber-600" },
                  { label: "Health", value: metrics.error_spans === 0 ? "Good" : "Warning", color: metrics.error_spans === 0 ? "text-green-600" : "text-amber-600" },
                ].map((s) => (
                  <Card key={s.label}>
                    <CardContent className="p-4 text-center">
                      <div className={`text-2xl font-bold ${s.color}`}>{s.value}</div>
                      <p className="text-xs text-gray-500 mt-1">{s.label}</p>
                    </CardContent>
                  </Card>
                ))}
              </div>
            )}
          </TabsContent>

          <TabsContent value="observe" className="space-y-4 mt-4">
            {/* Traces */}
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
                          <Badge variant="outline" className="text-[10px] font-mono w-16">{t.type}</Badge>
                          <span className="flex-1 truncate">{t.name}</span>
                          <span className="font-mono text-gray-400">{t.duration_ms?.toFixed(1)}ms</span>
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
