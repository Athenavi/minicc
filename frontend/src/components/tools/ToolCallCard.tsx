"use client";

import React from "react";
import { cn } from "@/lib/utils";
import { FileText, FileEdit, Terminal, Search, Globe, type LucideIcon } from "lucide-react";
import { type ToolCall, type ToolCallStatus, type PermissionLevel } from "@/lib/types";
import { FileEditCard } from "./FileEditCard";
import { ShellCard } from "./ShellCard";

const toolIcons: Record<string, LucideIcon> = {
  read_file: FileText,
  file_read: FileText,
  write_to_file: FileEdit,
  str_replace_editor: FileEdit,
  file_write: FileEdit,
  file_edit: FileEdit,
  bash: Terminal,
  shell: Terminal,
  grep: Search,
  glob: Search,
  web_fetch: Globe,
  web_search: Globe,
};

const levelColors: Record<string, string> = {
  read: "border-l-blue-500",
  write: "border-l-amber-500",
  execute: "border-l-red-500",
};

const statusLabels: Record<string, string> = {
  pending: "等待审批",
  approved: "已批准",
  rejected: "已拒绝",
  running: "执行中...",
  completed: "已完成",
  failed: "失败",
};

const statusColors: Record<string, string> = {
  pending: "text-yellow-600 bg-yellow-50 dark:text-yellow-400 dark:bg-yellow-950",
  approved: "text-green-600 bg-green-50 dark:text-green-400 dark:bg-green-950",
  rejected: "text-red-600 bg-red-50 dark:text-red-400 dark:bg-red-950",
  running: "text-blue-600 bg-blue-50 dark:text-blue-400 dark:bg-blue-950",
  completed: "text-green-600 bg-green-50 dark:text-green-400 dark:bg-green-950",
  failed: "text-red-600 bg-red-50 dark:text-red-400 dark:bg-red-950",
};

interface ToolCallCardProps {
  toolCall: ToolCall;
  status: ToolCallStatus;
  level?: PermissionLevel;
  diffPreview?: string;
  result?: string;
  isError?: boolean;
  requestId?: string;
  onApprove?: () => void;
  onReject?: () => void;
  onAlwaysAllow?: () => void;
}

export function ToolCallCard({
  toolCall,
  status,
  level = "read",
  diffPreview,
  result,
  isError,
  requestId,
  onApprove,
  onReject,
  onAlwaysAllow,
}: ToolCallCardProps) {
  const Icon = toolIcons[toolCall.name] ?? FileText;
  const borderColor = levelColors[level] ?? "border-l-gray-400";

  return (
    <div className={cn(
      "border rounded-lg border-l-4 overflow-hidden",
      borderColor,
      "bg-card text-card-foreground shadow-sm"
    )}>
      {/* Header */}
      <div className="flex items-center gap-2 px-3 py-2 border-b bg-muted/30">
        <Icon className="w-4 h-4 shrink-0 text-muted-foreground" />
        <span className="text-sm font-mono font-medium">{toolCall.name}</span>
        <span className={cn(
          "text-xs px-2 py-0.5 rounded-full font-medium ml-auto",
          statusColors[status] ?? "bg-muted text-muted-foreground"
        )}>
          {statusLabels[status] ?? status}
        </span>
      </div>

      {/* Body */}
      <div className="p-3 space-y-3">
        {/* File path */}
        {toolCall.input?.path ? (
          <p className="text-xs text-muted-foreground font-mono truncate">
            📄 {String(toolCall.input.path)}
          </p>
        ) : null}

        {/* Command preview */}
        {toolCall.name === "bash" && toolCall.input?.command ? (
          <ShellCard command={String(toolCall.input.command)} />
        ) : null}

        {/* Diff preview */}
        {diffPreview ? (
          <FileEditCard
            diff={diffPreview}
            isNew={status === "pending" && !result}
          />
        ) : null}

        {/* Result output */}
        {result ? (
          <pre className={cn(
            "text-xs font-mono p-2 rounded max-h-40 overflow-auto",
            isError ? "bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-300" : "bg-muted"
          )}>
            {result.slice(0, 2000)}
            {result.length > 2000 ? "\n...(truncated)" : null}
          </pre>
        ) : null}

        {status === "pending" && level !== "read" ? (
          <ApprovalButtons
            level={level}
            onApprove={onApprove}
            onReject={onReject}
            onAlwaysAllow={onAlwaysAllow}
          />
        ) : null}
      </div>
    </div>
  );
}

// -- Approval Buttons --

interface ApprovalButtonsProps {
  level: PermissionLevel;
  onApprove?: () => void;
  onReject?: () => void;
  onAlwaysAllow?: () => void;
}

function ApprovalButtons({ level, onApprove, onReject, onAlwaysAllow }: ApprovalButtonsProps) {
  const [understood, setUnderstood] = React.useState(level !== "execute");

  return (
    <div className="space-y-2">
      {/* Danger warning for EXECUTE */}
      {level === "execute" && (
        <div className="flex items-start gap-2 p-2 bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-800 rounded text-xs text-red-700 dark:text-red-300">
          <span>⚠️</span>
          <div className="space-y-1">
            <p className="font-semibold">此操作将执行 Shell 命令</p>
            <label className="flex items-center gap-1.5 cursor-pointer">
              <input
                type="checkbox"
                checked={understood}
                onChange={(e) => setUnderstood(e.target.checked)}
                className="rounded"
              />
              <span>我了解风险</span>
            </label>
          </div>
        </div>
      )}

      <div className="flex gap-2">
        <button
          onClick={onApprove}
          disabled={!understood}
          className="flex-1 px-3 py-1.5 text-xs font-medium rounded bg-green-600 text-white hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          ✅ 批准
        </button>
        <button
          onClick={onReject}
          className="px-3 py-1.5 text-xs font-medium rounded bg-red-600 text-white hover:bg-red-700 transition-colors"
        >
          ❌ 拒绝
        </button>
        <button
          onClick={onAlwaysAllow}
          disabled={!understood}
          className="px-3 py-1.5 text-xs font-medium rounded bg-muted text-muted-foreground hover:bg-muted/80 border disabled:opacity-50 transition-colors"
        >
          ⭐ 始终允许
        </button>
      </div>
    </div>
  );
}
