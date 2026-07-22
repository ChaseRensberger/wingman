import type { Message, Part, ToolActivity, ToolCallPart, ToolPart, ToolResultPart } from "@/lib/types";
import { CircleNotchIcon } from "@phosphor-icons/react";
import { Markdown } from "./markdown";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@wingman/core/components/core/collapsible";

type PatchFile = {
  relativePath: string;
  type: string;
  patch: string;
  additions: number;
  deletions: number;
};

function InlineToolRow({ call, isError = false, isRunning = false }: { call?: ToolCallPart; isError?: boolean; isRunning?: boolean }) {
  const summary = toolSummary(call);
  return (
    <div className={`flex min-w-0 items-center gap-2 py-0.5 text-xs leading-5 ${isError ? "text-destructive" : isRunning ? "text-foreground" : "text-muted-foreground"}`}>
      <span className="w-4 shrink-0 text-center">{toolGlyph(call?.name)}</span>
      {isRunning && <CircleNotchIcon className="size-3 shrink-0 animate-spin" />}
      <span className="truncate">{summary}</span>
    </div>
  );
}

function ToolActivityItem({ call, result, activity }: { call: ToolCallPart; result?: ToolResultPart; activity?: ToolActivity }) {
  const status = activity?.status ?? (result ? (result.is_error ? "error" : "completed") : "pending");
  const displayCall = activity?.input ? { ...call, input: activity.input } : call;
  const displayResult = result ?? (activity && (activity.status === "completed" || activity.status === "error")
    ? { type: "tool_result" as const, call_id: call.call_id, name: call.name, output: activity.output ? [{ type: "text" as const, text: activity.output }] : [], is_error: activity.status === "error", metadata: activity.metadata }
    : undefined);
  const hasDetails = Boolean(displayResult);

  if (!hasDetails) {
    return <InlineToolRow call={displayCall} isError={status === "error"} isRunning={status === "pending" || status === "running"} />;
  }

  return (
    <Collapsible>
      <CollapsibleTrigger className="w-full text-left">
        <InlineToolRow call={displayCall} isError={status === "error"} isRunning={status === "pending" || status === "running"} />
      </CollapsibleTrigger>
      {displayResult && (
        <CollapsibleContent>
          <ToolDetails part={displayResult} call={displayCall} />
        </CollapsibleContent>
      )}
    </Collapsible>
  );
}

function ToolDetails({ part, call }: { part: ToolResultPart; call?: ToolCallPart }) {
  if (call?.name === "apply_patch") return <FileMutationDetails part={part} />;
  if (call?.name === "write" || call?.name === "edit") return <FileMutationDetails part={part} />;
  if (call?.name === "read") return <ReadDetails part={part} />;
  if (call?.name === "bash") return <BashDetails part={part} call={call} />;

  const text = toolText(part);
  return <pre className="ml-5 mt-1 max-h-96 overflow-auto border-l border-border/60 pl-3 text-xs leading-5">{text || JSON.stringify(part.output, null, 2)}</pre>;
}

function BashDetails({ part, call }: { part: ToolResultPart; call: ToolCallPart }) {
  const output = toolText(part);
  const command = typeof call.input.command === "string" ? call.input.command : "";
  return (
    <pre className="ml-5 mt-1 max-h-96 overflow-auto border-l border-border/60 bg-zinc-950 px-3 py-2 text-xs leading-5 text-zinc-100">
      <code>{`$ ${command}${output ? `\n\n${output}` : ""}`}</code>
    </pre>
  );
}

function ReadDetails({ part }: { part: ToolResultPart }) {
  const text = toolText(part);
  const parsed = parseReadOutput(text);
  return (
    <>
      {parsed.path && <div className="ml-5 mt-1 truncate border-l border-border/60 pl-3 text-xs text-muted-foreground">{parsed.path}</div>}
      <pre className="ml-5 mt-1 max-h-96 overflow-auto border-l border-border/60 bg-muted/45 px-3 py-2 text-xs leading-5">
        <code>{parsed.body || text}</code>
      </pre>
    </>
  );
}

function FileMutationDetails({ part }: { part: ToolResultPart }) {
  const files = patchFiles(part.metadata?.files);
  if (files.length === 0) {
    return <pre className="ml-5 mt-1 max-h-96 overflow-auto border-l border-border/60 bg-muted/45 px-3 py-2 text-xs leading-5">{toolText(part)}</pre>;
  }

  return (
    <div className="ml-5 mt-1 space-y-1 border-l border-border/60 pl-3">
      {files.map((file) => (
        <Collapsible key={file.relativePath}>
          <CollapsibleTrigger className="flex w-full items-center justify-between gap-3 py-0.5 text-left text-xs">
            <span className="min-w-0 truncate">{file.relativePath}</span>
            <span className="flex shrink-0 items-center gap-2 font-mono">
              <span className="text-emerald-600">+{file.additions}</span>
              <span className="text-red-600">-{file.deletions}</span>
            </span>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <DiffBlock patch={file.patch} />
          </CollapsibleContent>
        </Collapsible>
      ))}
    </div>
  );
}

