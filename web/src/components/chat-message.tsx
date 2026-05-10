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
    <div
      className={`border-b border-border/60 py-5 last:border-b-0 ${
        isUser ? "bg-primary/[0.03]" : message.role === "tool" ? "bg-muted/35" : ""
      }`}
    >
      <div
        className={`min-w-0 border-l-2 px-4 text-sm leading-6 ${
          isUser
            ? "border-primary/35 text-foreground"
            : isAssistant
              ? "border-transparent text-foreground"
              : "border-muted-foreground/25 text-muted-foreground"
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
