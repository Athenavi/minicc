"use client";

import { api } from "@/lib/api";

export function useToolRunner() {
  const runTool = async (name: string, input: Record<string, any> = {}): Promise<string> => {
    try {
      const data = await api("/v1/tools/execute", {
        method: "POST",
        body: JSON.stringify({ name, input }),
      });
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
