"use client";

import { useCallback, useState, useEffect } from "react";
import { api, apiUrl } from "@/lib/api";
import {
  ReactFlow,
  MiniMap,
  Controls,
  Background,
  useNodesState,
  useEdgesState,
  addEdge,
  type Connection,
  type Node,
  type Edge,
  type NodeTypes,
  type NodeChange,
  applyNodeChanges,
  applyEdgeChanges,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

// ─── Types ─────────────────────────────────────────────────────────────────

type NodeType = "input" | "output" | "llm" | "tool" | "condition" | "code" | "wait";

interface WorkflowMeta {
  name: string;
  description: string;
  version: string;
  tags: string[];
}

interface LogEntry {
  time: string;
  message: string;
  type: "info" | "success" | "error" | "warning";
}

// ─── Node Definitions ──────────────────────────────────────────────────────

const NODE_DEFINITIONS: { type: NodeType; label: string; color: string; icon: string; defaultConfig: Record<string, string> }[] = [
  { type: "input", label: "Trigger", color: "border-purple-400 bg-purple-50 dark:bg-purple-900/30", icon: "▶", defaultConfig: { trigger: "manual" } },
  { type: "llm", label: "LLM Call", color: "border-blue-400 bg-blue-50 dark:bg-blue-900/30", icon: "🧠", defaultConfig: { model: "", prompt: "" } },
  { type: "tool", label: "Tool", color: "border-green-400 bg-green-50 dark:bg-green-900/30", icon: "🔧", defaultConfig: { tool: "", params: "{}" } },
  { type: "condition", label: "Condition", color: "border-amber-400 bg-amber-50 dark:bg-amber-900/30", icon: "🔀", defaultConfig: { condition: "" } },
  { type: "code", label: "Code", color: "border-cyan-400 bg-cyan-50 dark:bg-cyan-900/30", icon: "💻", defaultConfig: { language: "python", code: "" } },
  { type: "wait", label: "Wait", color: "border-slate-400 bg-slate-50 dark:bg-slate-900/30", icon: "⏳", defaultConfig: { duration: "5" } },
  { type: "output", label: "Output", color: "border-red-400 bg-red-50 dark:bg-red-900/30", icon: "⏹", defaultConfig: {} },
];

const nodeTypeColors: Record<string, string> = Object.fromEntries(
  NODE_DEFINITIONS.map((n) => [n.type, n.color])
);

// ─── Components ────────────────────────────────────────────────────────────

function WorkflowNode({ data }: { data: any }) {
  return (
    <div className={`px-4 py-2 rounded-lg border-2 shadow-sm min-w-[140px] ${nodeTypeColors[data.nodeType] || "border-gray-300 bg-white"}`}>
      <div className="flex items-center gap-2">
        <span className="text-base">{data.icon || "●"}</span>
        <div>
          <div className="text-xs font-semibold text-gray-800 dark:text-gray-100">{data.label}</div>
          {data.subLabel && <div className="text-[10px] text-gray-500 dark:text-gray-400 mt-0.5 truncate max-w-[120px]">{data.subLabel}</div>}
        </div>
      </div>
      {data.status && (
        <div className={`mt-1.5 text-[10px] font-medium ${
          data.status === "running" ? "text-blue-500" :
          data.status === "success" ? "text-green-500" :
          data.status === "error" ? "text-red-500" : "text-gray-400"
        }`}>
          {data.status === "running" ? "⟳ Running" : data.status === "success" ? "✓ Success" : data.status === "error" ? "✗ Failed" : ""}
        </div>
      )}
    </div>
  );
}

const nodeTypes: NodeTypes = {
  workflowNode: WorkflowNode,
};

// ─── Main Page ─────────────────────────────────────────────────────────────

export default function WorkflowPage() {
  const [nodes, setNodes] = useNodesState<Node>([]);
  const [edges, setEdges] = useEdgesState<Edge>([]);
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);
  const [executing, setExecuting] = useState(false);
  const [result, setResult] = useState("");
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [showPanel, setShowPanel] = useState<"config" | "logs" | "export">("config");
  const [meta, setMeta] = useState<WorkflowMeta>({ name: "My Workflow", description: "", version: "1.0", tags: [] });
  const [edgeType, setEdgeType] = useState<"default" | "step">("default");

  useEffect(() => {
    // Initialize with start node
    setNodes([
      {
        id: "start",
        type: "workflowNode",
        position: { x: 300, y: 50 },
        data: { label: "Start", nodeType: "input", icon: "▶" },
      },
    ]);
  }, [setNodes]);

  const addLog = useCallback((message: string, type: LogEntry["type"] = "info") => {
    setLogs((prev) => [...prev, { time: new Date().toLocaleTimeString(), message, type }]);
  }, []);

  const onConnect = useCallback(
    (params: Connection) => {
      setEdges((eds) => addEdge({ ...params, animated: true, style: { stroke: "#6366f1", strokeWidth: 2 } }, eds));
      addLog(`Connected: ${params.source} → ${params.target}`, "info");
    },
    [setEdges, addLog]
  );

  const onNodesChange = useCallback(
    (changes: NodeChange[]) => setNodes((nds) => applyNodeChanges(changes, nds)),
    [setNodes]
  );
  const onEdgesChange = useCallback(
    (changes: any) => setEdges((eds) => applyEdgeChanges(changes, eds)),
    [setEdges]
  );

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      const type = event.dataTransfer.getData("application/reactflow") as NodeType;
      if (!type) return;

      const viewport = document.querySelector(".react-flow__viewport")?.getBoundingClientRect();
      const position = {
        x: event.clientX - (viewport?.left || 0) - 80,
        y: event.clientY - (viewport?.top || 0) - 20,
      };

      const def = NODE_DEFINITIONS.find((n) => n.type === type);
      const newNode: Node = {
        id: `${type}-${Date.now()}`,
        type: "workflowNode",
        position,
        data: {
          label: def?.label || type,
          nodeType: type,
          icon: def?.icon || "●",
          config: { ...(def?.defaultConfig || {}) },
        },
      };
      setNodes((nds) => [...nds, newNode]);
      addLog(`Added node: ${def?.label || type}`, "success");
    },
    [setNodes, addLog]
  );

  const onDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
  }, []);

  const deleteSelected = useCallback(() => {
    if (selectedNode) {
      setNodes((nds) => nds.filter((n) => n.id !== selectedNode.id));
      setEdges((eds) => eds.filter((e) => e.source !== selectedNode.id && e.target !== selectedNode.id));
      setSelectedNode(null);
      addLog(`Deleted node: ${selectedNode.id}`, "warning");
    }
  }, [selectedNode, setNodes, setEdges, addLog]);

  const [savedWorkflows, setSavedWorkflows] = useState<any[]>([]);
  const [showSaved, setShowSaved] = useState(false);

  useEffect(() => {
    api("/v1/workflows", { skipAuth: true }).then((d) => {
      if (Array.isArray(d?.data)) setSavedWorkflows(d.data);
    }).catch(() => {});
  }, []);

  const handleRunGraph = async () => {
    setExecuting(true);
    setResult("");
    setLogs([]);

    const steps = nodes
      .filter((n) => n.id !== "start")
      .map((n) => {
        const d = n.data as Record<string, any>;
        return {
          id: n.id,
          tool: d.nodeType === "llm" ? "llm_chat" :
                d.nodeType === "tool" ? (d.config?.tool || "bash") :
                d.nodeType === "wait" ? "sleep" :
                d.nodeType === "condition" ? "condition_check" : "custom_code",
          params: d.config || {},
        };
      });

    const graphDef = { name: meta.name || "Workflow", steps, env: {} };

    addLog("Saving workflow...", "info");

    try {
      // Save workflow definition
      const createResp = await api("/v1/workflows", {
        method: "POST",
        body: JSON.stringify({
          name: meta.name || "Workflow",
          description: meta.description,
          definition: graphDef,
        }),
      });
      const wfId = createResp?.data?.id;
      if (!wfId) throw new Error("Failed to create workflow");

      addLog("Executing workflow...", "info");

      // Execute via real workflow API
      const execResp = await api(`/v1/workflows/${wfId}/execute`, { method: "POST", skipAuth: true });
      const execData = execResp?.data;

      if (execData?.steps) {
        const stepResults = execData.steps;
        setResult(execData.status === "completed" ? "✅ All steps completed" : `❌ Failed: status=${execData.status}`);

        // Update node statuses
        setNodes((nds) =>
          nds.map((n) => {
            const step = stepResults.find((s: any) => s.id === n.id);
            return {
              ...n,
              data: {
                ...n.data,
                status: step ? (step.status === "success" ? "success" : "failed") : n.data.status || undefined,
                output: step?.output || step?.error || undefined,
              },
            };
          })
        );

        stepResults.forEach((s: any) => {
          addLog(`${s.tool}: ${s.status} (${s.duration_ms}ms)${s.error ? ` — ${s.error}` : ""}`, s.status === "success" ? "success" : "error");
        });
      } else {
        setResult(JSON.stringify(execData));
        addLog("Workflow execution completed", "success");
      }
    } catch (err: any) {
      setResult(`Error: ${err.message}`);
      addLog(`Execution error: ${err.message}`, "error");
    } finally {
      setExecuting(false);
    }
  };

  const handleSaveWorkflow = async () => {
    const steps = nodes.map((n) => ({
      id: n.id, position: n.position, data: n.data,
    }));
    try {
      await api("/v1/workflows", {
        method: "POST",
        body: JSON.stringify({
          name: meta.name || "Workflow",
          description: meta.description,
          definition: { name: meta.name, steps, env: {} },
        }),
      });
      addLog("Workflow saved", "success");
      // Refresh list
      const d = await api("/v1/workflows", { skipAuth: true });
      if (Array.isArray(d?.data)) setSavedWorkflows(d.data);
    } catch (err: any) {
      addLog(`Save failed: ${err.message}`, "error");
    }
  };

  // Reload saved workflow
  const handleLoadWorkflow = async (id: string) => {
    try {
      const resp = await api(`/v1/workflows/${id}`, { skipAuth: true });
      const wf = resp?.data;
      if (!wf?.definition) return;
      const def = typeof wf.definition === "string" ? JSON.parse(wf.definition) : wf.definition;
      if (def.steps) {
        // Reconstruct nodes from saved definition
        const loadedNodes: Node[] = def.steps.map((s: any, i: number) => ({
          id: s.id || `node_${i}`,
          type: "workflowNode",
          position: s.position || { x: 100, y: i * 100 + 100 },
          data: s.data || { nodeType: "tool", config: { tool: s.tool || "bash" }, label: s.tool || "Step" },
        }));
        // Ensure start node
        loadedNodes.unshift({
          id: "start",
          type: "workflowNode",
          position: { x: 400, y: 10 },
          data: { nodeType: "input", config: { trigger: "manual" }, label: "Start", status: "idle" },
        });
        setNodes(loadedNodes);
        setMeta({ ...meta, name: wf.name || "Workflow", description: wf.description || "" });
        addLog(`Loaded: ${wf.name}`, "info");
      }
    } catch (err: any) {
      addLog(`Load failed: ${err.message}`, "error");
    }
  };

  const handleExportJSON = () => {
    const workflow = {
      name: meta.name,
      description: meta.description,
      version: meta.version,
      tags: meta.tags,
      nodes: nodes.map((n) => ({
        id: n.id,
        type: "workflowNode",
        position: n.position,
        data: n.data,
      })),
      edges: edges.map((e) => ({
        source: e.source,
        target: e.target,
      })),
    };
    const blob = new Blob([JSON.stringify(workflow, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${meta.name.replace(/\s+/g, "_")}.json`;
    a.click();
    URL.revokeObjectURL(url);
    addLog("Workflow exported to JSON", "success");
  };

  const handleClearAll = () => {
    setNodes([
      {
        id: "start",
        type: "workflowNode",
        position: { x: 300, y: 50 },
        data: { label: "Start", nodeType: "input", icon: "▶" },
      },
    ]);
    setEdges([]);
    setSelectedNode(null);
    setLogs([]);
    addLog("Canvas cleared", "warning");
  };

  return (
    <div className="flex h-screen bg-gray-50 dark:bg-gray-900">
      {/* Sidebar — Node Palette */}
      <div className="w-56 bg-white dark:bg-gray-800 p-4 border-r dark:border-gray-700 flex flex-col">
        <div className="mb-4">
          <input
            className="w-full px-3 py-2 text-sm border rounded-lg dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100"
            placeholder="Workflow name..."
            value={meta.name}
            onChange={(e) => setMeta({ ...meta, name: e.target.value })}
          />
        </div>

        <h3 className="text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-2">Nodes</h3>
        <div className="space-y-1.5 flex-1 overflow-y-auto">
          {NODE_DEFINITIONS.map((def) => (
            <div
              key={def.type}
              className={`flex items-center gap-2 px-3 py-2 rounded-lg cursor-grab text-xs font-medium border-2 transition-all hover:shadow-md ${def.color}`}
              draggable
              onDragStart={(e) => {
                e.dataTransfer.setData("application/reactflow", def.type);
                e.dataTransfer.effectAllowed = "move";
              }}
            >
              <span className="text-sm">{def.icon}</span>
              <span className="text-gray-700 dark:text-gray-200">{def.label}</span>
            </div>
          ))}
        </div>

        <div className="mt-4 space-y-2 border-t dark:border-gray-700 pt-4">
          <button
            onClick={handleRunGraph}
            disabled={executing}
            className="w-full px-3 py-2.5 bg-gradient-to-r from-blue-600 to-indigo-600 text-white text-sm font-medium rounded-lg hover:from-blue-700 hover:to-indigo-700 disabled:opacity-50 transition-all"
          >
            {executing ? "⟳ Running..." : "▶ Run Workflow"}
          </button>
          <button onClick={handleSaveWorkflow} className="w-full px-3 py-2 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 text-sm rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600 transition-all">
            💾 Save
          </button>
          <button onClick={() => setShowSaved(!showSaved)} className="w-full px-3 py-2 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 text-sm rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600 transition-all">
            📂 {showSaved ? "Hide Saved" : "Load Saved"}
          </button>
          <button onClick={handleExportJSON} className="w-full px-3 py-2 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 text-sm rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600 transition-all">
            📥 Export JSON
          </button>
          <button onClick={handleClearAll} className="w-full px-3 py-2 bg-red-50 dark:bg-red-900/30 text-red-600 dark:text-red-400 text-sm rounded-lg hover:bg-red-100 dark:hover:bg-red-900/50 transition-all">
            🗑 Clear Canvas
          </button>
          {showSaved && savedWorkflows.length > 0 && (
            <div className="max-h-40 overflow-auto space-y-1 border-t dark:border-gray-700 pt-2 mt-2">
              <p className="text-[10px] text-gray-400 font-medium px-1">Saved Workflows</p>
              {savedWorkflows.map((wf: any) => (
                <button key={wf.id} onClick={() => handleLoadWorkflow(wf.id)}
                  className="w-full text-left px-2 py-1.5 text-xs rounded hover:bg-gray-100 dark:hover:bg-gray-700 truncate">
                  {wf.name || wf.id}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Canvas */}
      <div className="flex-1 relative" onDrop={onDrop} onDragOver={onDragOver}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onNodeClick={(_, node) => setSelectedNode(node)}
          onPaneClick={() => setSelectedNode(null)}
          nodeTypes={nodeTypes}
          deleteKeyCode={["Backspace", "Delete"]}
          fitView
          attributionPosition="bottom-left"
        >
          <MiniMap
            nodeStrokeColor="#6366f1"
            nodeColor={(n) => n.data?.nodeType === "input" ? "#a78bfa" : "#6366f1"}
            maskColor="rgba(0,0,0,0.1)"
            style={{ borderRadius: 8, border: "1px solid #e5e7eb" }}
          />
          <Controls showInteractive={false} />
          <Background variant={"dots" as any} gap={20} size={1} color="#e5e7eb" />
        </ReactFlow>
      </div>

      {/* Bottom Panel — Config / Logs / Export */}
      <div className="absolute bottom-0 left-56 right-0 bg-white dark:bg-gray-800 border-t dark:border-gray-700 shadow-lg">
        <div className="flex items-center gap-1 px-4 pt-2 border-b dark:border-gray-700">
          {[
            { id: "config", label: "⚙ Config" },
            { id: "logs", label: "📋 Logs" },
            { id: "export", label: "📤 Export" },
          ].map((tab) => (
            <button key={tab.id} onClick={() => setShowPanel(tab.id as any)}
              className={`px-4 py-2 text-xs font-medium rounded-t-lg ${
                showPanel === tab.id ? "bg-blue-50 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400 border-b-2 border-blue-500" : "text-gray-500 hover:text-gray-700"
              }`}>
              {tab.label}
            </button>
          ))}
        </div>

        <div className="p-4 max-h-48 overflow-y-auto">
          {showPanel === "config" && selectedNode && (
            <div>
              <div className="flex items-center justify-between mb-3">
                <h4 className="text-sm font-semibold text-gray-700 dark:text-gray-300">
                  {(selectedNode.data as any).label} <span className="text-xs text-gray-400 font-normal">({selectedNode.id})</span>
                </h4>
                <button onClick={deleteSelected} className="px-2 py-1 text-xs bg-red-100 dark:bg-red-900/30 text-red-600 rounded hover:bg-red-200">Delete</button>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Label</label>
                  <input className="w-full p-1.5 text-xs border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100"
                    value={(selectedNode.data as any).label}
                    onChange={(e) => setNodes((nds) => nds.map((n) => n.id === selectedNode.id ? { ...n, data: { ...n.data, label: e.target.value } } : n))} />
                </div>
                {(selectedNode.data as any).config ? Object.keys((selectedNode.data as any).config).map((key) => (
                  <div key={key}>
                    <label className="block text-xs text-gray-500 mb-1 capitalize">{key.replace(/_/g, " ")}</label>
                    <input className="w-full p-1.5 text-xs border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-100 font-mono"
                      value={(selectedNode.data as any).config?.[key] || ""}
                      onChange={(e) => setNodes((nds) =>
                        nds.map((n) => n.id === selectedNode.id ? { ...n, data: { ...n.data, config: { ...(n.data as any).config, [key]: e.target.value } } } : n)
                      )} />
                  </div>
                )) : null}
              </div>
            </div>
          )}

          {showPanel === "config" && !selectedNode && (
            <p className="text-sm text-gray-400 text-center py-4">Click a node to edit its configuration</p>
          )}

          {showPanel === "logs" && (
            <div className="space-y-1">
              {logs.length === 0 && <p className="text-sm text-gray-400 text-center py-4">No logs yet. Run the workflow to see execution logs.</p>}
              {logs.map((log, i) => (
                <div key={i} className={`text-xs font-mono px-2 py-1 rounded ${
                  log.type === "error" ? "bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400" :
                  log.type === "success" ? "bg-green-50 dark:bg-green-900/20 text-green-600 dark:text-green-400" :
                  log.type === "warning" ? "bg-amber-50 dark:bg-amber-900/20 text-amber-600" :
                  "bg-gray-50 dark:bg-gray-700/50 text-gray-600 dark:text-gray-400"
                }`}>
                  <span className="text-gray-400 mr-2">{log.time}</span>
                  {log.message}
                </div>
              ))}
            </div>
          )}

          {showPanel === "export" && (
            <div>
              {result && (
                <div className="mb-3">
                  <h4 className="text-xs font-semibold text-gray-500 mb-1">Execution Result</h4>
                  <pre className="text-xs bg-gray-50 dark:bg-gray-900 p-3 rounded max-h-32 overflow-auto whitespace-pre-wrap">{result}</pre>
                </div>
              )}
              <p className="text-xs text-gray-400">
                Nodes: {nodes.length} | Edges: {edges.length} | Ready to export as JSON workflow definition.
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
