import { expect, test } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { SessionTranscript } from "@/components/session-transcript";
import { useTranscriptScroll } from "@/hooks/use-transcript-scroll";
import type { PermissionRequest } from "@/lib/types";

function Transcript({ request }: { request: PermissionRequest }) {
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
    permissionRequests: [request],
    permissionRepliesInFlight: new Set(),
    onReplyPermission: () => {},
    copiedFailedRunError: false,
    onCopyFailedRunError: () => {},
    onRetry: () => {},
    scroll,
  });
}

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
