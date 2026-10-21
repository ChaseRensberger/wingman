import { expect, test } from "bun:test";
import { createElement, type ComponentProps } from "react";
import { renderToReadableStream, renderToStaticMarkup } from "react-dom/server";

import { SessionTranscript } from "@/components/session-transcript";
import { useTranscriptScroll } from "@/hooks/use-transcript-scroll";
import type { Message, PermissionRequest } from "@/lib/types";

function Transcript({
  request,
  props,
}: {
  request?: PermissionRequest;
  props?: Partial<ComponentProps<typeof SessionTranscript>>;
}) {
  const scroll = useTranscriptScroll(null, null);
  return createElement(SessionTranscript, {
    messages: [],
    rawMessages: [],
    jsonMode: false,
    greeting: "",
    streamingText: "",
    streamingParts: [],
    isReasoningStreaming: false,
    isStreaming: true,
    toolCallsById: new Map(),
    toolResultsById: new Map(),
    toolActivitiesById: new Map(),
    modelCallsByMessageId: new Map(),
    modelCalls: [],
    onInspectCall: () => {},
    agentNames: new Map(),
    failedRun: null,
    permissionRequests: request ? [request] : [],
    permissionRepliesInFlight: new Set(),
    onReplyPermission: () => {},
    copiedFailedRunError: false,
    onCopyFailedRunError: () => {},
    onRetry: () => {},
    ...props,
    scroll,
  });
}

test("transcript groups saved and live exploration without hiding approval cards", () => {
  const markup = renderToStaticMarkup(
    createElement(Transcript, {
      props: {
        modelCalls: [
          {
            id: "mcl_after_read",
            session_id: "session-1",
            step: 2,
            attempt: 1,
            status: "completed",
            input_tokens: 0,
            output_tokens: 0,
            total_tokens: 0,
            context_tokens: 0,
            trace: {
              version: "1",
              model: {},
              capabilities: {},
              runtime: { current_date: false },
              system: { sha256: "system", bytes: 10 },
              messages: { count: 2, by_role: {}, part_kinds: {} },
              instructions: [
                {
                  kind: "directory",
                  path: "/project/src/AGENTS.md",
                  sha256: "guide",
                  tool_use_id: "read-1",
                  requested_path: "src/index.ts",
                  resolved_path: "/project/src/index.ts",
                },
              ],
            },
          },
        ],
        messages: [
          {
            id: "message-1",
            role: "assistant",
            content: [
              {
                type: "tool",
                tool_use_id: "read-1",
                call_id: "call-1",
                name: "read",
                state: "completed",
                input: { filePath: "src/index.ts" },
              },
            ],
          },
        ],
        toolActivitiesById: new Map([
          [
            "grep-1",
            {
              tool_use_id: "grep-1",
              call_id: "call-2",
              tool: "grep",
              status: "running",
              input: { pattern: "test" },
            },
          ],
        ]),
      },
      request: {
        id: "request-1",
        session_id: "session-1",
        tool_use_id: "grep-1",
        call_id: "call-2",
        action: "read",
        resources: ["/project/*"],
        status: "pending",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    }),
  );
  expect(markup.match(/data-component="exploration-group"/g)).toHaveLength(1);
  expect(markup).toContain("Exploring: 1 read, 1 search");
  expect(markup).toContain("Grep &quot;test&quot;");
  expect(markup).toContain("Permission requested");
  expect(markup).toContain("Allow once</button>");
  expect(markup).toContain("Directory instructions discovered");
  expect(markup).toContain("/project/src/AGENTS.md");
  expect(markup).toContain("View instructions in first affected call");
});

test.each(["read", "bash", "apply_patch", "custom"])(
  "transcript owns consistent gaps around thoughts and %s tools",
  async (name) => {
    const messages: Message[] = [
      { id: "user", role: "user", content: [{ type: "text", text: "Start" }] },
      { id: "thought", role: "assistant", content: [{ type: "reasoning", reasoning: "Checking" }] },
      {
        id: "tool",
        role: "assistant",
        content: [{ type: "tool", call_id: "call", name, state: "completed", input: {} }],
      },
      { id: "answer", role: "assistant", content: [{ type: "text", text: "Answer" }] },
      { id: "next-user", role: "user", content: [{ type: "text", text: "Continue" }] },
      {
        id: "next-thought",
        role: "assistant",
        content: [{ type: "reasoning", reasoning: "Next" }],
      },
    ];
    const stream = await renderToReadableStream(
      createElement(Transcript, { props: { messages, isStreaming: false } }),
    );
    await stream.allReady;
    const markup = await new Response(stream).text();
    expect(markup).toContain("<p>Answer</p>");
    const rowClasses = [
      ...markup.matchAll(/<div class="([^"]*)" data-slot="transcript-row">/g),
    ].map((match) => match[1]);
    const layoutClasses = markup
      .match(/class="([^"]*)" data-slot="transcript-rows"/)?.[1]
      .split(" ");
    expect(layoutClasses).toEqual(expect.arrayContaining(["flex", "flex-col", "gap-2", "pb-5"]));
    expect(rowClasses).toHaveLength(6);
    expect(rowClasses.map((classes) => classes.includes("mt-4"))).toEqual([
      false,
      true,
      false,
      false,
      true,
      true,
    ]);
    expect(markup).not.toMatch(/class="relative[^"]*p[bt]-5/);
    expect(markup).not.toMatch(/class="(?:[^"]*\s)?my-(?:1\.5|2)(?:\s|")/);
  },
);

test("mixed thought, tool, and text parts use the same internal gap", () => {
  const markup = renderToStaticMarkup(
    createElement(Transcript, {
      props: {
        messages: [
          {
            role: "assistant",
            content: [
              { type: "reasoning", reasoning: "Checking" },
              { type: "tool", call_id: "call", name: "bash", state: "completed", input: {} },
              { type: "text", text: "Answer" },
            ],
          },
        ],
        isStreaming: false,
      },
    }),
  );
  expect(markup).toContain("group/message flex min-w-0 flex-col gap-2");
  expect(markup.match(/data-slot="transcript-row"/g)).toHaveLength(1);
  expect(markup).not.toMatch(/class="(?:[^"]*\s)?my-2(?:\s|")/);
});

test("running compaction replaces the generic responding indicator", () => {
  const markup = renderToStaticMarkup(
    createElement(Transcript, {
      props: {
        messages: [
          {
            id: "compaction",
            role: "user",
            state: "in_progress",
            content: [{ type: "compaction_marker" }],
          },
        ],
      },
    }),
  );
  expect(markup).toContain('aria-label="Compaction running"');
  expect(markup).not.toContain('aria-label="Responding"');
  expect(markup.match(/data-hexwave=""/g)).toHaveLength(1);
});

test("directory approval shows its action, scope, and session lifetime", () => {
  const markup = renderToStaticMarkup(
    createElement(Transcript, {
      request: {
        id: "prq_directory",
        session_id: "ses_directory",
        action: "read",
        resources: ["/projects/reference/*"],
        status: "pending",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    }),
  );
  expect(markup).toContain("/projects/reference/*");
  expect(markup).toContain(">read</span>");
  expect(markup).toContain("Allow once permits only this call");
  expect(markup).toContain("including after a server restart");
  expect(markup).toContain("includes its files and subdirectories");
  expect(markup).toContain("Read approval does not permit changes");
  expect(markup).toContain("Allow once</button>");
  expect(markup).toContain("Always allow</button>");
  expect(markup).toContain("Reject</button>");
});
