import type { ModelCall } from "./types";

export function formatModelCallDiagnostics(call: ModelCall): string {
  const trace = call.trace;
  const failure = trace?.failure;
  const duration =
    call.started_at && call.completed_at
      ? Date.parse(call.completed_at) - Date.parse(call.started_at)
      : undefined;
  const snapshot = {
    version: 1,
    build: trace?.build ?? null,
    session_id: call.session_id,
    run_id: call.run_id,
    model_call_id: call.id,
    assistant_message_id: call.assistant_message_id,
    agent_id: call.agent_id,
    step: call.step,
    attempt: call.attempt,
    status: call.status,
    model_ref: call.model_ref,
    provider: call.provider,
    model_id: call.model_id,
    api: call.api,
    provider_request_id: call.provider_request_id,
    started_at: call.started_at,
    completed_at: call.completed_at,
    duration_ms: duration !== undefined && Number.isFinite(duration) ? duration : undefined,
    error: call.error_message,
    error_type: call.error_type,
    retry: trace?.retry,
    timing: trace?.timing,
    failure: failure
      ? {
          stage: failure.stage,
          route: failure.route,
          protocol: failure.protocol,
          endpoint: failure.endpoint,
          model: failure.model,
          settings: failure.settings,
          http_status: failure.http_status,
          request_id: failure.request_id,
          category: failure.category,
          classification: failure.classification,
          retryable: failure.retryable,
          retry_after_ms: failure.retry_after_ms,
          event: failure.event,
          code: failure.code,
          type: failure.type,
          param: failure.param,
          message: failure.message,
          body: failure.body,
          body_kind: failure.body_kind,
          response_headers: failure.response_headers,
          rate_limit: failure.rate_limit,
          transport: failure.transport,
          causes: failure.causes,
          output_started: failure.output_started,
          redacted: failure.redacted,
          truncated: failure.truncated,
          detail_omitted: failure.detail_omitted,
        }
      : null,
    finish_reason: call.finish_reason,
    usage: {
      input_tokens: call.input_tokens,
      output_tokens: call.output_tokens,
      reasoning_tokens: call.reasoning_tokens,
      cached_input_tokens: call.cached_input_tokens,
      cache_write_tokens: call.cache_write_tokens,
      total_tokens: call.total_tokens,
    },
    request: trace
      ? {
          model: {
            provider: trace.model.provider,
            id: trace.model.id,
            variant: trace.model.variant,
            api: trace.model.api,
          },
          capabilities: trace.capabilities,
          runtime: trace.runtime,
          messages: trace.messages,
          tools: trace.tools,
          system: trace.system,
          lowered: trace.lowered,
        }
      : null,
    notes: [
      "Provider messages, failure bodies, response headers, and causes are included when captured. Known credentials are redacted. Provider evidence can contain quoted input or output.",
      ...(!trace ? ["Request trace was not captured for this call."] : []),
      ...(!failure && call.error_message
        ? ["Structured failure details were not captured for this call."]
        : []),
      ...(failure?.redacted ? ["Provider diagnostic fields contain redactions."] : []),
      ...(failure?.truncated ? ["Provider diagnostic details were truncated."] : []),
      ...(failure?.detail_omitted
        ? [
            "Some failure details were not captured. Older records can also have omitted provider text.",
          ]
        : []),
    ],
  };
  return [
    "Wingman model-call diagnostics",
    `${call.model_ref || call.model_id || "Unknown model"} · ${call.status} · step ${call.step}, attempt ${call.attempt}`,
    ...(call.error_message ? [`Error: ${call.error_message}`] : []),
    "",
    "```json",
    JSON.stringify(snapshot, null, 2),
    "```",
  ].join("\n");
}
