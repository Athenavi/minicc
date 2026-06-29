"use client";

import { cn } from "@/lib/utils";
import { type Message, type ContentBlock } from "@/lib/types";
import { MarkdownRenderer } from "./MarkdownRenderer";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";

interface MessageBubbleProps {
  message: Message;
}

export function MessageBubble({ message }: MessageBubbleProps) {
  const isUser = message.role === "user";
  const isAssistant = message.role === "assistant";

  return (
    <div className={cn("flex gap-3 items-start", isUser && "flex-row-reverse")}>
      {/* Avatar */}
      <Avatar className="w-8 h-8 shrink-0 mt-1">
        <AvatarFallback className={cn(
          "text-xs font-medium",
          isUser ? "bg-primary text-primary-foreground" : "bg-muted"
        )}>
          {isUser ? "U" : isAssistant ? "M" : "S"}
        </AvatarFallback>
      </Avatar>

      {/* Content */}
      <div className={cn(
        "max-w-[80%] rounded-xl px-4 py-2.5",
        isUser
          ? "bg-primary text-primary-foreground"
          : "bg-muted/50 border"
      )}>
        {typeof message.content === "string" ? (
          isUser ? (
            <p className="text-sm whitespace-pre-wrap">{message.content}</p>
          ) : (
            <MarkdownRenderer content={message.content} />
          )
        ) : (
          <ContentBlocksRenderer blocks={message.content} />
        )}
      </div>
    </div>
  );
}

function ContentBlocksRenderer({ blocks }: { blocks: ContentBlock[] }) {
  return (
    <div className="flex flex-col gap-2">
      {blocks.map((block, i) => {
        if (block.type === "text" && block.text) {
          return <MarkdownRenderer key={i} content={block.text} />;
        }
        if (block.type === "tool_use") {
          return (
            <div key={i} className="text-xs font-mono text-muted-foreground border rounded p-2">
              🔧 调用工具: {block.name}
            </div>
          );
        }
        if (block.type === "tool_result") {
          return (
            <div key={i} className="text-xs font-mono bg-muted rounded p-2">
              {typeof block.content?.[0]?.text === "string"
                ? block.content[0].text.slice(0, 500)
                : "(工具结果)"}
            </div>
          );
        }
        return null;
      })}
    </div>
  );
}
