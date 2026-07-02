"use client";

import { useState, useEffect } from "react";
import { useToolRunner } from "@/hooks/useToolRunner";
import { apiUrl } from "@/lib/api";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Toaster, toast } from "sonner";

const CATEGORIES = [
  { id: "browser", label: "🌐 Browser", filter: (n: string) => n.startsWith("browser_") || n.startsWith("web_") },
  { id: "desktop", label: "🖥️ Desktop", filter: (n: string) => n.startsWith("desktop_") || n.startsWith("ocr_") },
  { id: "office", label: "📊 Office", filter: (n: string) => n.startsWith("excel_") || n.startsWith("word_") || n.startsWith("email_") },
  { id: "workflow", label: "⚡ Workflow", filter: (n: string) => n.startsWith("workflow_") || n.startsWith("scheduler_") },
];

export default function RPAPage() {
  const [tools, setTools] = useState<string[]>([]);
  const [loading, setLoading] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const { runTool } = useToolRunner();

  useEffect(() => {
    fetch(apiUrl("/api/tools")).then((r) => r.json()).then((d) => setTools((d.tools || []).map((t: any) => t.name))).catch(() => {});
  }, []);

  const handleRun = async (name: string) => {
    setLoading(name);
    toast.success(await runTool(name));
    setLoading(null);
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <Toaster richColors />
      <div className="max-w-6xl mx-auto space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold">🤖 RPA Control Center</h1>
            <p className="text-sm text-gray-500">Enterprise RPA Platform · {tools.length} tools</p>
          </div>
          <Input className="w-48" placeholder="Search tools..." value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>

        <Tabs defaultValue="browser">
          <TabsList className="w-full">
            {CATEGORIES.map((c) => <TabsTrigger key={c.id} value={c.id} className="flex-1">{c.label}</TabsTrigger>)}
          </TabsList>
          {CATEGORIES.map((c) => (
            <TabsContent key={c.id} value={c.id} className="mt-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                {tools.filter(c.filter).filter((n) => !query || n.includes(query.toLowerCase())).map((name) => (
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
