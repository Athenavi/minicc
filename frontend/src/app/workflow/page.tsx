"use client";

import { useCallback, useState } from "react";
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
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

const initialNodes: Node[] = [
  {
    id: "start",
    type: "input",
    position: { x: 250, y: 0 },
    data: { label: "Start" },
  },
];

const initialEdges: Edge[] = [];

const nodeTypes: NodeTypes = {};

export default function WorkflowPage() {
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);
  const [executing, setExecuting] = useState(false);
  const [result, setResult] = useState("");

  const onConnect = useCallback(
    (params: Connection) => setEdges((eds) => addEdge(params, eds)),
    [setEdges]
  );

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      const type = event.dataTransfer.getData("application/reactflow");
      if (!type) return;
      const position = { x: event.clientX - 100, y: event.clientY - 50 };
      const newNode: Node = {
        id: `${type}-${Date.now()}`,
        type: "default",
        position,
        data: { label: type.charAt(0).toUpperCase() + type.slice(1) },
      };
      setNodes((nds) => [...nds, newNode]);
    },
    [setNodes]
  );

  const onDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
  }, []);

  const handleRunGraph = async () => {
    setExecuting(true);
    setResult("");

    const graphDef = {
      name: "Workflow",
      entry_point: "start",
      nodes: nodes.map((n) => ({
        id: n.id,
        label: n.data.label,
        node_type: n.type === "input" ? "input" : n.type === "output" ? "output" : "llm",
        config: {},
      })),
      edges: edges.map((e) => ({
        source_id: e.source,
        target_id: e.target,
      })),
    };

    try {
      const resp = await fetch("http://localhost:8000/api/submit", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          message: `Run graph: ${JSON.stringify(graphDef)}`,
          session_id: "workflow-ui",
        }),
      });
      const data = await resp.json();
      setResult(data.output || "Graph execution submitted");
    } catch (err: any) {
      setResult(`Error: ${err.message}`);
    } finally {
      setExecuting(false);
    }
  };

  const nodeColors: Record<string, string> = {
    llm: "bg-blue-100 border-blue-400",
    tool: "bg-green-100 border-green-400",
    condition: "bg-amber-100 border-amber-400",
    input: "bg-purple-100 border-purple-400",
    output: "bg-red-100 border-red-400",
  };

  return (
    <div className="flex h-screen">
      {/* Sidebar */}
      <div className="w-48 bg-gray-50 dark:bg-gray-900 p-4 border-r dark:border-gray-700">
        <h3 className="text-sm font-semibold mb-3 text-gray-700 dark:text-gray-300">Nodes</h3>
        {["llm", "tool", "condition", "input", "output"].map((type) => (
          <div
            key={type}
            className={`p-2 mb-2 rounded cursor-grab text-xs font-medium text-center
              ${nodeColors[type] || "bg-gray-100 border-gray-300"}
              dark:bg-opacity-80`}
            draggable
            onDragStart={(e) => {
              e.dataTransfer.setData("application/reactflow", type);
              e.dataTransfer.effectAllowed = "move";
            }}
          >
            {type.charAt(0).toUpperCase() + type.slice(1)}
          </div>
        ))}
        <div className="mt-6 space-y-2">
          <button
            onClick={handleRunGraph}
            disabled={executing}
            className="w-full px-3 py-2 bg-blue-600 text-white text-xs rounded hover:bg-blue-700 disabled:opacity-50"
          >
            {executing ? "Running..." : "▶ Run Graph"}
          </button>
        </div>
      </div>

      {/* Canvas */}
      <div className="flex-1" onDrop={onDrop} onDragOver={onDragOver}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onNodeClick={(_, node) => setSelectedNode(node)}
          fitView
        >
          <MiniMap />
          <Controls />
          <Background />
        </ReactFlow>
      </div>

      {/* Config panel */}
      {selectedNode && (
        <div className="w-64 bg-white dark:bg-gray-800 p-4 border-l dark:border-gray-700">
          <h3 className="text-sm font-semibold mb-2">Node Config</h3>
          <p className="text-xs text-gray-500 mb-4">ID: {selectedNode.id}</p>
          <label className="text-xs block mb-1">Label</label>
          <input
            className="w-full p-1.5 text-xs border rounded dark:bg-gray-700 dark:border-gray-600"
            value={selectedNode.data.label as string}
            onChange={(e) => {
              setNodes((nds) =>
                nds.map((n) =>
                  n.id === selectedNode.id ? { ...n, data: { ...n.data, label: e.target.value } } : n
                )
              );
              setSelectedNode({ ...selectedNode, data: { ...selectedNode.data, label: e.target.value } });
            }}
          />
          {result && (
            <div className="mt-4">
              <h4 className="text-xs font-semibold mb-1">Result</h4>
              <pre className="text-xs bg-gray-100 dark:bg-gray-700 p-2 rounded max-h-40 overflow-auto">{result}</pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
