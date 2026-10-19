import { lookupToolActivity, sameToolActivity, toolActivityKey } from "@/lib/tool-activity-state";
import type {
  Message,
  Part,
  ToolActivity,
  ToolCallPart,
  ToolPart,
  ToolResultPart,
} from "@/lib/types";

export type TranscriptTool = {
  key: string;
  call: ToolCallPart;
  result?: ToolResultPart;
  activity?: ToolActivity;
};

export type TranscriptRow =
  | {
      type: "message";
      key: string;
      message: Message;
      sourceMessage: Message;
      showMeta: boolean;
      isStreaming?: boolean;
    }
  | { type: "exploration"; key: string; tools: TranscriptTool[] }
  | { type: "tool"; key: string; tool: TranscriptTool };

const explorationTools = new Set(["read", "grep", "glob", "websearch", "webfetch"]);

export function toolPartProps(tool: ToolPart): Omit<TranscriptTool, "key"> {
  return {
    call: {
      type: "tool_call",
      tool_use_id: tool.tool_use_id,
      call_id: tool.call_id,
      name: tool.name,
      input: tool.input,
    },
    result:
      tool.state === "completed" || tool.state === "error"
        ? {
            type: "tool_result",
            tool_use_id: tool.tool_use_id,
            call_id: tool.call_id,
            name: tool.name,
            output: tool.output_parts ?? (tool.output ? [{ type: "text", text: tool.output }] : []),
            structured: tool.structured,
            is_error: tool.state === "error",
            metadata: tool.metadata,
          }
        : undefined,
    activity: {
      tool_use_id: tool.tool_use_id,
      call_id: tool.call_id,
      tool: tool.name,
      status: tool.state,
      input: tool.input,
      output: tool.output,
      metadata: tool.metadata,
      error: tool.error,
    },
  };
}

export function transcriptToolStatus(tool: TranscriptTool): ToolActivity["status"] {
  return (
    tool.activity?.status ??
    (tool.result ? (tool.result.is_error ? "error" : "completed") : "pending")
  );
}

export function explorationSummary(tools: TranscriptTool[]) {
  const counts = new Map<string, number>();
  for (const { call } of tools) {
    const name =
      call.name === "grep" || call.name === "glob" || call.name === "websearch"
        ? "search"
        : call.name === "webfetch"
          ? "fetch"
          : "read";
    counts.set(name, (counts.get(name) ?? 0) + 1);
  }
  const active = tools.some((tool) => ["pending", "running"].includes(transcriptToolStatus(tool)));
  const names = [...counts].map(
    ([name, count]) => `${count} ${name}${count === 1 ? "" : name === "read" ? "s" : "es"}`,
  );
  return `${active ? "Exploring" : "Explored"}: ${names.join(", ")}`;
}

export function projectTranscript({
  messages,
  toolCallsById,
  toolResultsById,
  toolActivitiesById,
  streamingReasoning = "",
  isReasoningStreaming = false,
  streamingText = "",
  isStreaming = false,
}: {
  messages: Message[];
  toolCallsById: Map<string, ToolCallPart>;
  toolResultsById: Map<string, ToolResultPart>;
  toolActivitiesById: Map<string, ToolActivity>;
  streamingReasoning?: string;
  isReasoningStreaming?: boolean;
  streamingText?: string;
  isStreaming?: boolean;
}): TranscriptRow[] {
  const rows: TranscriptRow[] = [];
  const renderedCalls: ToolCallPart[] = [];
  const appendTool = (tool: TranscriptTool) => {
    if (!explorationTools.has(tool.call.name)) {
      rows.push({ type: "tool", key: tool.key, tool });
      return;
    }
    const previous = rows.at(-1);
    if (previous?.type === "exploration") previous.tools.push(tool);
    else rows.push({ type: "exploration", key: `exploration:${tool.key}`, tools: [tool] });
  };

  messages.forEach((message, messageIndex) => {
    if (message.role === "tool") return;
    const messageKey = message.id ?? messageIndex;
    if (
      message.role !== "assistant" ||
      message.content.some((part) => part.type === "compaction_marker")
    ) {
      rows.push({
        type: "message",
        key: `message:${messageKey}`,
        message,
        sourceMessage: message,
        showMeta: true,
      });
      return;
    }
    let content: Part[] = [];
    let start = 0;
    const flush = () => {
      if (content.length === 0) return;
      rows.push({
        type: "message",
        key: `message:${messageKey}:${start}`,
        message: { ...message, content },
        sourceMessage: message,
        showMeta: false,
      });
      content = [];
    };
    message.content.forEach((part, index) => {
      if (part.type === "tool_result") {
        const result = part as ToolResultPart;
        const key = toolActivityKey(result) ?? result.call_id;
        if (toolCallsById.has(key) || toolCallsById.has(result.call_id)) return;
      }
      if (part.type === "text" && !(part as { text: string }).text.trim()) return;
      if (part.type === "reasoning" && !(part as { reasoning: string }).reasoning.trim()) return;
      if (part.type === "tool" || part.type === "tool_call") {
        const call = part as ToolCallPart;
        renderedCalls.push(call);
        if (explorationTools.has(call.name)) {
          flush();
          const key = toolActivityKey(call) ?? call.call_id;
          appendTool({
            key,
            ...(part.type === "tool"
              ? toolPartProps(part as ToolPart)
              : {
                  call,
                  result: toolResultsById.get(key) ?? toolResultsById.get(call.call_id),
                  activity: lookupToolActivity(toolActivitiesById, call),
                }),
          });
          return;
        }
      }
      if (content.length === 0) start = index;
      content.push(part);
    });
    flush();
  });

  if (streamingReasoning) {
    const message: Message = {
      role: "assistant",
      content: [{ type: "reasoning", reasoning: streamingReasoning }],
    };
    rows.push({
      type: "message",
      key: "streaming-reasoning",
      message,
      sourceMessage: message,
      showMeta: false,
      isStreaming: isReasoningStreaming,
    });
  }
  for (const activity of toolActivitiesById.values()) {
    if (renderedCalls.some((call) => sameToolActivity(call, activity))) continue;
    appendTool({
      key: toolActivityKey(activity) ?? activity.call_id,
      call: {
        type: "tool_call",
        tool_use_id: activity.tool_use_id,
        call_id: activity.call_id,
        name: activity.tool,
        input: activity.input ?? {},
      },
      activity,
    });
  }
  if (streamingText) {
    const message: Message = {
      role: "assistant",
      content: [{ type: "text", text: streamingText }],
    };
    rows.push({
      type: "message",
      key: "streaming-text",
      message,
      sourceMessage: message,
      showMeta: false,
      isStreaming: true,
    });
  }

  let lastResponseMessage: Extract<TranscriptRow, { type: "message" }> | undefined;
  const commitResponseMeta = () => {
    if (lastResponseMessage) lastResponseMessage.showMeta = true;
    lastResponseMessage = undefined;
  };
  for (const row of rows) {
    if (row.type !== "message") continue;
    if (row.message.role === "user") {
      commitResponseMeta();
      continue;
    }
    if (
      row.message.role === "assistant" &&
      row.message.content.some(
        (part) => part.type === "text" && (part as { text: string }).text.trim(),
      )
    ) {
      lastResponseMessage = row;
    }
  }
  const activeTool = [...toolActivitiesById.values()].some((activity) =>
    ["pending", "running"].includes(activity.status),
  );
  if (!isStreaming && !streamingText && !streamingReasoning && !activeTool) commitResponseMeta();

  return rows;
}
