import type { Message, Part, ToolCallPart, ToolResultPart } from "@/lib/types";
import { CheckIcon } from "@phosphor-icons/react";
import { Markdown } from "./markdown";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@wingman/core/components/core/collapsible";

type PatchFile = {
  relativePath: string;
  type: string;
  patch: string;
  additions: number;
  deletions: number;
};

function ToolCallCard({ part }: { part: ToolCallPart }) {
  return (
    <Collapsible>
      <CollapsibleTrigger className="text-xs text-muted-foreground">
        <span className="font-semibold text-foreground">Calling {part.name}</span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre className="mt-1 overflow-auto rounded border bg-muted p-2 text-xs">
          {JSON.stringify(part.input, null, 2)}
        </pre>
      </CollapsibleContent>
    </Collapsible>
  );
}

function ToolResultCard({ part, call }: { part: ToolResultPart; call?: ToolCallPart }) {
  if (call?.name === "apply_patch") return <ApplyPatchResult part={part} />;
  if (call?.name === "write" || call?.name === "edit") return <FileMutationResult part={part} title={call.name} />;
  if (call?.name === "read") return <ReadResult part={part} call={call} />;
  if (call?.name === "bash") return <BashResult part={part} call={call} />;

  const text = toolText(part);
  return (
    <Collapsible>
      <CollapsibleTrigger className="flex w-full items-center justify-between gap-3 rounded-lg border bg-card px-3 py-2 text-xs">
        <span className="min-w-0 truncate font-medium">{call?.name ?? "Tool"}</span>
        <ToolStatus isError={part.is_error} />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre className="mt-1 overflow-auto rounded border bg-muted p-2 text-xs">
          {text || JSON.stringify(part.output, null, 2)}
        </pre>
      </CollapsibleContent>
    </Collapsible>
  );
}

function BashResult({ part, call }: { part: ToolResultPart; call: ToolCallPart }) {
  const output = toolText(part);
  const command = typeof call.input.command === "string" ? call.input.command : "";
  return (
    <Collapsible>
      <CollapsibleTrigger className="flex w-full items-center justify-between gap-3 rounded-lg border bg-card px-3 py-2 text-xs">
        <span className="min-w-0 truncate font-medium" title={command || undefined}>{command || "Shell"}</span>
        <ToolStatus isError={part.is_error} />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre className="mt-2 max-h-96 overflow-auto rounded-lg border bg-zinc-950 p-3 text-xs leading-5 text-zinc-100">
          <code>{`$ ${command}${output ? `\n\n${output}` : ""}`}</code>
        </pre>
      </CollapsibleContent>
    </Collapsible>
  );
}

function ReadResult({ part, call }: { part: ToolResultPart; call: ToolCallPart }) {
  const text = toolText(part);
  const parsed = parseReadOutput(text);
  const path = parsed.path || stringInput(call, "filePath") || "read";
  return (
    <Collapsible>
      <CollapsibleTrigger className="flex w-full items-center justify-between gap-3 text-xs">
        <span className="min-w-0 truncate font-semibold">Read {filename(path)}</span>
        <span className="shrink-0 text-muted-foreground">{parsed.type || "file"}</span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        {parsed.path && <div className="mt-1 truncate text-xs text-muted-foreground">{parsed.path}</div>}
        <pre className="mt-2 max-h-96 overflow-auto rounded-lg border bg-muted/45 p-3 text-xs leading-5">
          <code>{parsed.body || text}</code>
        </pre>
      </CollapsibleContent>
    </Collapsible>
  );
}

function ApplyPatchResult({ part }: { part: ToolResultPart }) {
  return <FileMutationResult part={part} title="Patch" />;
}

function FileMutationResult({ part, title }: { part: ToolResultPart; title: string }) {
  const files = patchFiles(part.metadata?.files);
  const summary = files.length > 0 ? `${capitalize(title)} ${formatFileSummary(files)}` : capitalize(title);
  if (files.length === 0) {
    return (
      <Collapsible>
        <CollapsibleTrigger className="flex w-full items-center justify-between gap-3 rounded-lg border bg-card px-3 py-2 text-xs">
          <span className="min-w-0 truncate font-medium">{summary}</span>
          <ToolStatus isError={part.is_error} />
        </CollapsibleTrigger>
        <CollapsibleContent>
          <pre className="mt-1 overflow-auto rounded border bg-muted p-2 text-xs">{toolText(part)}</pre>
        </CollapsibleContent>
      </Collapsible>
    );
  }

  return (
    <Collapsible>
      <CollapsibleTrigger className="flex w-full items-center justify-between gap-3 rounded-lg border bg-card px-3 py-2 text-xs">
        <span className="min-w-0 truncate font-medium">{summary}</span>
        <span className="flex shrink-0 items-center gap-3">
          <span className="flex items-center gap-2 font-mono">
            <span className="text-emerald-600">+{sumPatchField(files, "additions")}</span>
            <span className="text-red-600">-{sumPatchField(files, "deletions")}</span>
          </span>
          <ToolStatus isError={part.is_error} />
        </span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-2 space-y-2">
          {files.map((file) => (
            <Collapsible key={file.relativePath}>
              <CollapsibleTrigger className="flex w-full items-center justify-between gap-3 rounded-lg border bg-card px-3 py-2 text-xs">
                <span className="min-w-0 truncate font-medium">{file.relativePath}</span>
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
      </CollapsibleContent>
    </Collapsible>
  );
}

function DiffBlock({ patch }: { patch: string }) {
  return (
    <pre className="mt-2 max-h-[32rem] overflow-auto rounded-lg border bg-muted/35 p-0 text-xs leading-5">
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

export function ChatMessage({ message, isStreaming = false, toolCallsById, toolResultsById }: { message: Message; isStreaming?: boolean; toolCallsById?: Map<string, ToolCallPart>; toolResultsById?: Map<string, true> }) {
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
              return <Markdown key={idx} text={textPart.text} isStreaming={isStreaming} />;
            }
            return (
              <div key={idx} className="whitespace-pre-wrap text-sm">
                {textPart.text}
              </div>
            );
          }
          if (part.type === "tool_call") {
            if (toolResultsById?.has((part as ToolCallPart).call_id)) return null;
            return (
              <div key={idx} className="mt-1">
                <ToolCallCard part={part as ToolCallPart} />
              </div>
            );
          }
          if (part.type === "tool_result") {
            const result = part as ToolResultPart;
            return (
              <div key={idx} className="mt-1">
                <ToolResultCard part={result} call={toolCallsById?.get(result.call_id)} />
              </div>
            );
          }
          if (part.type === "reasoning") {
            return (
              <div key={idx} className="mt-1 rounded border border-dashed p-2 text-xs text-muted-foreground italic">
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

function capitalize(value: string) {
  return value ? value[0]!.toUpperCase() + value.slice(1) : value;
}

function ToolStatus({ isError }: { isError?: boolean }) {
  if (isError) return <span className="shrink-0 text-destructive">error</span>;
  return <CheckIcon className="size-4 shrink-0 text-muted-foreground" aria-label="done" />;
}

function formatFileSummary(files: PatchFile[]) {
  if (files.length === 1) return files[0]!.relativePath;
  return `${files.length} files`;
}

function sumPatchField(files: PatchFile[], field: "additions" | "deletions") {
  return files.reduce((total, file) => total + file[field], 0);
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
