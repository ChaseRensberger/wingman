import { ArrowClockwiseIcon, CheckIcon, CopyIcon, WarningCircleIcon } from "@phosphor-icons/react";

import WingmanIcon from "@/assets/icon-128.png";
import { ChatMessage } from "@/components/chat-message";
import { HexWaveSpinner } from "@/components/hex-wave-spinner";
import { RawMessages } from "@/components/raw-messages";
import { ReasoningPart } from "@/components/reasoning-part";
import { ToolActivityItem } from "@/components/tool-activity";
import { useTranscriptScroll } from "@/hooks/use-transcript-scroll";
import { shouldShowThinking } from "@/lib/session-detail";
import { sameToolActivity } from "@/lib/tool-activity-state";
import type {
  Message,
  ModelCall,
  PermissionRequest,
  ToolActivity,
  ToolCallPart,
  ToolResultPart,
} from "@/lib/types";
import { Button } from "@wingman/core/components/core/button";

type FailedRun = { message: string; agentId: string; modelRef: string; error: string };

type Props = {
  messages: Message[];
  rawMessages: Message[];
  jsonMode: boolean;
  greeting: string;
  streamingText: string;
  streamingReasoning: string;
  isStreaming: boolean;
  toolCallsById: Map<string, ToolCallPart>;
  toolResultsById: Map<string, ToolResultPart>;
  toolActivitiesById: Map<string, ToolActivity>;
  modelCallsByMessageId: Map<string, ModelCall>;
  agentNames: Map<string, string>;
  failedRun: FailedRun | null;
  permissionRequests: PermissionRequest[];
  permissionRepliesInFlight: ReadonlySet<string>;
  onReplyPermission: (requestID: string, response: "once" | "always" | "reject") => void;
  copiedFailedRunError: boolean;
  onCopyFailedRunError: () => void;
  onRetry: () => void;
  scroll: ReturnType<typeof useTranscriptScroll>;
};

function ThinkingIndicator() {
  return (
    <div className="flex justify-center px-4 py-5">
      <HexWaveSpinner size={28} className="size-7" label="Responding" />
    </div>
  );
}

function PermissionRequestCard({
  request,
  replying,
  onReply,
}: {
  request: PermissionRequest;
  replying: boolean;
  onReply: Props["onReplyPermission"];
}) {
  const headingID = `permission-request-${request.id}`;
  return (
    <section
      aria-labelledby={headingID}
      className="mx-4 my-5 rounded-lg border border-border bg-muted/30 p-4 text-sm sm:mx-6"
    >
      <div className="font-medium" id={headingID}>
        Permission requested
      </div>
      <div className="mt-2">
        <span className="text-muted-foreground">Action: </span>
        <span className="font-medium">{request.action}</span>
      </div>
      <ul className="mt-2 space-y-1" aria-label="Requested resources">
        {request.resources.map((resource, index) => (
          <li key={`${resource}:${index}`} className="break-all text-muted-foreground">
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs text-foreground">
              {resource}
            </code>
          </li>
        ))}
      </ul>
      <p className="mt-3 text-xs text-muted-foreground">
        Always allow is remembered for this session.
      </p>
      <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:justify-end">
        <Button
          size="sm"
          type="button"
          disabled={replying}
          onClick={() => onReply(request.id, "once")}
        >
          Allow once
        </Button>
        <Button
          size="sm"
          variant="outline"
          type="button"
          disabled={replying}
          onClick={() => onReply(request.id, "always")}
        >
          Always allow
        </Button>
        <Button
          size="sm"
          variant="destructive"
          type="button"
          disabled={replying}
          onClick={() => onReply(request.id, "reject")}
        >
          Reject
        </Button>
      </div>
    </section>
  );
}

