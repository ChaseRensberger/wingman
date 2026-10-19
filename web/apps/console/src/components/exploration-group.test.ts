import { expect, test } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { ExplorationGroup } from "@/components/exploration-group";
import { ToolActivityItem } from "@/components/tool-activity";
import { toolPartProps, type TranscriptTool } from "@/lib/transcript";
import type { PermissionRequest, ToolPart } from "@/lib/types";

function item(overrides: Partial<ToolPart> = {}): TranscriptTool {
  return {
    key: "tool-1",
    ...toolPartProps({
      type: "tool",
      tool_use_id: "tool-1",
      call_id: "call-1",
      name: "read",
      state: "completed",
      input: { filePath: "src/index.ts", offset: 20, limit: 40 },
      ...overrides,
    }),
  };
}

const permission: PermissionRequest = {
  id: "permission-1",
  session_id: "session-1",
  tool_use_id: "tool-1",
  call_id: "call-1",
  action: "read",
  resources: ["/project/*"],
  status: "pending",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

test("exploration starts collapsed with an accessible summary button", () => {
  const markup = renderToStaticMarkup(
    createElement(ExplorationGroup, { tools: [item()], permissionRequests: [] }),
  );
  expect(markup).toContain("Explored: 1 read");
  expect(markup).toContain('aria-expanded="false"');
  expect(markup).not.toContain("Read src/index.ts");
  expect(markup).not.toContain("hover:bg-");
  expect(markup.match(/<svg\b/g)).toHaveLength(1);
  expect(markup).toContain("group-hover/exploration:text-foreground");
});

test("pending exploration has an active label", () => {
  const markup = renderToStaticMarkup(
    createElement(ExplorationGroup, {
      tools: [item({ state: "running" })],
      permissionRequests: [],
    }),
  );
  expect(markup).toContain("Exploring: 1 read");
  expect(markup).toContain('data-hexwave=""');
});

test("permission-blocked calls remain visible under a collapsed group", () => {
  const markup = renderToStaticMarkup(
    createElement(ExplorationGroup, {
      tools: [item({ state: "pending" })],
      permissionRequests: [permission],
    }),
  );
  expect(markup).toContain('aria-expanded="false"');
  expect(markup).toContain("Read src/index.ts:20-59");
});

test("permission matching uses tool-use identity rather than a reused call ID", () => {
  const markup = renderToStaticMarkup(
    createElement(ExplorationGroup, {
      tools: [item()],
      permissionRequests: [{ ...permission, tool_use_id: "another-tool" }],
    }),
  );
  expect(markup).not.toContain("Read src/index.ts");
});

test("legacy permission calls are matched by call ID", () => {
  const markup = renderToStaticMarkup(
    createElement(ExplorationGroup, {
      tools: [item({ tool_use_id: undefined })],
      permissionRequests: [{ ...permission, tool_use_id: undefined }],
    }),
  );
  expect(markup).toContain("Read src/index.ts:20-59");
});

test("errors remain visible when exploration is collapsed", () => {
  const markup = renderToStaticMarkup(
    createElement(ExplorationGroup, {
      tools: [item({ state: "error", error: "Cannot read file" })],
      permissionRequests: [],
    }),
  );
  expect(markup).toContain("Read src/index.ts:20-59");
  expect(markup).toContain("Cannot read file");
});

test("image results remain visible when exploration is collapsed", () => {
  const markup = renderToStaticMarkup(
    createElement(ExplorationGroup, {
      tools: [item({ output_parts: [{ type: "image", url: "https://example.com/result.png" }] })],
      permissionRequests: [],
    }),
  );
  expect(markup).toContain('src="https://example.com/result.png"');
  expect(markup).not.toContain("Read src/index.ts");
});

test("compact search rows include the pattern, path, and match count", () => {
  const markup = renderToStaticMarkup(
    createElement(ToolActivityItem, {
      ...item({
        name: "grep",
        input: { pattern: "keydown|useHotkey", path: "web/apps/console/src" },
        metadata: { matches: 20 },
      }),
    }),
  );
  expect(markup).toContain(
    "Grep &quot;keydown|useHotkey&quot; in web/apps/console/src (20 matches)",
  );
  expect(markup).toContain("✱");
});
