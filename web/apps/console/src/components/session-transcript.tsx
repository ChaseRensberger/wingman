import { ArrowClockwiseIcon, CheckIcon, CopyIcon, WarningCircleIcon } from "@phosphor-icons/react";
import { useMemo } from "react";

import WingmanIcon from "@/assets/icon-128.png";
import { ChatMessage } from "@/components/chat-message";
import { ExplorationGroup } from "@/components/exploration-group";
import { RawMessages } from "@/components/raw-messages";
import { ToolActivityItem } from "@/components/tool-activity";
import { useTranscriptScroll } from "@/hooks/use-transcript-scroll";
import { indexInstructionDiscoveries, type InstructionDiscovery } from "@/lib/model-input";
import { shouldShowThinking } from "@/lib/session-detail";
import { projectTranscript } from "@/lib/transcript";
import type { StreamingPart } from "@/lib/streaming-parts";
import type {
  Message,
  ModelCall,
  PermissionRequest,
  ToolActivity,
  ToolCallPart,
  ToolResultPart,
} from "@/lib/types";
import { Button } from "@wingman/core/components/core/button";
import { Spinner } from "@wingman/core/components/core/spinner";

type FailedRun = {
  message: string;
  agentId: string;
  modelRef: string;
  kind?: "message" | "action";
  error: string;
};

