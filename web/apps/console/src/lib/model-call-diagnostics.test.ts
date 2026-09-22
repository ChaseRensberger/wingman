import { expect, test } from "bun:test";

import { formatModelCallDiagnostics } from "./model-call-diagnostics";
import type { ModelCall } from "./types";

const call: ModelCall = {
  id: "mcl_test",
  session_id: "ses_test",
  run_id: "run_test",
  step: 1,
  attempt: 2,
  status: "failed",
  model_ref: "openai/luna",
  provider_request_id: "req_test",
  error_message: "stream.Final: openai: provider response failed (HTTP 200)",
  input_tokens: 0,
  output_tokens: 0,
  total_tokens: 0,
  context_tokens: 0,
  started_at: "2026-09-20T12:00:00.000Z",
  completed_at: "2026-09-20T12:00:01.250Z",
};

function snapshot(text: string) {
  return JSON.parse(text.split("```json\n")[1]!.split("\n```")[0]!);
}

test("exports persisted diagnostics with correlation, request shape, and capture limits", () => {
  const text = formatModelCallDiagnostics({
    ...call,
    trace: {
      version: "1",
      model: { provider: "openai", id: "luna", variant: "fast" },
      capabilities: {},
      runtime: { current_date: false },
      messages: { count: 1, by_role: { user: 1 }, part_kinds: { text: 1 } },
      system: { sha256: "hash", bytes: 100 },
      build: { version: "(devel)", revision: "abc123", go: "go1.26" },
      failure: {
        stage: "stream",
        http_status: 200,
        category: "provider",
        retryable: false,
        output_started: false,
        code: "unsupported_model",
        message: "Use the Responses API",
        body: '{"error":{"code":"unsupported_model","message":"Use the Responses API","extra":"native evidence"}}',
        body_kind: "event",
        response_headers: { "x-request-id": ["req_test"] },
        rate_limit: { remaining: { tokens: "10" } },
        transport: {
          kind: "http",
          operation: "read",
          phase: "receive",
          delivery: "accepted",
          recovery: "fail",
        },
        causes: [
          {
            type: "*net.OpError",
            message: "read: connection reset",
            causes: [{ type: "syscall.Errno", message: "connection reset", code: "ECONNRESET" }],
          },
        ],
        redacted: true,
        truncated: true,
        detail_omitted: true,
      },
      retry: { decision: "not_retried", reason: "established_stream" },
      timing: { first_response_ms: 123, first_activity_ms: 200, first_answer_ms: 350 },
    },
  });
  expect(text).toStartWith("Wingman model-call diagnostics\nopenai/luna · failed");
  const data = snapshot(text);
  expect(data.model_call_id).toBe("mcl_test");
  expect(data.run_id).toBe("run_test");
  expect(data.attempt).toBe(2);
  expect(data.provider_request_id).toBe("req_test");
  expect(data.duration_ms).toBe(1250);
  expect(data.failure.code).toBe("unsupported_model");
  expect(data.retry).toEqual({ decision: "not_retried", reason: "established_stream" });
  expect(data.timing.first_answer_ms).toBe(350);
  expect(data.failure.message).toBe("Use the Responses API");
  expect(JSON.parse(data.failure.body).error.extra).toBe("native evidence");
  expect(data.failure.response_headers["x-request-id"]).toEqual(["req_test"]);
  expect(data.failure.rate_limit.remaining.tokens).toBe("10");
  expect(data.failure.transport.delivery).toBe("accepted");
  expect(data.failure.causes[0].causes[0].code).toBe("ECONNRESET");
  expect(data.request.model.variant).toBe("fast");
  expect(data.build.revision).toBe("abc123");
  expect(data.notes.join(" ")).toContain("redactions");
  expect(data.notes.join(" ")).toContain("truncated");
});

test("marks missing historical diagnostics without inventing a cause", () => {
  const data = snapshot(formatModelCallDiagnostics(call));
  expect(data.failure).toBeNull();
  expect(data.request).toBeNull();
  expect(data.build).toBeNull();
  expect(data.notes.join(" ")).toContain("Structured failure details were not captured");
});

test("excludes legacy endpoint credentials and unrelated record fields", () => {
  const value = {
    ...call,
    raw_response: "private response",
    trace: {
      version: "1",
      model: { id: "luna", base_url: "https://user:secret@provider.test/?key=secret" },
      capabilities: {},
      runtime: { current_date: false },
      messages: { count: 0, by_role: {}, part_kinds: {} },
      system: { sha256: "hash", bytes: 0 },
      raw_request: "private prompt",
      failure: {
        stage: "stream",
        category: "invalid_request",
        retryable: false,
        output_started: false,
        code: "context_length_exceeded",
        param: "text.format",
        message: "Provider explanation with quoted input",
        raw_response: "private response",
      },
    },
  };
  const text = formatModelCallDiagnostics(value);
  expect(text).not.toContain("secret");
  expect(text).not.toContain("private");
  const data = snapshot(text);
  expect(data.failure.code).toBe("context_length_exceeded");
  expect(data.failure.param).toBe("text.format");
  expect(data.failure.message).toBe("Provider explanation with quoted input");
  expect(data.failure).not.toHaveProperty("raw_response");
  expect(data.notes.join(" ")).toContain("Provider evidence can contain quoted input or output");
});