export function SessionTranscript({
  messages,
  rawMessages,
  jsonMode,
  greeting,
  streamingText,
  streamingReasoning,
  isStreaming,
  toolCallsById,
  toolResultsById,
  toolActivitiesById,
  modelCallsByMessageId,
  agentNames,
  failedRun,
  permissionRequests,
  permissionRepliesInFlight,
  onReplyPermission,
  copiedFailedRunError,
  onCopyFailedRunError,
  onRetry,
  scroll,
}: Props) {
  const renderedToolCalls = messages.flatMap(
    (message) =>
      message.content.filter(
        (part) => part.type === "tool_call" || part.type === "tool",
      ) as ToolCallPart[],
  );
  const pendingToolActivities = [...toolActivitiesById.values()].filter(
    (activity) => !renderedToolCalls.some((call) => sameToolActivity(call, activity)),
  );
  const hasActiveTool = [...toolActivitiesById.values()].some(
    (activity) => activity.status === "pending" || activity.status === "running",
  );
  const showThinking = shouldShowThinking(
    isStreaming,
    Boolean(streamingText || streamingReasoning || hasActiveTool || permissionRequests.length),
  );
  return (
    <div
      className="relative min-h-0 flex-1"
      onPointerEnter={() => scroll.setIsHovered(true)}
      onPointerLeave={() => scroll.setIsHovered(false)}
    >
      <div
        ref={scroll.scrollRef}
        onScroll={scroll.handleScroll}
        onKeyDown={scroll.handleKeyDown}
        tabIndex={0}
        role="region"
        aria-label="Session transcript"
        data-scrollable
        className="h-full overflow-y-auto outline-none focus-visible:ring-3 focus-visible:ring-ring/50 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        <div className="mx-auto flex min-h-full w-full max-w-4xl flex-col px-3 pt-5 pb-0 sm:px-4 sm:pt-6">
          {jsonMode ? (
            <div className="px-4 pb-6 sm:px-6">
              <RawMessages messages={rawMessages} />
              {permissionRequests.map((request) => (
                <PermissionRequestCard
                  key={request.id}
                  request={request}
                  replying={permissionRepliesInFlight.has(request.id)}
                  onReply={onReplyPermission}
                />
              ))}
            </div>
          ) : messages.length === 0 && !isStreaming ? (
            <div className="flex flex-1 items-start justify-center pt-[20dvh] pb-12 text-center">
              <div className="flex flex-col items-center gap-4">
                <img src={WingmanIcon} className="size-16" alt="Wingman logo" />
                <div className="text-2xl font-semibold sm:text-3xl">{greeting}</div>
              </div>
            </div>
          ) : (
            <div>
              {messages.map((message, index) => {
                const call = modelCallsByMessageId.get(message.id ?? "");
                const hasTool =
                  message.role === "assistant" &&
                  message.content.some((part) => part.type === "tool" || part.type === "tool_call");
                const hasText =
                  message.role === "assistant" &&
                  message.content.some((part) => part.type === "text");
                const previousHasTool =
                  index > 0 &&
                  messages[index - 1].role === "assistant" &&
                  messages[index - 1].content.some(
                    (part) => part.type === "tool" || part.type === "tool_call",
                  );
                const nextHasText =
                  index < messages.length - 1 &&
                  messages[index + 1].role === "assistant" &&
                  messages[index + 1].content.some((part) => part.type === "text");
                return (
                  <ChatMessage
                    key={message.id ?? index}
                    message={message}
                    toolCallsById={toolCallsById}
                    toolResultsById={toolResultsById}
                    toolActivitiesById={toolActivitiesById}
                    modelCall={call}
                    agentName={agentNames.get(call?.agent_id ?? "")}
                    compactTop={hasText && previousHasTool}
                    compactBottom={hasTool && nextHasText}
                  />
                );
              })}
              {streamingReasoning && (
                <div className="px-4">
                  <ReasoningPart reasoning={streamingReasoning} isStreaming />
                </div>
              )}
              {permissionRequests.map((request) => (
                <PermissionRequestCard
                  key={request.id}
                  request={request}
                  replying={permissionRepliesInFlight.has(request.id)}
                  onReply={onReplyPermission}
                />
              ))}
              {pendingToolActivities.map((activity) => (
                <div
                  key={activity.tool_use_id ?? `${activity.run_id ?? "legacy"}:${activity.call_id}`}
                  className="px-4"
                >
                  <ToolActivityItem
                    call={{
                      type: "tool_call",
                      tool_use_id: activity.tool_use_id,
                      call_id: activity.call_id,
                      name: activity.tool,
                      input: activity.input ?? {},
                    }}
                    activity={activity}
                  />
                </div>
              ))}
              {streamingText && (
                <ChatMessage
                  message={{ role: "assistant", content: [{ type: "text", text: streamingText }] }}
                  isStreaming
                />
              )}
              {showThinking && <ThinkingIndicator />}
              {failedRun && (
                <div className="mx-4 my-5 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm sm:mx-6">
                  <div className="flex items-start gap-2">
                    <WarningCircleIcon
                      className="mt-0.5 size-4 shrink-0 text-destructive"
                      weight="fill"
                    />
                    <div className="min-w-0 flex-1">
                      <div className="font-medium text-destructive">Message failed</div>
                      <pre
                        data-scrollable
                        tabIndex={0}
                        className="mt-1 max-h-32 overflow-auto whitespace-pre-wrap font-sans text-xs text-muted-foreground"
                      >
                        {failedRun.error}
                      </pre>
                    </div>
                  </div>
                  <div className="mt-3 flex justify-end gap-2">
                    <Button size="sm" variant="ghost" type="button" onClick={onCopyFailedRunError}>
                      {copiedFailedRunError ? (
                        <CheckIcon className="size-4" />
                      ) : (
                        <CopyIcon className="size-4" />
                      )}
                      {copiedFailedRunError ? "Copied" : "Copy error"}
                    </Button>
                    <Button size="sm" type="button" onClick={onRetry} disabled={isStreaming}>
                      <ArrowClockwiseIcon className="size-4" />
                      Retry
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
      {scroll.scrollbar.height > 0 && (
        <div
          className={`absolute right-0 z-10 w-3 select-none transition-opacity duration-200 ${scroll.isHovered || scroll.isScrolling || scroll.isScrollbarDragging ? "opacity-100" : "pointer-events-none opacity-0"}`}
          style={{ height: scroll.scrollbar.height, top: scroll.scrollbar.top }}
          onPointerDown={scroll.handleScrollbarPointerDown}
          onPointerMove={scroll.handleScrollbarPointerMove}
          onPointerUp={scroll.handleScrollbarPointerUp}
          onPointerCancel={scroll.handleScrollbarPointerUp}
        >
          <div className="absolute inset-y-0 left-1/2 w-1 -translate-x-1/2 rounded-full bg-border/70 transition-colors hover:bg-foreground/40" />
        </div>
      )}
    </div>
  );
}