type Props = {
  messages: Message[];
  rawMessages: Message[];
  jsonMode: boolean;
  greeting: string;
  streamingText: string;
  streamingParts: StreamingPart[];
  isReasoningStreaming: boolean;
  isStreaming: boolean;
  toolCallsById: Map<string, ToolCallPart>;
  toolResultsById: Map<string, ToolResultPart>;
  toolActivitiesById: Map<string, ToolActivity>;
  modelCallsByMessageId: Map<string, ModelCall>;
  modelCalls: ModelCall[];
  onInspectCall: (id: string) => void;
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
      <Spinner size={28} className="size-7" label="Responding" />
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
        Allow once permits only this call. Always allow remembers this action for this session,
        including after a server restart.
      </p>
      {(request.action === "read" || request.action === "edit") &&
        request.resources.some((resource) => resource.endsWith("/*")) && (
          <p className="mt-2 text-xs text-muted-foreground">
            A directory ending in /* includes its files and subdirectories. Read approval does not
            permit changes.
          </p>
        )}
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
  streamingParts,
  isReasoningStreaming,
  isStreaming,
  toolCallsById,
  toolResultsById,
  toolActivitiesById,
  modelCallsByMessageId,
  modelCalls,
  onInspectCall,
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
  const instructionDiscoveries = useMemo(
    () => indexInstructionDiscoveries(modelCalls),
    [modelCalls],
  );
  const rows = projectTranscript({
    modelCalls,
    messages,
    toolCallsById,
    toolResultsById,
    toolActivitiesById,
    streamingParts,
    isReasoningStreaming,
    streamingText,
    isStreaming,
  });
  const hasActiveTool = [...toolActivitiesById.values()].some(
    (activity) => activity.status === "pending" || activity.status === "running",
  );
  const hasActiveCompaction = messages.some(
    (message) =>
      message.state === "in_progress" &&
      message.content.some((part) => part.type === "compaction_marker"),
  );
  const showThinking = shouldShowThinking(
    isStreaming,
    Boolean(
      streamingText ||
      streamingParts.length ||
      hasActiveTool ||
      hasActiveCompaction ||
      permissionRequests.length,
    ),
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
              <p className="mb-3 text-sm text-muted-foreground">
                Stored conversation records. These are not the input of any single model call. Use
                the Inspector to view recorded model input.
              </p>
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
          ) : messages.length === 0 && !isStreaming && !failedRun ? (
            <div className="flex flex-1 items-start justify-center pt-[20dvh] pb-12 text-center">
              <div className="flex flex-col items-center gap-4">
                <img src={WingmanIcon} className="size-16" alt="Wingman logo" />
                <div className="text-2xl font-semibold sm:text-3xl">{greeting}</div>
              </div>
            </div>
          ) : (
            <div className="flex flex-col gap-2 pb-5" data-slot="transcript-rows">
              {rows.map((row, index) => {
                const previous = rows[index - 1];
                const turnBoundary =
                  index > 0 &&
                  ((row.type === "message" && row.message.role === "user") ||
                    (previous?.type === "message" && previous.message.role === "user"));
                const call =
                  row.type === "message"
                    ? modelCallsByMessageId.get(row.message.id ?? "")
                    : undefined;
                const rowTools =
                  row.type === "exploration" ? row.tools : row.type === "tool" ? [row.tool] : [];
                const discovered = new Map<string, InstructionDiscovery>();
                for (const tool of rowTools) {
                  const sources = [
                    ...(instructionDiscoveries.get(`tool:${tool.call.tool_use_id}`)?.values() ??
                      []),
                    ...(instructionDiscoveries.get(`call:${tool.call.call_id}`)?.values() ?? []),
                  ].sort((a, b) => a.order - b.order);
                  for (const source of sources) {
                    if (!discovered.has(source.path)) discovered.set(source.path, source);
                  }
                }
                return (
                  <div
                    key={row.key}
                    className={turnBoundary ? "min-w-0 mt-4" : "min-w-0"}
                    data-slot="transcript-row"
                  >
                    {row.type === "model-call" ? (
                      <details className="mx-4 rounded-md border border-dashed px-3 py-2 text-xs text-muted-foreground">
                        <summary className="cursor-pointer">{row.notices.join(" · ")}</summary>
                        <p className="mt-2">
                          Step {row.call.step} · attempt {row.call.attempt} · {row.call.status}.
                          Settings changes compare recorded calls; this entry is not a conversation
                          message.
                        </p>
                        {row.call.status === "aborted" && (
                          <p className="mt-1">
                            Execution stopped. This does not mean an interruption message was
                            included in a later model request.
                          </p>
                        )}
                        {row.call.trace?.instructions
                          ?.filter((source) => source.resolved_path)
                          .map((source) => (
                            <p key={source.path} className="mt-1 break-all">
                              Guidance: {source.path} · discovered by reading{" "}
                              {source.requested_path} → {source.resolved_path}
                            </p>
                          ))}
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="mt-1"
                          onClick={() => onInspectCall(row.call.id)}
                        >
                          Inspect model input
                        </Button>
                      </details>
                    ) : row.type === "exploration" ? (
                      <ExplorationGroup tools={row.tools} permissionRequests={permissionRequests} />
                    ) : row.type === "tool" ? (
                      <div className="px-4">
                        <ToolActivityItem
                          call={row.tool.call}
                          result={row.tool.result}
                          activity={row.tool.activity}
                        />
                      </div>
                    ) : (
                      <ChatMessage
                        message={row.message}
                        sourceMessage={row.sourceMessage}
                        showMeta={row.showMeta}
                        isStreaming={row.isStreaming}
                        toolCallsById={toolCallsById}
                        toolResultsById={toolResultsById}
                        toolActivitiesById={toolActivitiesById}
                        modelCall={call}
                        agentName={agentNames.get(call?.agent_id ?? "")}
                        onInspectCall={onInspectCall}
                      />
                    )}
                    {[...discovered.values()].map((source) => (
                      <details
                        key={source.path}
                        className="mx-4 mt-1 rounded-md border border-dashed px-3 py-2 text-xs text-muted-foreground"
                      >
                        <summary className="cursor-pointer">
                          Directory instructions discovered · {source.path}
                        </summary>
                        <p className="mt-2 break-all">
                          Read {source.requested} → {source.resolved}
                        </p>
                        <Button
                          type="button"
                          size="sm"
                          variant="ghost"
                          onClick={() => onInspectCall(source.id)}
                        >
                          View instructions in first affected call
                        </Button>
                      </details>
                    ))}
                  </div>
                );
              })}
              {permissionRequests.map((request) => (
                <PermissionRequestCard
                  key={request.id}
                  request={request}
                  replying={permissionRepliesInFlight.has(request.id)}
                  onReply={onReplyPermission}
                />
              ))}
              {showThinking && <ThinkingIndicator />}
              {failedRun && (
                <div className="mx-4 my-5 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm sm:mx-6">
                  <div className="flex items-start gap-2">
                    <WarningCircleIcon
                      className="mt-0.5 size-4 shrink-0 text-destructive"
                      weight="fill"
                    />
                    <div className="min-w-0 flex-1">
                      <div className="font-medium text-destructive">
                        {failedRun.kind === "action" ? "Action failed" : "Message failed"}
                      </div>
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
