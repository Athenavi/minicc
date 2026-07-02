"use client";

import { useState } from "react";
import { useToolRunner } from "@/hooks/useToolRunner";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Badge } from "@/components/ui/badge";
import { Toaster, toast } from "sonner";

const TABS = [
  { id: "pm", label: "📋 Planning", tools: ["prd_generate","tech_design","task_decompose","requirement_validate"] },
  { id: "dev", label: "👷 Development", tools: ["architect_agent","coding_agent","test_agent","review_agent"] },
  { id: "cicd", label: "🚀 CI/CD", tools: ["ci_generate","deploy_service","monitor_setup"] },
  { id: "ops", label: "🔧 Operations", tools: ["error_analyze","self_heal","goal_set","goal_status"] },
];

const PIPELINE = [
  { name: "Lint", status: "success" as const },
  { name: "Unit Tests", status: "success" as const },
  { name: "Integration", status: "running" as const },
  { name: "Build", status: "pending" as const },
  { name: "Deploy", status: "pending" as const },
];

const statusColor = (s: string) =>
  s === "success" ? "bg-green-500" : s === "running" ? "bg-blue-500" : "bg-gray-300 dark:bg-gray-600";

export default function DevOpsPage() {
  const [loading, setLoading] = useState<string | null>(null);
  const { runTool } = useToolRunner();

  const handleRun = async (name: string) => {
    setLoading(name);
    const out = await runTool(name);
    toast.success(out.slice(0, 100));
    setLoading(null);
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <Toaster richColors />
      <div className="max-w-6xl mx-auto space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold">🚀 DevOps Platform</h1>
            <p className="text-sm text-gray-500">AI-powered Development Lifecycle</p>
          </div>
          <Badge variant="outline" className="text-green-600 bg-green-50 dark:bg-green-950">● All Operational</Badge>
        </div>

        {/* Pipeline */}
        <div className="flex gap-2 overflow-x-auto pb-2">
          {PIPELINE.map((s) => (
            <div key={s.name} className="flex flex-col items-center min-w-[90px] p-3 bg-white dark:bg-gray-800 rounded-lg border shadow-sm">
              <div className={`w-3 h-3 rounded-full mb-1.5 ${statusColor(s.status)}`} />
              <span className="text-xs font-medium text-gray-700 dark:text-gray-300">{s.name}</span>
              <Badge variant="outline" className="mt-1 text-[10px] px-1">{s.status}</Badge>
            </div>
          ))}
        </div>

        <Tabs defaultValue="pm">
          <TabsList className="w-full">
            {TABS.map((t) => <TabsTrigger key={t.id} value={t.id} className="flex-1">{t.label}</TabsTrigger>)}
          </TabsList>
          {TABS.map((t) => (
            <TabsContent key={t.id} value={t.id} className="mt-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                {t.tools.map((name) => (
                  <Card key={name} className="hover:shadow-md transition-shadow">
                    <CardContent className="p-4 flex items-center justify-between">
                      <span className="text-sm font-mono">{name}</span>
                      <Button size="sm" onClick={() => handleRun(name)} disabled={loading === name}>
                        {loading === name ? "..." : "▶ Run"}
                      </Button>
                    </CardContent>
                  </Card>
                ))}
              </div>
            </TabsContent>
          ))}
        </Tabs>
      </div>
    </div>
  );
}
