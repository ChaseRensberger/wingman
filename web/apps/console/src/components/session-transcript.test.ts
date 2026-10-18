import { expect, test } from "bun:test";
import { createElement, type ComponentProps } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { SessionTranscript } from "@/components/session-transcript";
import { useTranscriptScroll } from "@/hooks/use-transcript-scroll";
import type { PermissionRequest } from "@/lib/types";

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
    streamingReasoning: "",
    isStreaming: true,
    toolCallsById: new Map(),
    toolResultsById: new Map(),
    toolActivitiesById: new Map(),
    modelCallsByMessageId: new Map(),
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
