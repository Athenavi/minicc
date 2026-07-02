"use client";

import { useState, useCallback, useEffect, useRef } from "react";
import { MonacoEditor } from "@/components/editor/MonacoEditor";
import { apiUrl } from "@/lib/api";

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
  language?: string;
}

interface CursorPosition {
  line: number;
  col: number;
}

const getFileLanguage = (name: string): string => {
  const ext = name.split(".").pop()?.toLowerCase() || "";
  const map: Record<string, string> = {
    py: "python", ts: "typescript", tsx: "typescript", js: "javascript",
    jsx: "javascript", md: "markdown", json: "json", yaml: "yaml",
    yml: "yaml", css: "css", html: "html", go: "go", rs: "rust",
    java: "java", toml: "ini", sql: "sql", sh: "shell", bash: "shell",
  };
  return map[ext] || "plaintext";
};

const getFileIcon = (name: string): string => {
  const ext = name.split(".").pop();
  const icons: Record<string, string> = {
    py: "🐍", ts: "🔵", tsx: "⚛️", js: "🟨", jsx: "⚛️",
    md: "📝", json: "📋", yaml: "⚙️", css: "🎨", html: "🌐",
    toml: "⚙️", go: "🔷", rs: "🦀", java: "☕", sql: "🗄️",
    sh: "🐚", bash: "🐚", lock: "🔒", gitignore: "🙈",
  };
  return icons[ext || ""] || "📄";
};

