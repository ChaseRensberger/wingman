import type { Message, Part, ToolCallPart, ToolResultPart } from "@/lib/types";
import { Markdown } from "./markdown";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/core/collapsible";

function ToolCallCard({ part }: { part: ToolCallPart }) {
  return (
    <Collapsible>
      <CollapsibleTrigger className="text-xs">
        <span className="font-semibold">{part.name}</span>
        <span className="text-muted-foreground">({part.call_id})</span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre className="mt-1 overflow-auto rounded border bg-muted p-2 text-xs">
          {JSON.stringify(part.input, null, 2)}
        </pre>
      </CollapsibleContent>
    </Collapsible>
  );
}

function ToolResultCard({ part }: { part: ToolResultPart }) {
  const text = part.output
    .filter((p: Part) => p.type === "text")
    .map((p) => (p as { text: string }).text)
    .join("");
  return (
    <Collapsible>
      <CollapsibleTrigger className="text-xs">
        <span className="font-semibold">Result</span>
        <span className="text-muted-foreground">({part.call_id})</span>
        {part.is_error && (
          <span className="ml-1 text-destructive">error</span>
        )}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre className="mt-1 overflow-auto rounded border bg-muted p-2 text-xs">
          {text || JSON.stringify(part.output, null, 2)}
        </pre>
      </CollapsibleContent>
    </Collapsible>
  );
}

export function ChatMessage({ message }: { message: Message }) {
  const isUser = message.role === "user";
  const isAssistant = message.role === "assistant";

  return (
    <div className={`flex flex-col gap-1 py-3 ${isUser ? "items-end" : "items-start"}`}>
      <div className="text-xs text-muted-foreground font-medium uppercase tracking-wide">
        {message.role}
      </div>
      <div
        className={`max-w-[90%] rounded-lg border px-3 py-2 ${
          isUser
            ? "bg-primary text-primary-foreground"
            : "bg-card text-card-foreground"
        }`}
      >
        {message.content.map((part, idx) => {
          if (part.type === "text") {
            const textPart = part as { text: string };
            if (isAssistant) {
              return <Markdown key={idx} text={textPart.text} />;
            }
            return (
              <div key={idx} className="whitespace-pre-wrap text-sm">
                {textPart.text}
              </div>
            );
          }
          if (part.type === "tool_call") {
            return (
              <div key={idx} className="mt-1">
                <ToolCallCard part={part as ToolCallPart} />
              </div>
            );
          }
          if (part.type === "tool_result") {
            return (
              <div key={idx} className="mt-1">
                <ToolResultCard part={part as ToolResultPart} />
              </div>
            );
          }
          if (part.type === "reasoning") {
            return (
              <div
                key={idx}
                className="mt-1 rounded border border-dashed p-2 text-xs text-muted-foreground italic"
              >
                {(part as { reasoning: string }).reasoning}
              </div>
            );
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
