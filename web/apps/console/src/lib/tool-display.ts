import type { Part, ToolCallPart, ToolResultPart } from "@/lib/types";

export type PatchFile = {
  filePath?: string;
  relativePath: string;
  movePath?: string;
  type: string;
  patch: string;
  additions: number;
  deletions: number;
};

export type DiffLine = {
  kind: "header" | "hunk" | "context" | "addition" | "deletion" | "note";
  text: string;
  oldLine?: number;
  newLine?: number;
};

export function toolText(part?: ToolResultPart): string {
  if (!part) return "";
  return part.output
    .filter((item: Part) => item.type === "text")
    .map((item) => (item as { text: string }).text)
    .join("");
}

export function stripAnsi(value: string): string {
  return value.replace(
    // oxlint-disable-next-line no-control-regex -- ANSI escape sequences are control characters by definition.
    /[\u001B\u009B][[\]()#;?]*(?:(?:(?:;[-a-zA-Z\d/#&.:=?%@~_]+)*|[a-zA-Z\d]+(?:;[-a-zA-Z\d/#&.:=?%@~_]*)?)?\u0007|(?:(?:\d{1,4}(?:[;:]\d{0,4})*)?[\dA-PR-TZcf-nq-uy=><~]))/g,
    "",
  );
}

export function collapseOutput(output: string, maxLines: number, maxChars: number) {
  const lines = output.split("\n");
  if (lines.length <= maxLines && Array.from(output).length <= maxChars) {
    return { output, overflow: false };
  }
  const preview = lines.slice(0, maxLines).join("\n");
  if (Array.from(preview).length > maxChars) {
    return {
      output: `${Array.from(preview)
        .slice(0, Math.max(0, maxChars - 1))
        .join("")}…`,
      overflow: true,
    };
  }
  return { output: `${lines.slice(0, maxLines).join("\n")}\n…`, overflow: true };
}

export function stringInput(call: ToolCallPart, key: string) {
  const value = call.input[key];
  return typeof value === "string" ? value : "";
}

export function filename(path: string) {
  return path.split(/[\\/]/).filter(Boolean).at(-1) || path;
}

function quote(value: string) {
  return value ? `"${value}"` : "";
}

function inPath(path: string) {
  return path ? ` in ${path}` : "";
}

export function toolSummary(call: ToolCallPart) {
  if (call.name === "bash") return stringInput(call, "command") || "Running command";
  if (call.name === "read") return `Read ${filename(stringInput(call, "filePath")) || "file"}`;
  if (call.name === "grep")
    return `Grep ${quote(stringInput(call, "pattern"))}${inPath(stringInput(call, "path"))}`;
  if (call.name === "glob")
    return `Glob ${quote(stringInput(call, "pattern"))}${inPath(stringInput(call, "path"))}`;
  if (call.name === "write") return `Write ${filename(stringInput(call, "filePath")) || "file"}`;
  if (call.name === "edit") return `Edit ${filename(stringInput(call, "filePath")) || "file"}`;
  if (call.name === "apply_patch") return "Apply patch";
  if (call.name === "websearch") return `Search ${quote(stringInput(call, "query"))}`;
  if (call.name === "webfetch") return `Fetch ${stringInput(call, "url") || "URL"}`;
  return humanizeToolName(call.name);
}

export function humanizeToolName(name: string) {
  return name.replace(/[_-]+/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase()) || "Tool";
}

export function compactInput(input: Record<string, unknown>): string {
  const entries = Object.entries(input);
  if (entries.length === 0) return "";
  return entries
    .slice(0, 3)
    .map(([key, value]) => {
      const rendered = typeof value === "string" ? value : JSON.stringify(value);
      const short = rendered.length > 72 ? `${rendered.slice(0, 71)}…` : rendered;
      return `${key}=${short}`;
    })
    .join(" · ");
}

export function formatDuration(durationMs?: number): string {
  if (durationMs === undefined) return "";
  if (durationMs < 1000) return `${durationMs}ms`;
  if (durationMs < 60_000) return `${(durationMs / 1000).toFixed(durationMs < 10_000 ? 1 : 0)}s`;
  const minutes = Math.floor(durationMs / 60_000);
  const seconds = Math.floor((durationMs % 60_000) / 1000);
  return `${minutes}m ${seconds}s`;
}

export function parseReadOutput(text: string) {
  const path = text.match(/<path>([\s\S]*?)<\/path>/)?.[1] ?? "";
  const type = text.match(/<type>([\s\S]*?)<\/type>/)?.[1] ?? "";
  const content = text.match(/<content>\n?([\s\S]*?)\n?<\/content>/)?.[1];
  const entries = text.match(/<entries>\n?([\s\S]*?)\n?<\/entries>/)?.[1];
  return { path, type, body: content ?? entries ?? "" };
}

export function patchFiles(raw: unknown): PatchFile[] {
  if (!Array.isArray(raw)) return [];
  return raw.flatMap((item) => {
    if (!item || typeof item !== "object") return [];
    const value = item as Record<string, unknown>;
    if (typeof value.relativePath !== "string" || typeof value.patch !== "string") return [];
    return [
      {
        filePath: typeof value.filePath === "string" ? value.filePath : undefined,
        relativePath: value.relativePath,
        movePath: typeof value.movePath === "string" && value.movePath ? value.movePath : undefined,
        type: typeof value.type === "string" ? value.type : "update",
        patch: value.patch,
        additions: typeof value.additions === "number" ? value.additions : 0,
        deletions: typeof value.deletions === "number" ? value.deletions : 0,
      },
    ];
  });
}

export function parseUnifiedDiff(patch: string): DiffLine[] {
  let oldLine = 0;
  let newLine = 0;
  let inHunk = false;
  return patch.split("\n").flatMap((line, index, lines): DiffLine[] => {
    if (index === lines.length - 1 && line === "") return [];
    if (line.startsWith("@@")) {
      inHunk = true;
      const match = line.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
      oldLine = Number(match?.[1] ?? 0);
      newLine = Number(match?.[2] ?? 0);
      return [{ kind: "hunk", text: line }];
    }
    if (!inHunk && (line.startsWith("---") || line.startsWith("+++")))
      return [{ kind: "header", text: line }];
    if (line.startsWith("\\")) return [{ kind: "note", text: line }];
    if (line.startsWith("+"))
      return [{ kind: "addition", text: line.slice(1), newLine: newLine++ }];
    if (line.startsWith("-"))
      return [{ kind: "deletion", text: line.slice(1), oldLine: oldLine++ }];
    const result = {
      kind: "context" as const,
      text: line.startsWith(" ") ? line.slice(1) : line,
      oldLine,
      newLine,
    };
    oldLine++;
    newLine++;
    return [result];
  });
}