function DiffBlock({ patch }: { patch: string }) {
  return (
    <pre className="mt-1 max-h-[32rem] overflow-auto bg-muted/35 px-3 py-2 text-xs leading-5">
      <code>
        {patch.split("\n").map((line, idx) => (
          <div key={idx} className={diffLineClass(line)}>
            {line || " "}
          </div>
        ))}
      </code>
    </pre>
  );
}

export function ChatMessage({ message, isStreaming = false, toolCallsById, toolResultsById, toolActivitiesById }: { message: Message; isStreaming?: boolean; toolCallsById?: Map<string, ToolCallPart>; toolResultsById?: Map<string, ToolResultPart>; toolActivitiesById?: Map<string, ToolActivity> }) {
  if (message.role === "tool") return null;
  const visibleParts = message.content.filter((part) => part.type !== "reasoning");
  if (visibleParts.length === 0) return null;

  const isUser = message.role === "user";
  const isAssistant = message.role === "assistant";

  return (
    <div
      className={`relative border-b border-border/60 py-5 last:border-b-0 ${
        isUser ? "mx-2 bg-primary/[0.03] before:absolute before:inset-y-0 before:-left-2 before:w-0.5 before:bg-primary/35 before:content-[''] after:absolute after:inset-y-0 after:-right-2 after:w-0.5 after:bg-primary/35 after:content-['']" : ""
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
        {visibleParts.map((part, idx) => {
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

function toolText(part: ToolResultPart) {
  return part.output
    .filter((p: Part) => p.type === "text")
    .map((p) => (p as { text: string }).text)
    .join("");
}

function stringInput(call: ToolCallPart, key: string) {
  const value = call.input[key];
  return typeof value === "string" ? value : "";
}

function filename(path: string) {
  return path.split(/[\\/]/).filter(Boolean).at(-1) || path;
}

function toolGlyph(name?: string) {
  if (name === "bash") return "$";
  if (name === "read") return "->";
  if (name === "grep" || name === "glob" || name === "websearch") return "*";
  if (name === "write" || name === "edit" || name === "apply_patch") return "%";
  return "+";
}

function toolSummary(call?: ToolCallPart) {
  if (!call) return "Tool";
  if (call.name === "bash") return stringInput(call, "command") || "Running command";
  if (call.name === "read") return `Read ${filename(stringInput(call, "filePath")) || "file"}`;
  if (call.name === "grep") return `Grep ${quote(stringInput(call, "pattern"))}${inPath(stringInput(call, "path"))}`;
  if (call.name === "glob") return `Glob ${quote(stringInput(call, "pattern"))}${inPath(stringInput(call, "path"))}`;
  if (call.name === "write") return `Write ${filename(stringInput(call, "filePath")) || "file"}`;
  if (call.name === "edit") return `Edit ${filename(stringInput(call, "filePath")) || "file"}`;
  if (call.name === "apply_patch") return "Apply patch";
  if (call.name === "websearch") return `Search ${quote(stringInput(call, "query"))}`;
  if (call.name === "webfetch") return `Fetch ${stringInput(call, "url") || "URL"}`;
  return call.name ? call.name[0]!.toUpperCase() + call.name.slice(1) : "Tool";
}

function quote(value: string) {
  return value ? `"${value}"` : "";
}

function inPath(path: string) {
  return path ? ` in ${path}` : "";
}

function parseReadOutput(text: string) {
  const path = text.match(/<path>([\s\S]*?)<\/path>/)?.[1] ?? "";
  const type = text.match(/<type>([\s\S]*?)<\/type>/)?.[1] ?? "";
  const content = text.match(/<content>\n?([\s\S]*?)\n?<\/content>/)?.[1];
  const entries = text.match(/<entries>\n?([\s\S]*?)\n?<\/entries>/)?.[1];
  return { path, type, body: content ?? entries ?? "" };
}

function patchFiles(raw: unknown): PatchFile[] {
  if (!Array.isArray(raw)) return [];
  return raw.flatMap((item) => {
    if (!item || typeof item !== "object") return [];
    const value = item as Record<string, unknown>;
    if (typeof value.relativePath !== "string" || typeof value.patch !== "string") return [];
    return [{
      relativePath: value.relativePath,
      type: typeof value.type === "string" ? value.type : "update",
      patch: value.patch,
      additions: typeof value.additions === "number" ? value.additions : 0,
      deletions: typeof value.deletions === "number" ? value.deletions : 0,
    }];
  });
}

function diffLineClass(line: string) {
  if (line.startsWith("+++") || line.startsWith("---") || line.startsWith("@@")) return "px-3 font-semibold text-muted-foreground";
  if (line.startsWith("+")) return "bg-emerald-500/12 px-3 text-emerald-700 dark:text-emerald-300";
  if (line.startsWith("-")) return "bg-red-500/12 px-3 text-red-700 dark:text-red-300";
  return "px-3 text-muted-foreground";
}
