"use client";

import { useState, useCallback, useEffect } from "react";
import { MonacoEditor } from "@/components/editor/MonacoEditor";

interface FileNode {
  name: string;
  path: string;
  type: "file" | "dir";
  children?: FileNode[];
}

interface OpenTab {
  path: string;
  name: string;
  content: string;
  dirty: boolean;
}

export default function EditorPage() {
  const [files, setFiles] = useState<FileNode[]>([]);
  const [tabs, setTabs] = useState<OpenTab[]>([]);
  const [activeTab, setActiveTab] = useState<string | null>(null);
  const [showChat, setShowChat] = useState(true);
  const [expandedDirs, setExpandedDirs] = useState<Set<string>>(new Set(["/"]));

  // Load file tree on mount
  useEffect(() => {
    fetch("/api/editor/files")
      .then((r) => r.json())
      .then((data) => setFiles(data.files || []))
      .catch(() => {});
  }, []);

  const openFile = useCallback(async (path: string, name: string) => {
    // Check if already open
    const existing = tabs.find((t) => t.path === path);
    if (existing) {
      setActiveTab(path);
      return;
    }

    try {
      const resp = await fetch(`/api/editor/read?path=${encodeURIComponent(path)}`);
      const data = await resp.json();
      setTabs((prev) => [...prev, { path, name, content: data.content || "", dirty: false }]);
      setActiveTab(path);
    } catch {}
  }, [tabs]);

  const closeTab = useCallback((path: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setTabs((prev) => prev.filter((t) => t.path !== path));
    if (activeTab === path) {
      const remaining = tabs.filter((t) => t.path !== path);
      setActiveTab(remaining.length > 0 ? remaining[remaining.length - 1].path : null);
    }
  }, [tabs, activeTab]);

  const handleEditorChange = useCallback((value: string | undefined) => {
    if (!activeTab) return;
    setTabs((prev) =>
      prev.map((t) => (t.path === activeTab ? { ...t, content: value || "", dirty: true } : t))
    );
  }, [activeTab]);

  const saveFile = useCallback(async () => {
    const tab = tabs.find((t) => t.path === activeTab);
    if (!tab || !tab.dirty) return;
    await fetch("/api/editor/write", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path: tab.path, content: tab.content }),
    });
    setTabs((prev) => prev.map((t) => (t.path === activeTab ? { ...t, dirty: false } : t)));
  }, [tabs, activeTab]);

  const toggleDir = useCallback((path: string) => {
    setExpandedDirs((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }, []);

  const renderFileTree = (nodes: FileNode[], depth: number = 0): React.ReactNode => {
    return nodes.map((node) => (
      <div key={node.path}>
        <div
          className={`flex items-center gap-1 px-2 py-1 cursor-pointer text-xs rounded hover:bg-gray-200 dark:hover:bg-gray-700
            ${activeTab === node.path ? "bg-blue-100 dark:bg-blue-900" : ""}
          `}
          style={{ paddingLeft: `${depth * 16 + 8}px` }}
          onClick={() => node.type === "dir" ? toggleDir(node.path) : openFile(node.path, node.name)}
        >
          <span className="text-gray-500 dark:text-gray-400">
            {node.type === "dir" ? (expandedDirs.has(node.path) ? "📂" : "📁") : getFileIcon(node.name)}
          </span>
          <span className="truncate">{node.name}</span>
        </div>
        {node.type === "dir" && expandedDirs.has(node.path) && node.children && (
          <div>{renderFileTree(node.children, depth + 1)}</div>
        )}
      </div>
    ));
  };

  const getFileIcon = (name: string): string => {
    const ext = name.split(".").pop();
    const icons: Record<string, string> = {
      py: "🐍", ts: "🔵", tsx: "⚛️", js: "🟨", jsx: "⚛️",
      md: "📝", json: "📋", yaml: "⚙️", css: "🎨", html: "🌐",
      toml: "⚙️", go: "🔷", rs: "🦀", java: "☕",
    };
    return icons[ext || ""] || "📄";
  };

  const activeContent = tabs.find((t) => t.path === activeTab);

  return (
    <div className="flex h-screen bg-white dark:bg-gray-900">
      {/* File Sidebar */}
      <div className="w-56 bg-gray-50 dark:bg-gray-800 border-r dark:border-gray-700 flex flex-col">
        <div className="p-2 border-b dark:border-gray-700 flex items-center justify-between">
          <span className="text-xs font-semibold text-gray-600 dark:text-gray-300">Explorer</span>
          <button
            onClick={() => setShowChat(!showChat)}
            className="text-xs px-2 py-1 rounded bg-gray-200 dark:bg-gray-700 hover:bg-gray-300 dark:hover:bg-gray-600"
          >
            {showChat ? "Hide Chat" : "Chat"}
          </button>
        </div>
        <div className="flex-1 overflow-auto py-1">
          {files.length > 0 ? renderFileTree(files) : (
            <div className="text-xs text-gray-400 p-4 text-center">
              Connect backend to browse files
            </div>
          )}
        </div>
      </div>

      {/* Editor Area */}
      <div className="flex-1 flex flex-col">
        {/* Tabs Bar */}
        <div className="flex items-center bg-gray-100 dark:bg-gray-800 border-b dark:border-gray-700 overflow-x-auto">
          {tabs.map((tab) => (
            <div
              key={tab.path}
              onClick={() => setActiveTab(tab.path)}
              className={`flex items-center gap-1 px-3 py-1.5 text-xs cursor-pointer border-r dark:border-gray-700 whitespace-nowrap
                ${activeTab === tab.path
                  ? "bg-white dark:bg-gray-900 border-t-2 border-t-blue-500"
                  : "hover:bg-gray-200 dark:hover:bg-gray-700"}
              `}
            >
              {tab.dirty && <span className="text-blue-500">●</span>}
              {tab.name}
              <button
                onClick={(e) => closeTab(tab.path, e)}
                className="ml-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
              >
                ✕
              </button>
            </div>
          ))}
          {tabs.length === 0 && (
            <div className="text-xs text-gray-400 px-3 py-1.5">
              Open a file to start editing
            </div>
          )}
        </div>

        {/* Editor */}
        <div className="flex-1">
          {activeContent ? (
            <MonacoEditor
              key={activeContent.path}
              value={activeContent.content}
              path={activeContent.path}
              onChange={handleEditorChange}
            />
          ) : (
            <div className="flex items-center justify-center h-full text-sm text-gray-400 dark:text-gray-500">
              <div className="text-center">
                <div className="text-4xl mb-4">📝</div>
                <p>Open a file from the explorer</p>
                <p className="text-xs mt-2">Press Ctrl+S to save</p>
              </div>
            </div>
          )}
        </div>

        {/* Status Bar */}
        {activeContent && (
          <div className="h-6 bg-blue-600 text-white text-xs flex items-center px-3 gap-4">
            <span>{activeContent.name}</span>
            <span className="opacity-70">{activeContent.dirty ? "● unsaved" : "saved"}</span>
            <button onClick={saveFile} className="ml-auto hover:underline opacity-80 hover:opacity-100">
              Save (Ctrl+S)
            </button>
          </div>
        )}
      </div>

      {/* Chat Panel */}
      {showChat && (
        <div className="w-80 border-l dark:border-gray-700 bg-white dark:bg-gray-900 flex flex-col">
          <div className="p-3 border-b dark:border-gray-700 text-xs font-semibold text-gray-600 dark:text-gray-300">
            AI Chat
          </div>
          <div className="flex-1 overflow-auto p-3 text-xs text-gray-500 dark:text-gray-400">
            <p>Chat panel integration coming in Phase N.</p>
            <p className="mt-2">When AI edits your code, changes will appear in the Diff editor (M5).</p>
          </div>
        </div>
      )}
    </div>
  );
}
