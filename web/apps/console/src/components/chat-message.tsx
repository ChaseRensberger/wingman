import { lazy, Suspense, useState } from "react";
import { CheckIcon, CopyIcon } from "@phosphor-icons/react";

import type {
  Message,
  ModelCall,
  ToolActivity,
  ToolCallPart,
  ToolPart,
  ToolResultPart,
  Usage,
} from "@/lib/types";
import { ReasoningPart } from "@/components/reasoning-part";
import { ToolActivityItem } from "@/components/tool-activity";
import { lookupToolActivity, toolActivityKey } from "@/lib/tool-activity-state";
import { formatDuration } from "@/lib/tool-display";
import { toolPartProps } from "@/lib/transcript";
import { formatTokenCount } from "@/lib/utils";
import { Button } from "@wingman/core/components/core/button";
import { Spinner } from "@wingman/core/components/core/spinner";

const Markdown = lazy(async () => {
  const { Markdown } = await import("./markdown");
  return { default: Markdown };
});

function AssistantMeta({
  message,
  call,
  agentName,
  onInspectCall,
}: {
  message: Message;
  call?: ModelCall;
  agentName?: string;
  onInspectCall?: (id: string) => void;
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
    <div className="flex min-h-7 flex-wrap items-center gap-2 text-xs text-muted-foreground opacity-0 transition-opacity group-hover/message:opacity-100 group-focus-within/message:opacity-100">
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
      {call && onInspectCall && (
        <Button type="button" variant="ghost" size="sm" onClick={() => onInspectCall(call.id)}>
          Inspect model input
        </Button>
      )}
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
  showMeta = true,
  sourceMessage = message,
  onInspectCall,
}: {
  message: Message;
  isStreaming?: boolean;
  toolCallsById?: Map<string, ToolCallPart>;
  toolResultsById?: Map<string, ToolResultPart>;
  toolActivitiesById?: Map<string, ToolActivity>;
  modelCall?: ModelCall;
  agentName?: string;
  showMeta?: boolean;
  sourceMessage?: Message;
  onInspectCall?: (id: string) => void;
}) {
  if (message.role === "tool") return null;
  if (message.content.length === 0) return null;

  const compaction = message.content.find((part) => part.type === "compaction_marker") as
    | { summary?: string; usage?: Usage }
    | undefined;
  if (compaction) {
    const usage = compaction.usage;
    const running = message.state === "in_progress";
    const label =
      message.state === "failed"
        ? "Compaction failed"
        : !running && usage && usage.input_tokens + usage.output_tokens > 0
          ? `Compaction · ${formatTokenCount(usage.input_tokens).toUpperCase()} in · ${formatTokenCount(usage.output_tokens).toUpperCase()} out`
          : "Compaction";
    const separator = (
      <>
        <span aria-hidden="true" className="h-px flex-1 bg-border" />
        <span className="flex min-w-0 items-center gap-2">
          {running && <Spinner size={16} className="size-4 shrink-0" label="Compaction running" />}
          <span className="transition-colors group-hover/compaction:text-foreground">{label}</span>
        </span>
        <span aria-hidden="true" className="h-px flex-1 bg-border" />
      </>
    );
    return (
      <div data-component="session-compaction-message">
        {compaction.summary ? (
          <details>
            <summary
              className="group/compaction flex min-h-7 cursor-pointer list-none items-center gap-2 rounded-sm text-xs text-muted-foreground outline-none focus-visible:ring-3 focus-visible:ring-ring/50 [&::-webkit-details-marker]:hidden"
              onKeyDown={(event) => {
                if (event.key === " ") event.stopPropagation();
              }}
            >
              {separator}
            </summary>
            <div className="pt-2 text-sm leading-6 text-foreground">
              <p className="mb-2 text-xs text-muted-foreground">
                {message.state === "completed" || !message.state
                  ? "Earlier context was summarized. The stored conversation remains available; inspect a later model call to see the summary and recent context used for that call."
                  : "This is a session operation. Inspect a model call to see the context it actually used."}
              </p>
              <Suspense
                fallback={<div className="whitespace-pre-wrap text-sm">{compaction.summary}</div>}
              >
                <Markdown text={compaction.summary} />
              </Suspense>
            </div>
          </details>
        ) : (
          <div className="flex min-h-7 items-center gap-2 text-xs text-muted-foreground">
            {separator}
          </div>
        )}
      </div>
    );
  }

  const isUser = message.role === "user";
  const isAssistant = message.role === "assistant";

  return (
    <div
      className={`relative ${
        isUser
          ? "bg-primary/[0.03] py-4 before:absolute before:inset-y-0 before:left-0 before:w-0.5 before:bg-primary/35 before:content-[''] after:absolute after:inset-y-0 after:right-0 after:w-0.5 after:bg-primary/35 after:content-['']"
          : ""
      }`}
    >
      <div
        className={`group/message flex min-w-0 flex-col gap-2 ${isUser ? "px-3" : ""} text-sm leading-6 break-words ${
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
              <ReasoningPart
                key={reasoningPart.id ?? idx}
                reasoning={reasoningPart.reasoning}
                isStreaming={isStreaming}
              />
            );
          }
          if (part.type === "tool_call") {
            const call = part as ToolCallPart;
            const key = toolActivityKey(call) ?? call.call_id;
            return (
              <ToolActivityItem
                key={call.id ?? key}
                call={call}
                result={toolResultsById?.get(key) ?? toolResultsById?.get(call.call_id)}
                activity={
                  toolActivitiesById ? lookupToolActivity(toolActivitiesById, call) : undefined
                }
              />
            );
          }
          if (part.type === "tool") {
            const tool = part as ToolPart;
            return (
              <ToolActivityItem
                key={tool.id ?? tool.tool_use_id ?? tool.call_id}
                {...toolPartProps(tool)}
              />
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
              <ToolActivityItem
                key={result.id ?? key}
                call={call}
                result={result}
                activity={
                  toolActivitiesById ? lookupToolActivity(toolActivitiesById, result) : undefined
                }
              />
            );
          }
          return (
            <div key={idx} className="text-xs text-muted-foreground">
              [unknown part: {part.type}]
            </div>
          );
        })}
        {isAssistant && showMeta && (
          <AssistantMeta
            message={sourceMessage}
            call={modelCall}
            agentName={agentName}
            onInspectCall={onInspectCall}
          />
        )}
      </div>
    </div>
  );
}
