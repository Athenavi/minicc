"use client";

import { useState, useEffect, useCallback } from "react";
import { useToolRunner } from "@/hooks/useToolRunner";
import { api } from "@/lib/api";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Badge } from "@/components/ui/badge";
import { Toaster, toast } from "sonner";

interface ListItem {
  id: string;
  title?: string;
  name?: string;
  subject?: string;
  objective?: string;
  status?: string;
  priority?: string;
  [key: string]: any;
}

const MODULES = [
  { id: "collab", label: "👥 Collaboration", listEndpoint: "/v1/enterprise/tasks", tools: ["collab_task_create","collab_wiki_create","collab_wiki_search","collab_okr_create","collab_message_send","collab_meeting_summary"] },
  { id: "brain", label: "🧠 Enterprise Brain", listEndpoint: "/v1/enterprise/brain", tools: ["brain_query","brain_decision","brain_predict","brain_compliance"] },
  { id: "support", label: "🎫 Customer Service", listEndpoint: "/v1/enterprise/tickets", tools: ["support_ticket_create","support_kb_search","support_chatbot_reply","marketing_campaign_create","marketing_abtest"] },
];

const priorityColors: Record<string, string> = {
  low: "bg-gray-100 text-gray-600", medium: "bg-blue-100 text-blue-700",
  high: "bg-amber-100 text-amber-700", urgent: "bg-red-100 text-red-700",
};
const statusColors: Record<string, string> = {
  open: "bg-yellow-100 text-yellow-800", in_progress: "bg-blue-100 text-blue-800",
  done: "bg-green-100 text-green-800", resolved: "bg-green-100 text-green-800",
  closed: "bg-gray-100 text-gray-500", cancelled: "bg-gray-100 text-gray-500",
  draft: "bg-gray-100 text-gray-600", running: "bg-blue-100 text-blue-800",
  completed: "bg-green-100 text-green-800", active: "bg-green-100 text-green-800",
};

export default function EnterprisePage() {
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState<string | null>(null);
  const [lists, setLists] = useState<Record<string, ListItem[]>>({});
  const { runTool } = useToolRunner();

  const fetchList = useCallback(async (moduleId: string, endpoint: string) => {
    try {
      const resp = await api(endpoint, { skipAuth: true });
      const items = resp?.data || [];
      setLists((prev) => ({ ...prev, [moduleId]: items }));
    } catch { /* ignore */ }
  }, []);

  useEffect(() => {
    MODULES.forEach((m) => fetchList(m.id, m.listEndpoint));
  }, [fetchList]);

  const handleRun = async (name: string) => {
    setLoading(name);
    const out = await runTool(name);
    toast.success(out ? out.slice(0, 120) : "Done");
    setLoading(null);
    // Refresh lists
    const mod = MODULES.find((m) => m.tools.includes(name));
    if (mod) fetchList(mod.id, mod.listEndpoint);
  };

  const renderItemRow = (item: ListItem) => {
    const title = item.title || item.name || item.subject || item.objective || "—";
    const status = item.status || "";
    const priority = item.priority || "";
    return (
      <div key={item.id} className="flex items-center gap-2 px-3 py-1.5 text-xs border-b last:border-b-0 hover:bg-gray-50 dark:hover:bg-gray-800">
        <span className="flex-1 truncate">{title}</span>
        {priority && <Badge variant="outline" className={`text-[10px] ${priorityColors[priority] || ""}`}>{priority}</Badge>}
        {status && <Badge variant="outline" className={`text-[10px] ${statusColors[status] || ""}`}>{status.replace("_", " ")}</Badge>}
      </div>
    );
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 p-4">
      <Toaster richColors />
      <div className="max-w-7xl mx-auto space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold">🏢 Enterprise OS</h1>
            <p className="text-sm text-gray-500">Collaboration · Brain · Customer Service</p>
          </div>
          <Input className="w-48" placeholder="Search tools..." value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>

        <Tabs defaultValue="collab">
          <TabsList className="w-full">
            {MODULES.map((m) => (
              <TabsTrigger key={m.id} value={m.id} className="flex-1">
                {m.label} {lists[m.id]?.length > 0 ? `(${lists[m.id].length})` : ""}
              </TabsTrigger>
            ))}
          </TabsList>

          {MODULES.map((mod) => (
            <TabsContent key={mod.id} value={mod.id} className="mt-4 space-y-4">
              {/* Data list */}
              {lists[mod.id] && lists[mod.id].length > 0 && (
                <Card>
                  <CardContent className="p-3">
                    <ScrollArea className="max-h-48">
                      {lists[mod.id].map(renderItemRow)}
                    </ScrollArea>
                  </CardContent>
                </Card>
              )}

              {/* Tool buttons */}
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                {mod.tools.filter((n) => !query || n.includes(query.toLowerCase())).map((name) => (
                  <Card key={name} className="hover:shadow-md transition-shadow">
                    <CardContent className="p-4 flex items-center justify-between">
                      <span className="text-sm font-mono truncate">{name}</span>
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
