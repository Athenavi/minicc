"use client";

import { useState } from "react";
import { useToolRunner } from "@/hooks/useToolRunner";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Toaster, toast } from "sonner";

const MODULES = [
  { id: "collab", label: "👥 Collaboration", tools: ["collab_task_create","collab_wiki_create","collab_wiki_search","collab_okr_create","collab_message_send","collab_meeting_summary"] },
  { id: "brain", label: "🧠 Enterprise Brain", tools: ["brain_query","brain_decision","brain_predict","brain_compliance"] },
  { id: "support", label: "🎫 Customer Service", tools: ["support_ticket_create","support_kb_search","support_chatbot_reply","marketing_campaign_create","marketing_abtest"] },
];

export default function EnterprisePage() {
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState<string | null>(null);
  const { runTool } = useToolRunner();

  const filtered = MODULES.flatMap((m) => m.tools).filter((n) => !query || n.includes(query.toLowerCase()));
  const [activeModule, setActiveModule] = useState("collab");

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
            <h1 className="text-xl font-bold">🏢 Enterprise OS</h1>
            <p className="text-sm text-gray-500">Collaboration · Brain · Customer Service</p>
          </div>
          <Input className="w-48" placeholder="Search tools..." value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>

        <Tabs defaultValue="collab">
          <TabsList className="w-full">
            {MODULES.map((m) => <TabsTrigger key={m.id} value={m.id} className="flex-1">{m.label}</TabsTrigger>)}
          </TabsList>
          {MODULES.map((m) => (
            <TabsContent key={m.id} value={m.id} className="mt-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                {m.tools.filter((n) => !query || n.includes(query.toLowerCase())).map((name) => (
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
