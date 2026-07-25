import type { Message, ToolActivity, ToolCallPart, ToolPart, ToolResultPart } from "@/lib/types";
import { ReasoningPart } from "@/components/reasoning-part";
import { ToolActivityItem } from "@/components/tool-activity";
import { Markdown } from "./markdown";

export function ChatMessage({ message, isStreaming = false, toolCallsById, toolResultsById, toolActivitiesById }: { message: Message; isStreaming?: boolean; toolCallsById?: Map<string, ToolCallPart>; toolResultsById?: Map<string, ToolResultPart>; toolActivitiesById?: Map<string, ToolActivity> }) {
  if (message.role === "tool") return null;
  if (message.content.length === 0) return null;

  const isUser = message.role === "user";
  const isAssistant = message.role === "assistant";

  return (
    <div
      className={`relative py-5 ${
        isUser
          ? "mx-2 bg-primary/[0.03] before:absolute before:inset-y-0 before:-left-2 before:w-0.5 before:bg-primary/35 before:content-[''] after:absolute after:inset-y-0 after:-right-2 after:w-0.5 after:bg-primary/35 after:content-['']"
          : ""
      }`}
    >
      <div
        className={`min-w-0 ${isUser ? "px-2" : "px-4"} text-sm leading-6 ${
          isUser
            ? "text-foreground"
            : isAssistant
              ? "text-foreground"
              : "text-muted-foreground"
        }`}
      >
        {message.content.map((part, idx) => {
          if (part.type === "text") {
            const textPart = part as { text: string };
            if (isAssistant) {
              return <Markdown key={idx} text={textPart.text} isStreaming={isStreaming} />;
            }
            return (
              <div key={idx} className="whitespace-pre-wrap text-sm">
                {textPart.text}
              </div>
            );
          }
          if (part.type === "reasoning") {
            const reasoningPart = part as { reasoning: string };
            return <ReasoningPart key={idx} reasoning={reasoningPart.reasoning} />;
          }
          if (part.type === "tool_call") {
            const call = part as ToolCallPart;
            return (
              <div key={idx} className="mt-1">
                <ToolActivityItem call={call} result={toolResultsById?.get(call.call_id)} activity={toolActivitiesById?.get(call.call_id)} />
              </div>
            );
          }
          if (part.type === "tool") {
            const tool = part as ToolPart;
            const call = { type: "tool_call" as const, call_id: tool.call_id, name: tool.name, input: tool.input };
            const result = tool.state === "completed" || tool.state === "error"
              ? { type: "tool_result" as const, call_id: tool.call_id, name: tool.name, output: tool.output ? [{ type: "text" as const, text: tool.output }] : [], is_error: tool.state === "error", metadata: tool.metadata }
              : undefined;
            return <div key={idx} className="mt-1"><ToolActivityItem call={call} result={result} activity={{ call_id: tool.call_id, tool: tool.name, status: tool.state, input: tool.input, output: tool.output, metadata: tool.metadata, error: tool.error }} /></div>;
          }
          if (part.type === "tool_result") {
            const result = part as ToolResultPart;
            if (toolCallsById?.has(result.call_id)) return null;
            const call = { type: "tool_call" as const, call_id: result.call_id, name: result.name || "tool", input: {} };
            return <div key={idx} className="mt-1"><ToolActivityItem call={call} result={result} activity={toolActivitiesById?.get(result.call_id)} /></div>;
          }
          return (
            <div key={idx} className="text-xs text-muted-foreground">
              [unknown part: {part.type}]
            </div>
          );
        })}
      </div>
    </div>
  );
}
