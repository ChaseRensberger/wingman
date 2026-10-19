import { lookupToolActivity, sameToolActivity, toolActivityKey } from "@/lib/tool-activity-state";
import { modelCallChanges } from "@/lib/model-input";
import type { StreamingPart } from "@/lib/streaming-parts";
import type {
  Message,
  ModelCall,
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
  | { type: "model-call"; key: string; call: ModelCall; notices: string[] }
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
  streamingParts = [],
  isReasoningStreaming = false,
  streamingText = "",
  isStreaming = false,
  modelCalls = [],
}: {
  messages: Message[];
  toolCallsById: Map<string, ToolCallPart>;
  toolResultsById: Map<string, ToolResultPart>;
  toolActivitiesById: Map<string, ToolActivity>;
  streamingParts?: StreamingPart[];
  isReasoningStreaming?: boolean;
  streamingText?: string;
  isStreaming?: boolean;
  modelCalls?: ModelCall[];
}): TranscriptRow[] {
  const rows: TranscriptRow[] = [];
  const noticesByMessage = new Map<string, Array<{ call: ModelCall; notices: string[] }>>();
  modelCalls.forEach((call, index) => {
    if (!call.assistant_message_id) return;
    const notices = modelCallChanges(call, modelCalls[index - 1]);
    if (call.status === "aborted") notices.push("Model call interrupted");
    if (notices.length === 0) return;
    const entries = noticesByMessage.get(call.assistant_message_id) ?? [];
    entries.push({ call, notices });
    noticesByMessage.set(call.assistant_message_id, entries);
  });
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
    for (const { call, notices } of noticesByMessage.get(message.id ?? "") ?? []) {
      rows.push({ type: "model-call", key: `model-call:${call.id}`, call, notices });
    }
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

  const renderedActivities = new Set<string>();
  const appendActivity = (activity: ToolActivity) => {
    const key = toolActivityKey(activity) ?? activity.call_id;
    if (
      renderedActivities.has(key) ||
      renderedCalls.some((call) => sameToolActivity(call, activity))
    )
      return;
    renderedActivities.add(key);
    appendTool({
      key,
      call: {
        type: "tool_call",
        tool_use_id: activity.tool_use_id,
        call_id: activity.call_id,
        name: activity.tool,
        input: activity.input ?? {},
      },
      activity,
    });
  };
  let textOffset = 0;
  streamingParts.forEach((part, index) => {
    if (part.type === "tool") {
      const activity = lookupToolActivity(toolActivitiesById, { ...part, id: part.partID });
      if (activity) appendActivity(activity);
      return;
    }
    const text =
      part.type === "text"
        ? streamingText.slice(textOffset, textOffset + part.text.length)
        : part.text;
    if (part.type === "text") textOffset += part.text.length;
    if (!text) return;
    const message: Message = {
      role: "assistant",
      content: [
        part.type === "text" ? { type: "text", text } : { type: "reasoning", reasoning: text },
      ],
    };
    rows.push({
      type: "message",
      key: `streaming:${part.messageID}:${part.partID ?? index}`,
      message,
      sourceMessage: message,
      showMeta: false,
      isStreaming:
        part.type === "text" || (isReasoningStreaming && index === streamingParts.length - 1),
    });
  });
  for (const activity of toolActivitiesById.values()) {
    appendActivity(activity);
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
  if (!isStreaming && !streamingParts.length && !activeTool) commitResponseMeta();

  return rows;
}
