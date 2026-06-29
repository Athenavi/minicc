"use client";

interface ShellCardProps {
  command: string;
  description?: string;
  timeout?: number;
}

export function ShellCard({ command, description, timeout }: ShellCardProps) {
  return (
    <div className="bg-black text-green-400 rounded-lg overflow-hidden border border-green-800">
      {/* Terminal header */}
      <div className="flex items-center gap-1.5 px-3 py-1.5 bg-zinc-900 border-b border-zinc-700">
        <span className="w-2.5 h-2.5 rounded-full bg-red-500" />
        <span className="w-2.5 h-2.5 rounded-full bg-yellow-500" />
        <span className="w-2.5 h-2.5 rounded-full bg-green-500" />
        <span className="text-xs text-zinc-400 ml-2">bash</span>
        {timeout && <span className="text-[10px] text-zinc-500 ml-auto">⏱ {timeout}s</span>}
      </div>

      {/* Description */}
      {description && (
        <div className="px-3 py-1.5 text-xs text-zinc-300 bg-zinc-900 border-b border-zinc-800">
          💬 {description}
        </div>
      )}

      {/* Command */}
      <pre className="px-3 py-2 text-sm font-mono whitespace-pre-wrap overflow-x-auto">
        <span className="text-green-600">$ </span>{command}
      </pre>
    </div>
  );
}
