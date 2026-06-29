"use client";

import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { Plus, History, Trash2 } from "lucide-react";
import { cn } from "@/lib/utils";

interface SessionItem {
  id: string;
  title: string;
  updatedAt: string;
}

interface SessionListProps {
  sessions: SessionItem[];
  activeId?: string;
  onSelect: (id: string) => void;
  onNew: () => void;
  onDelete: (id: string) => void;
  className?: string;
}

export function SessionList({ sessions, activeId, onSelect, onNew, onDelete, className }: SessionListProps) {
  return (
    <div className={cn("flex flex-col h-full bg-muted/30 border-r", className)}>
      {/* Header */}
      <div className="p-3">
        <Button onClick={onNew} className="w-full gap-2" size="sm">
          <Plus className="w-4 h-4" />
          新对话
        </Button>
      </div>

      <Separator />

      {/* Session list */}
      <ScrollArea className="flex-1 p-2">
        <div className="flex flex-col gap-1">
          {sessions.length === 0 && (
            <p className="text-xs text-muted-foreground text-center py-8">
              暂无历史记录
            </p>
          )}
          {sessions.map((s) => (
            <button
              key={s.id}
              onClick={() => onSelect(s.id)}
              className={cn(
                "flex items-center gap-2 w-full text-left px-3 py-2 rounded-md text-sm transition-colors",
                s.id === activeId
                  ? "bg-accent text-accent-foreground"
                  : "hover:bg-muted"
              )}
            >
              <History className="w-3.5 h-3.5 shrink-0 text-muted-foreground" />
              <span className="truncate flex-1">{s.title}</span>
              <button
                onClick={(e) => { e.stopPropagation(); onDelete(s.id); }}
                className="opacity-0 group-hover:opacity-100 hover:text-destructive"
              >
                <Trash2 className="w-3 h-3" />
              </button>
            </button>
          ))}
        </div>
      </ScrollArea>

      {/* Footer */}
      <div className="p-3 border-t">
        <p className="text-[10px] text-muted-foreground text-center">MiniCC v0.1</p>
      </div>
    </div>
  );
}