export default function EditorPage() {
  const [files, setFiles] = useState<FileNode[]>([]);
  const [tabs, setTabs] = useState<OpenTab[]>([]);
  const [activeTab, setActiveTab] = useState<string | null>(null);
  const [showChat, setShowChat] = useState(true);
  const [showDiff, setShowDiff] = useState(false);
  const [expandedDirs, setExpandedDirs] = useState<Set<string>>(new Set(["/"]));
  const [cursorPos, setCursorPos] = useState<CursorPosition>({ line: 1, col: 1 });
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; path: string } | null>(null);
  const editorRef = useRef<any>(null);

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "s") {
        e.preventDefault();
        saveFile();
      }
      if ((e.ctrlKey || e.metaKey) && e.key === "w") {
        e.preventDefault();
        if (activeTab) closeTab(activeTab);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [tabs, activeTab]);

  // Load file tree
  useEffect(() => {
    fetch(apiUrl("/api/editor/files"))
      .then((r) => r.json())
      .then((d) => setFiles(d.data?.files || d.files || []))
      .catch(() => {});
  }, []);

  const openFile = useCallback(async (path: string, name: string) => {
    const existing = tabs.find((t) => t.path === path);
    if (existing) { setActiveTab(path); return; }
    try {
      const resp = await fetch(apiUrl(`/api/editor/read?path=${encodeURIComponent(path)}`));
      const data = await resp.json();
      const content = data.data?.content || data.content || "";
      setTabs((prev) => [...prev, { path, name, content, dirty: false, language: getFileLanguage(name) }]);
      setActiveTab(path);
    } catch {}
  }, [tabs]);

  const closeTab = useCallback((path: string) => {
    setTabs((prev) => prev.filter((t) => t.path !== path));
    if (activeTab === path) {
      const remaining = tabs.filter((t) => t.path !== path);
      setActiveTab(remaining.length > 0 ? remaining[remaining.length - 1].path : null);
    }
  }, [tabs, activeTab]);

  const handleEditorChange = useCallback((value: string | undefined) => {
    if (!activeTab || value === undefined) return;
    setTabs((prev) => prev.map((t) => (t.path === activeTab ? { ...t, content: value, dirty: true } : t)));
  }, [activeTab]);

  const saveFile = useCallback(async () => {
    const tab = tabs.find((t) => t.path === activeTab);
    if (!tab || !tab.dirty) return;
    try {
      await fetch(apiUrl("/api/editor/write"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: tab.path, content: tab.content }),
      });
      setTabs((prev) => prev.map((t) => (t.path === activeTab ? { ...t, dirty: false } : t)));
    } catch {}
  }, [tabs, activeTab]);

  const toggleDir = useCallback((path: string) => {
    setExpandedDirs((prev) => {
      const next = new Set(prev);
      next.has(path) ? next.delete(path) : next.add(path);
      return next;
    });
  }, []);

  // Context menu
  useEffect(() => {
    const handleClick = () => setContextMenu(null);
    window.addEventListener("click", handleClick);
    return () => window.removeEventListener("click", handleClick);
  }, []);

  const handleContextMenu = (e: React.MouseEvent, path: string) => {
    e.preventDefault();
    setContextMenu({ x: e.clientX, y: e.clientY, path });
  };

  const handleCursorChange = useCallback((pos: CursorPosition) => {
    setCursorPos(pos);
  }, []);

  const renderFileTree = (nodes: FileNode[], depth: number = 0): React.ReactNode => {
    return nodes.map((node) => (
      <div key={node.path}>
        <div
          className={`flex items-center gap-1 px-2 py-1 cursor-pointer text-xs rounded hover:bg-gray-200 dark:hover:bg-gray-700
            ${activeTab === node.path ? "bg-blue-100 dark:bg-blue-900/50 text-blue-700 dark:text-blue-300" : ""}`}
          style={{ paddingLeft: `${depth * 16 + 8}px` }}
          onClick={() => node.type === "dir" ? toggleDir(node.path) : openFile(node.path, node.name)}
          onContextMenu={(e) => handleContextMenu(e, node.path)}
        >
          <span className="text-gray-500 dark:text-gray-400">
            {node.type === "dir" ? (expandedDirs.has(node.path) ? "📂" : "📁") : getFileIcon(node.name)}
          </span>
          <span className="truncate text-gray-700 dark:text-gray-300">{node.name}</span>
        </div>
        {node.type === "dir" && expandedDirs.has(node.path) && node.children && (
          <div>{renderFileTree(node.children, depth + 1)}</div>
        )}
      </div>
    ));
  };

  const activeContent = tabs.find((t) => t.path === activeTab);
  const dirtyCount = tabs.filter((t) => t.dirty).length;

  return (
    <div className="flex h-screen bg-white dark:bg-gray-900">
      {/* File Sidebar */}
      <div className="w-56 bg-gray-50 dark:bg-gray-800 border-r dark:border-gray-700 flex flex-col">
        <div className="p-2 border-b dark:border-gray-700 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="text-xs font-semibold text-gray-600 dark:text-gray-300">Explorer</span>
            {dirtyCount > 0 && <span className="text-[10px] bg-blue-100 dark:bg-blue-900 text-blue-600 dark:text-blue-400 px-1.5 py-0.5 rounded-full">{dirtyCount}</span>}
          </div>
          <div className="flex gap-1">
            <button onClick={() => setShowDiff(!showDiff)} className="text-[10px] px-1.5 py-0.5 rounded bg-gray-200 dark:bg-gray-700 hover:bg-gray-300" title="Toggle diff">
              {showDiff ? "◀" : "▶"}
            </button>
            <button onClick={() => setShowChat(!showChat)} className="text-[10px] px-1.5 py-0.5 rounded bg-gray-200 dark:bg-gray-700 hover:bg-gray-300">
              {showChat ? "✕" : "💬"}
            </button>
          </div>
        </div>
        <div className="flex-1 overflow-auto py-1">
          {files.length > 0 ? renderFileTree(files) : (
            <div className="text-xs text-gray-400 p-4 text-center">
              <p className="mb-2">📁 No files</p>
              <p className="text-[10px]">Connect backend to browse workspace</p>
            </div>
          )}
        </div>
      </div>

      {/* Context Menu */}
      {contextMenu && (
        <div className="fixed bg-white dark:bg-gray-800 shadow-xl border dark:border-gray-700 rounded-lg py-1 z-50 min-w-[140px]"
          style={{ left: contextMenu.x, top: contextMenu.y }}>
          <button className="w-full text-left px-3 py-1.5 text-xs hover:bg-gray-100 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-300"
            onClick={() => { openFile(contextMenu.path, contextMenu.path.split("/").pop() || ""); setContextMenu(null); }}>
            📄 Open
          </button>
          <button className="w-full text-left px-3 py-1.5 text-xs hover:bg-gray-100 dark:hover:bg-gray-700 text-red-600"
            onClick={() => { setContextMenu(null); }}>
            🗑 Delete (TODO)
          </button>
        </div>
      )}

      {/* Editor Area */}
      <div className="flex-1 flex flex-col">
        {/* Tabs Bar */}
        <div className="flex items-center bg-gray-100 dark:bg-gray-800 border-b dark:border-gray-700 overflow-x-auto">
          {tabs.map((tab) => (
            <div key={tab.path} onClick={() => setActiveTab(tab.path)}
              className={`flex items-center gap-1 px-3 py-1.5 text-xs cursor-pointer border-r dark:border-gray-700 whitespace-nowrap group
                ${activeTab === tab.path ? "bg-white dark:bg-gray-900 border-t-2 border-t-blue-500 font-medium" : "hover:bg-gray-200 dark:hover:bg-gray-700"}`}>
              {tab.dirty ? <span className="text-blue-500 text-[10px]">●</span> : <span className="text-gray-300">■</span>}
              <span className="text-gray-700 dark:text-gray-300">{getFileIcon(tab.name)} {tab.name}</span>
              <button onClick={(e) => { e.stopPropagation(); closeTab(tab.path); }}
                className="ml-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 opacity-0 group-hover:opacity-100 transition-opacity">✕</button>
            </div>
          ))}
          {tabs.length === 0 && (
            <div className="text-xs text-gray-400 px-3 py-1.5">Open a file from the explorer</div>
          )}
        </div>

        {/* Editor */}
        <div className="flex-1 relative">
          {activeContent ? (
            <MonacoEditor
              key={activeContent.path}
              value={activeContent.content}
              path={activeContent.path}
              language={activeContent.language}
              onChange={handleEditorChange}
            />
          ) : (
            <div className="flex items-center justify-center h-full text-sm text-gray-400 dark:text-gray-500">
              <div className="text-center">
                <div className="text-5xl mb-4">📝</div>
                <p className="text-base">AI-Native Editor</p>
                <p className="text-xs mt-2 text-gray-400">Open a file to start editing · Ctrl+S to save · Ctrl+W to close</p>
              </div>
            </div>
          )}
        </div>

        {/* Status Bar */}
        <div className="h-6 bg-blue-600 text-white text-[11px] flex items-center px-3 gap-3">
          <span className="font-medium">{activeContent?.name || "No file"}</span>
          {activeContent && (
            <>
              <span className="opacity-70">{activeContent.dirty ? "● unsaved" : "saved"}</span>
              <span className="opacity-70">Ln {cursorPos.line}, Col {cursorPos.col}</span>
              <span className="opacity-70">{activeContent.language || "plaintext"}</span>
              <div className="ml-auto flex gap-3">
                <button onClick={saveFile} className="hover:underline opacity-80 hover:opacity-100" disabled={!activeContent?.dirty}>
                  💾 Save
                </button>
                <button onClick={() => setShowDiff(!showDiff)} className="hover:underline opacity-80 hover:opacity-100">
                  {showDiff ? "Hide Diff" : "Show Diff"}
                </button>
              </div>
            </>
          )}
        </div>
      </div>

      {/* AI Chat Panel */}
      {showChat && (
        <div className="w-80 border-l dark:border-gray-700 bg-white dark:bg-gray-900 flex flex-col">
          <div className="p-3 border-b dark:border-gray-700 flex items-center justify-between">
            <span className="text-xs font-semibold text-gray-600 dark:text-gray-300">💬 AI Assistant</span>
            <button onClick={() => setShowChat(false)} className="text-xs text-gray-400 hover:text-gray-600">✕</button>
          </div>
          <div className="flex-1 overflow-auto p-3 text-xs text-gray-500 dark:text-gray-400 space-y-3">
            <div className="bg-blue-50 dark:bg-blue-900/20 p-2 rounded-lg">
              <p className="font-medium text-blue-600 dark:text-blue-400 text-[10px] mb-1">AI ✦</p>
              <p>I can help you edit this file. What would you like to change?</p>
            </div>
            <div className="bg-gray-50 dark:bg-gray-800 p-2 rounded-lg">
              <p className="font-medium text-gray-600 text-[10px] mb-1">You</p>
              <p>Coming in Phase N — natural language editing, multi-cursor AI, inline completions.</p>
            </div>
          </div>
          <div className="p-3 border-t dark:border-gray-700">
            <div className="flex gap-2">
              <input className="flex-1 px-2 py-1.5 text-xs border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100"
                placeholder="Ask AI to edit..." disabled />
              <button className="px-3 py-1.5 bg-blue-600 text-white text-xs rounded disabled:opacity-50" disabled>Send</button>
            </div>
          </div>
        </div>
      )}

      {/* Diff Panel */}
      {showDiff && activeContent && (
        <div className="w-96 border-l dark:border-gray-700 bg-white dark:bg-gray-900 flex flex-col">
          <div className="p-3 border-b dark:border-gray-700 flex items-center justify-between">
            <span className="text-xs font-semibold text-gray-600 dark:text-gray-300">📊 AI Changes</span>
            <button onClick={() => setShowDiff(false)} className="text-xs text-gray-400 hover:text-gray-600">✕</button>
          </div>
          <div className="flex-1 p-3 text-xs text-gray-400">
            <p>Diff view will show AI-proposed changes (Phase M5).</p>
            <p className="mt-2 text-[10px]">Features: Accept / Reject per change, inline diff highlights.</p>
          </div>
        </div>
      )}
    </div>
  );
}
