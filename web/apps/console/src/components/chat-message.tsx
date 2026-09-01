import { lazy, Suspense, useState } from "react";
import { CaretRightIcon, CheckIcon, CopyIcon } from "@phosphor-icons/react";

import type {
  Message,
  ModelCall,
  ToolActivity,
  ToolCallPart,
  ToolPart,
  ToolResultPart,
} from "@/lib/types";
import { ReasoningPart } from "@/components/reasoning-part";
import { ToolActivityItem } from "@/components/tool-activity";
import { lookupToolActivity, toolActivityKey } from "@/lib/tool-activity-state";
import { formatDuration } from "@/lib/tool-display";
import { Button } from "@wingman/core/components/core/button";

const Markdown = lazy(async () => {
  const { Markdown } = await import("./markdown");
  return { default: Markdown };
});

function AssistantMeta({
  message,
  call,
  agentName,
}: {
  message: Message;
  call?: ModelCall;
  agentName?: string;
}) {
  const [copied, setCopied] = useState(false);
  const text = message.content
    .filter((part) => part.type === "text")
    .map((part) => (part as { text: string }).text)
    .join("\n");
  const duration =
    call?.started_at && call.completed_at
      ? new Date(call.completed_at).getTime() - new Date(call.started_at).getTime()
      : undefined;
  const model = call?.model_id || message.origin?.model_id;
  const details = [
    agentName,
    model,
    duration !== undefined && duration >= 0 ? formatDuration(duration) : "",
  ].filter(Boolean);

  async function copy() {
    if (!text) return;
    await navigator.clipboard.writeText(text);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1200);
  }

  if (!text && !call) return null;
  return (
    <div className="mt-3 flex min-h-7 items-center gap-2 text-xs text-muted-foreground opacity-0 transition-opacity group-hover/message:opacity-100 group-focus-within/message:opacity-100">
      {text && (
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          onClick={() => void copy()}
          aria-label={copied ? "Copied response" : "Copy response"}
          title={copied ? "Copied" : "Copy response"}
        >
          {copied ? <CheckIcon className="size-3.5" /> : <CopyIcon className="size-3.5" />}
        </Button>
      )}
      {details.length > 0 && <span>{details.join(" · ")}</span>}
    </div>
  );
}

