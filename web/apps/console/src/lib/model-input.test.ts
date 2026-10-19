import { expect, test } from "bun:test";
import { indexInstructionDiscoveries, modelCallChanges } from "./model-input";
import { projectTranscript } from "./transcript";
import { formatModelCallDiagnostics } from "./model-call-diagnostics";
import type { ModelCall } from "./types";

const original: ModelCall = {
  id: "first",
  session_id: "session",
  assistant_message_id: "one",
  step: 1,
  attempt: 1,
  status: "completed",
  model_ref: "provider/model#high",
  input_tokens: 0,
  output_tokens: 0,
  total_tokens: 0,
  context_tokens: 0,
  trace: {
    version: "1",
    input_hash: "first-input",
    model: {},
    capabilities: {},
    runtime: { current_date: false },
    messages: { count: 2, by_role: {}, part_kinds: {} },
    system: { sha256: "original", bytes: 10 },
    tools: [{ name: "read", schema_hash: "schema", schema_bytes: 5 }],
  },
};

test("compares call-specific settings rather than current session configuration", () => {
  const next = {
    ...original,
    model_ref: "provider/model#low",
    trace: { ...original.trace!, system: { sha256: "updated", bytes: 10 }, tools: [] },
  };
  expect(modelCallChanges(next, original)).toEqual([
    "Model changed",
    "System prompt changed",
    "Available tools changed",
  ]);
  expect(modelCallChanges(original, original)).toEqual([]);
  expect(modelCallChanges(next)).toEqual([]);
  expect(modelCallChanges({ ...original, trace: undefined }, original)).toEqual([]);
});

test("timeline links an interrupted attempt and changed input even for a tool-only response", () => {
  const calls: ModelCall[] = [
    original,
    {
      ...original,
      id: "second",
      assistant_message_id: "two",
      step: 2,
      status: "aborted",
      trace: { ...original.trace!, system: { sha256: "changed", bytes: 10 } },
    },
  ];
  const rows = projectTranscript({
    messages: [
      {
        id: "one",
        role: "assistant",
        content: [{ type: "tool_call", name: "read", call_id: "read1", input: {} }],
      },
      {
        id: "two",
        role: "assistant",
        content: [{ type: "tool_call", name: "read", call_id: "read2", input: {} }],
      },
    ],
    modelCalls: calls,
    toolCallsById: new Map(),
    toolResultsById: new Map(),
    toolActivitiesById: new Map(),
  });
  expect(rows.map((row) => row.type)).toEqual(["exploration", "model-call", "exploration"]);
  const notice = rows[1];
  if (notice?.type !== "model-call") throw new Error("Missing call notice");
  expect(notice.call.id).toBe("second");
  expect(notice.notices).toEqual(["System prompt changed", "Model call interrupted"]);
});

test("diagnostic copies do not expand recorded input", () => {
  expect(formatModelCallDiagnostics(original)).not.toContain("first-input");
});

test("instruction notices compare recorded source hashes", () => {
  const source = {
    kind: "directory",
    path: "/repo/src/AGENTS.md",
    sha256: "old",
    tool_use_id: "read1",
    requested_path: "src/index.ts",
    resolved_path: "/repo/src/index.ts",
  };
  const before = { ...original, trace: { ...original.trace!, instructions: [source] } };
  const after = {
    ...before,
    trace: { ...before.trace, instructions: [{ ...source, sha256: "new" }] },
  };
  expect(modelCallChanges(after, before)).toEqual(["Instructions updated"]);
  expect(
    modelCallChanges({ ...before, trace: { ...before.trace, instructions: [] } }, before),
  ).toEqual(["Instructions updated"]);
});

test("discovery index keeps the first affected call and isolates tool-use IDs", () => {
  const source = {
    kind: "directory",
    path: "/repo/src/AGENTS.md",
    sha256: "old",
    tool_use_id: "read1",
    call_id: "provider-read",
    requested_path: "src/index.ts",
    resolved_path: "/repo/src/index.ts",
  };
  const first = { ...original, trace: { ...original.trace!, instructions: [source] } };
  const later = {
    ...original,
    id: "later",
    trace: {
      ...original.trace!,
      instructions: [
        { ...source, sha256: "new" },
        { ...source, tool_use_id: "read2" },
        { ...source, tool_use_id: undefined, path: "/repo/legacy/AGENTS.md" },
        { ...source, tool_use_id: undefined, call_id: undefined, path: "/repo/AGENTS.md" },
      ],
    },
  };
  const index = indexInstructionDiscoveries([first, later, { ...original, trace: undefined }]);
  expect([...index.keys()]).toEqual(["tool:read1", "tool:read2", "call:provider-read"]);
  expect([...index.get("tool:read1")!.values()]).toEqual([
    {
      id: "first",
      path: source.path,
      requested: source.requested_path,
      resolved: source.resolved_path,
      order: 0,
    },
  ]);
  expect(index.get("tool:read2")!.get(source.path)?.id).toBe("later");
  expect([...index.get("call:provider-read")!.keys()]).toEqual(["/repo/legacy/AGENTS.md"]);
});
