"use client";

/**
 * 工具执行 Hook — 直接调用 POST /api/tools/execute。
 * 替代之前的 /api/submit + /tools <name> 方式。
 */
export function useToolRunner() {
  const runTool = async (name: string, input: Record<string, any> = {}): Promise<string> => {
    try {
      const resp = await fetch("http://localhost:8000/api/tools/execute", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, input }),
      });
      const data = await resp.json();
      if (data.is_error) {
        return `[${name}] Error: ${data.output}`;
      }
      return data.output || `[${name}] Done (no output)`;
    } catch (err: any) {
      return `[${name}] Request failed: ${err.message}`;
    }
  };

  const runTools = async (names: string[]): Promise<string> => {
    const results = await Promise.all(names.map((n) => runTool(n)));
    return results.join("\n\n---\n\n");
  };

  return { runTool, runTools };
}
