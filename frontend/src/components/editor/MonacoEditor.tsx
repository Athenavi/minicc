"use client";

import { useRef, useCallback } from "react";
import Editor, { OnMount } from "@monaco-editor/react";
import type { editor } from "monaco-editor";

interface MonacoEditorProps {
  value: string;
  language?: string;
  path?: string;
  readOnly?: boolean;
  onChange?: (value: string | undefined) => void;
  onMount?: (editor: editor.IStandaloneCodeEditor) => void;
  height?: string;
}

const LANGUAGE_MAP: Record<string, string> = {
  py: "python",
  js: "javascript",
  jsx: "javascript",
  ts: "typescript",
  tsx: "typescript",
  md: "markdown",
  json: "json",
  yaml: "yaml",
  yml: "yaml",
  toml: "plaintext",
  css: "css",
  html: "html",
  xml: "xml",
  sh: "shell",
  bash: "shell",
  go: "go",
  rs: "rust",
  java: "java",
  cpp: "cpp",
  c: "c",
  h: "c",
};

function detectLanguage(path?: string): string {
  if (!path) return "plaintext";
  const ext = path.split(".").pop()?.toLowerCase() || "";
  return LANGUAGE_MAP[ext] || "plaintext";
}

export function MonacoEditor({
  value,
  language,
  path,
  readOnly = false,
  onChange,
  onMount: onMountProp,
  height = "100%",
}: MonacoEditorProps) {
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);

  const handleMount: OnMount = useCallback(
    (editor, monaco) => {
      editorRef.current = editor;

      // Ctrl+S save
      editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
        const val = editor.getValue();
        onChange?.(val);
      });

      onMountProp?.(editor);

      // Auto-resize on content change
      editor.onDidChangeModelContent(() => {
        const lineCount = editor.getModel()?.getLineCount() || 1;
        const lineHeight = 20;
        const newHeight = Math.max(200, Math.min(800, lineCount * lineHeight + 40));
        editor.getContainerDomNode().style.height = `${newHeight}px`;
        editor.layout();
      });
    },
    [onChange, onMountProp]
  );

  const resolvedLanguage = language || detectLanguage(path);

  return (
    <div className="border rounded-md overflow-hidden dark:border-gray-700">
      <Editor
        height={height}
        language={resolvedLanguage}
        value={value}
        theme="vs-dark"
        onChange={onChange}
        onMount={handleMount}
        options={{
          readOnly,
          minimap: { enabled: false },
          fontSize: 13,
          lineNumbers: "on",
          scrollBeyondLastLine: false,
          automaticLayout: true,
          tabSize: 4,
          wordWrap: "on",
          padding: { top: 8 },
          renderWhitespace: "selection",
          bracketPairColorization: { enabled: true },
          suggestOnTriggerCharacters: true,
          quickSuggestions: true,
        }}
      />
    </div>
  );
}
