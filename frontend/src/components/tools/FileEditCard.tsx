"use client";

import { cn } from "@/lib/utils";

interface FileEditCardProps {
  diff: string;
  isNew?: boolean;
}

export function FileEditCard({ diff, isNew }: FileEditCardProps) {
  // Parse unified diff and render inline
  const lines = diff.split("\n");

  if (isNew || diff.startsWith("(new file")) {
    return (
      <div className="bg-green-50 dark:bg-green-950 border border-green-200 dark:border-green-800 rounded p-2 text-xs font-mono">
        <p className="text-green-600 dark:text-green-400 font-semibold mb-1">📄 新文件</p>
        <pre className="text-green-700 dark:text-green-300 whitespace-pre-wrap">{diff}</pre>
      </div>
    );
  }

  return (
    <div className="border rounded text-xs font-mono overflow-hidden">
      <div className="px-2 py-1 bg-muted/50 text-muted-foreground text-[10px] font-semibold border-b">
        DIFF 预览
      </div>
      <pre className="p-2 overflow-x-auto leading-5 max-h-60 overflow-y-auto">
        {lines.map((line, i) => {
          const isAdded = line.startsWith("+") && !line.startsWith("+++");
          const isRemoved = line.startsWith("-") && !line.startsWith("---");
          const isHeader = line.startsWith("@@");
          return (
            <span
              key={i}
              className={cn(
                "block whitespace-pre",
                isAdded && "bg-green-100 dark:bg-green-950 text-green-700 dark:text-green-300",
                isRemoved && "bg-red-100 dark:bg-red-950 text-red-700 dark:text-red-300",
                isHeader && "bg-blue-50 dark:bg-blue-950 text-blue-600 dark:text-blue-400 text-[10px]",
              )}
            >
              {line}
            </span>
          );
        })}
      </pre>
    </div>
  );
}
