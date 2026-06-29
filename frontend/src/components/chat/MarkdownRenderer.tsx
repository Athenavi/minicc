"use client";

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import { useMemo, useState } from "react";
import { cn } from "@/lib/utils";
import { Check, Copy } from "lucide-react";

export function MarkdownRenderer({ content, className }: { content: string; className?: string }) {
  return (
    <div className={cn("prose prose-sm dark:prose-invert max-w-none", className)}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeHighlight]}
        components={{
          code: CodeBlock,
          table: ({ children }) => (
            <div className="overflow-x-auto my-2">
              <table className="border-collapse border text-sm">{children}</table>
            </div>
          ),
          img: ({ src, alt }) => (
            <img src={src} alt={alt} className="rounded-lg max-w-full" loading="lazy" />
          ),
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}

function CodeBlock({ className, children, ...props }: React.ComponentPropsWithoutRef<"code">) {
  const [copied, setCopied] = useState(false);
  const isInline = !className;

  const handleCopy = async () => {
    const text = String(children).replace(/\n$/, "");
    await navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  if (isInline) {
    return (
      <code className="bg-gray-100 dark:bg-gray-800 px-1.5 py-0.5 rounded text-sm font-mono text-pink-600 dark:text-pink-400" {...props}>
        {children}
      </code>
    );
  }

  const lang = className?.replace("language-", "") || "";

  return (
    <div className="relative group my-3">
      <div className="flex items-center justify-between px-4 py-1.5 bg-gray-200 dark:bg-gray-700 rounded-t-lg text-xs text-gray-500 dark:text-gray-400">
        <span>{lang || "code"}</span>
        <button onClick={handleCopy} className="flex items-center gap-1 hover:text-gray-700 dark:hover:text-gray-200 transition">
          {copied ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre className="bg-gray-100 dark:bg-gray-900 border-x border-b dark:border-gray-700 rounded-b-lg p-4 overflow-x-auto text-sm leading-6">
        <code className={className} {...props}>{children}</code>
      </pre>
    </div>
  );
}