export function ChatMessage({
  message,
  isStreaming = false,
  toolCallsById,
  toolResultsById,
  toolActivitiesById,
  modelCall,
  agentName,
  compactTop = false,
  compactBottom = false,
}: {
  message: Message;
  isStreaming?: boolean;
  toolCallsById?: Map<string, ToolCallPart>;
  toolResultsById?: Map<string, ToolResultPart>;
  toolActivitiesById?: Map<string, ToolActivity>;
  modelCall?: ModelCall;
  agentName?: string;
  compactTop?: boolean;
  compactBottom?: boolean;
}) {
  if (message.role === "tool") return null;
  if (message.content.length === 0) return null;

  const compaction = message.content.find((part) => part.type === "compaction_marker") as
    | {
        summary?: string;
        reason?: "auto" | "manual";
        original_count?: number;
        tokens_before?: number;
      }
    | undefined;
  if (compaction) {
    return (
      <div className="px-4 py-5" data-component="session-compaction-message">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <div className="h-px flex-1 bg-border" />
          <span>Session compacted</span>
          <div className="h-px flex-1 bg-border" />
        </div>
        {compaction.summary && (
          <details className="group/compaction mx-auto mt-2 max-w-3xl text-muted-foreground">
            <summary className="flex cursor-pointer list-none items-center justify-center gap-1 text-xs outline-none focus-visible:ring-3 focus-visible:ring-ring/40 [&::-webkit-details-marker]:hidden">
              <CaretRightIcon className="size-3 transition-transform duration-150 motion-reduce:transition-none group-open/compaction:rotate-90" />
              <span>
                {compaction.reason === "auto" ? "Automatic" : "Manual"} checkpoint
                {compaction.original_count
                  ? ` · ${compaction.original_count.toLocaleString()} messages`
                  : ""}
                {compaction.tokens_before
                  ? ` · ${compaction.tokens_before.toLocaleString()} tokens`
                  : ""}
              </span>
            </summary>
            <div className="mt-3 border-l border-border pl-3">
              <Suspense fallback={<div className="whitespace-pre-wrap text-sm">{compaction.summary}</div>}>
                <Markdown text={compaction.summary} />
              </Suspense>
            </div>
          </details>
        )}
      </div>
    );
  }

  const isUser = message.role === "user";
  const isAssistant = message.role === "assistant";

  return (
    <div
      className={`relative ${compactTop ? "pt-1" : "pt-5"} ${compactBottom ? "pb-0" : "pb-5"} ${
        isUser
          ? "mx-2 bg-primary/[0.03] before:absolute before:inset-y-0 before:-left-2 before:w-0.5 before:bg-primary/35 before:content-[''] after:absolute after:inset-y-0 after:-right-2 after:w-0.5 after:bg-primary/35 after:content-['']"
          : ""
      }`}
    >
      <div
        className={`group/message min-w-0 ${isUser ? "px-2" : "px-4"} text-sm leading-6 ${
          isUser ? "text-foreground" : isAssistant ? "text-foreground" : "text-muted-foreground"
        }`}
      >
        {message.content.map((part, idx) => {
          if (part.type === "text") {
            const textPart = part as { id?: string; text: string };
            if (isAssistant) {
              return (
                <Suspense
                  key={textPart.id ?? idx}
                  fallback={<div className="whitespace-pre-wrap text-sm">{textPart.text}</div>}
                >
                  <Markdown text={textPart.text} isStreaming={isStreaming} />
                </Suspense>
              );
            }
            return (
              <div key={textPart.id ?? idx} className="whitespace-pre-wrap text-sm">
                {textPart.text}
              </div>
            );
          }
          if (part.type === "reasoning") {
            const reasoningPart = part as { id?: string; reasoning: string };
            return (
              <ReasoningPart key={reasoningPart.id ?? idx} reasoning={reasoningPart.reasoning} />
            );
          }
          if (part.type === "tool_call") {
            const call = part as ToolCallPart;
            const key = toolActivityKey(call) ?? call.call_id;
            return (
              <div key={call.id ?? key} className={compactBottom ? "mt-0" : "mt-1"}>
                <ToolActivityItem
                  call={call}
                  result={toolResultsById?.get(key) ?? toolResultsById?.get(call.call_id)}
                  activity={
                    toolActivitiesById ? lookupToolActivity(toolActivitiesById, call) : undefined
                  }
                  compact={compactBottom}
                />
              </div>
            );
          }
          if (part.type === "tool") {
            const tool = part as ToolPart;
            const call = {
              type: "tool_call" as const,
              tool_use_id: tool.tool_use_id,
              call_id: tool.call_id,
              name: tool.name,
              input: tool.input,
            };
            const result =
              tool.state === "completed" || tool.state === "error"
                ? {
                    type: "tool_result" as const,
                    tool_use_id: tool.tool_use_id,
                    call_id: tool.call_id,
                    name: tool.name,
                    output:
                      tool.output_parts ??
                      (tool.output ? [{ type: "text" as const, text: tool.output }] : []),
                    structured: tool.structured,
                    is_error: tool.state === "error",
                    metadata: tool.metadata,
                  }
                : undefined;
            return (
              <div
                key={tool.id ?? tool.tool_use_id ?? tool.call_id}
                className={compactBottom ? "mt-0" : "mt-1"}
              >
                <ToolActivityItem
                  call={call}
                  result={result}
                  activity={{
                    tool_use_id: tool.tool_use_id,
                    call_id: tool.call_id,
                    tool: tool.name,
                    status: tool.state,
                    input: tool.input,
                    output: tool.output,
                    metadata: tool.metadata,
                    error: tool.error,
                  }}
                  compact={compactBottom}
                />
              </div>
            );
          }
          if (part.type === "tool_result") {
            const result = part as ToolResultPart;
            const key = toolActivityKey(result) ?? result.call_id;
            if (toolCallsById?.has(key) || toolCallsById?.has(result.call_id)) return null;
            const call = {
              type: "tool_call" as const,
              tool_use_id: result.tool_use_id,
              call_id: result.call_id,
              name: result.name || "tool",
              input: {},
            };
            return (
              <div key={result.id ?? key} className="mt-1">
                <ToolActivityItem
                  call={call}
                  result={result}
                  activity={
                    toolActivitiesById ? lookupToolActivity(toolActivitiesById, result) : undefined
                  }
                />
              </div>
            );
          }
          return (
            <div key={idx} className="text-xs text-muted-foreground">
              [unknown part: {part.type}]
            </div>
          );
        })}
        {isAssistant && <AssistantMeta message={message} call={modelCall} agentName={agentName} />}
      </div>
    </div>
  );
}
