"use client";

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { useMemo } from "react";
import { cn } from "@/lib/utils";

interface MarkdownRendererProps {
  content: string;
  className?: string;
}

export function MarkdownRenderer({ content, className }: MarkdownRendererProps) {
  // 代码块复制功能通过全局事件委托实现
  const components = useMemo(() => ({
    code: ({ className: cls, children, ...props }: React.ComponentPropsWithoutRef<"code">) => {
      const isInline = !cls;
      if (isInline) {
        return (
          <code className="bg-muted px-1 py-0.5 rounded text-sm font-mono" {...props}>
            {children}
          </code>
        );
      }
      return (
        <div className="relative group">
          <pre className="bg-muted/80 border rounded-lg p-4 overflow-x-auto text-sm">
            <code className={cls} {...props}>
              {children}
            </code>
          </pre>
        </div>
      );
    },
    table: ({ children }: React.ComponentPropsWithoutRef<"table">) => (
      <div className="overflow-x-auto my-2">
        <table className="border-collapse border text-sm">{children}</table>
      </div>
    ),
  }), []);

  return (
    <div className={cn("prose prose-sm dark:prose-invert max-w-none", className)}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {content}
      </ReactMarkdown>
    </div>
  );
}
