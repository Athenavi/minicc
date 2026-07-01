"use client";

import { useMemo } from "react";
import { DiffEditor } from "@monaco-editor/react";

interface DiffViewProps {
  original: string;
  modified: string;
  language?: string;
  originalLabel?: string;
  modifiedLabel?: string;
}

export function AIDiffViewer({
  original,
  modified,
  language = "plaintext",
  originalLabel = "Original",
  modifiedLabel = "AI Changes",
}: DiffViewProps) {
  return (
    <div className="border rounded-md overflow-hidden dark:border-gray-700">
      <div className="flex items-center justify-between bg-gray-100 dark:bg-gray-800 px-3 py-1.5 border-b dark:border-gray-700">
        <div className="flex gap-4 text-xs">
          <span className="flex items-center gap-1">
            <span className="w-2 h-2 rounded-full bg-red-500" />
            {originalLabel}
          </span>
          <span className="flex items-center gap-1">
            <span className="w-2 h-2 rounded-full bg-green-500" />
            {modifiedLabel}
          </span>
        </div>
      </div>
      <DiffEditor
        height="400px"
        language={language}
        original={original}
        modified={modified}
        theme="vs-dark"
        options={{
          readOnly: true,
          minimap: { enabled: false },
          fontSize: 13,
          scrollBeyondLastLine: false,
          wordWrap: "on",
          lineNumbers: "on",
        }}
      />
    </div>
  );
}
