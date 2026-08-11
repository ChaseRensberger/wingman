import createClient, { type ClientOptions } from "openapi-fetch";

import type { components, paths } from "./schema";

export type { components, operations, paths } from "./schema";

export type ErrorResponse = components["schemas"]["ErrorResponse"];
export type SessionEvent = components["schemas"]["SessionEvent"];
export type RunStreamEvent = components["schemas"]["RunStreamEvent"];

export class APIError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId?: string;
  readonly details: Array<{ field: string; reason: string }>;

  constructor(
    status: number,
    code: string,
    message: string,
    requestId?: string,
    details: Array<{ field: string; reason: string }> = [],
  ) {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
    this.details = details;
  }
}

export async function apiErrorFromResponse(response: Response): Promise<APIError> {
  const text = await response.text();
  let body: ErrorResponse | undefined;
  try {
    body = JSON.parse(text) as ErrorResponse;
  } catch {
    // Proxies and older daemons may not return Wingman's error envelope.
  }
  const error = body?.error;
  return new APIError(
    response.status,
    error?.code ?? "request_failed",
    error?.message ?? (text || `HTTP ${response.status}`),
    error?.request_id ?? response.headers.get("X-Request-ID") ?? undefined,
    error?.details ?? [],
  );
}

export type APIResult<T> = {
  data?: T;
  error?: unknown;
  response: Response;
};

export async function apiData<T>(request: Promise<APIResult<T>>): Promise<T> {
  const { data, error, response } = await request;
  if (!error && response.ok) return data as T;
  const detail = error && typeof error === "object" && "error" in error
    ? (error as ErrorResponse).error
    : undefined;
  const fallback = typeof error === "string" && error.trim() ? error : `HTTP ${response.status}`;
  throw new APIError(
    response.status,
    detail?.code ?? "request_failed",
    detail?.message ?? fallback,
    detail?.request_id ?? response.headers.get("X-Request-ID") ?? undefined,
    detail?.details ?? [],
  );
}

export type SSEEvent = { id?: string; event: string; data: unknown };

export async function* readSSE(response: Response): AsyncGenerator<SSEEvent> {
  if (!response.body) throw new APIError(response.status, "stream_unavailable", "response has no stream body");

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let event = "";
  let id: string | undefined;
  let data: string[] = [];

  const dispatch = (): SSEEvent | undefined => {
    if (data.length === 0) return;
    const raw = data.join("\n");
    const result: SSEEvent = { id, event: event || "message", data: raw };
    try {
      result.data = JSON.parse(raw);
    } catch {
      // SSE permits non-JSON data even though Wingman's event payloads are JSON.
    }
    event = "";
    data = [];
    return result;
  };

  const consume = (line: string): SSEEvent | undefined => {
    if (line === "") return dispatch();
    if (line.startsWith(":")) return;
    const separator = line.indexOf(":");
    const field = separator < 0 ? line : line.slice(0, separator);
    const value = separator < 0 ? "" : line.slice(separator + 1).replace(/^ /, "");
    if (field === "event") event = value;
    if (field === "id" && !value.includes("\0")) id = value;
    if (field === "data") data.push(value);
    return;
  };

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      while (true) {
        const newline = buffer.indexOf("\n");
        if (newline < 0) break;
        const line = buffer.slice(0, newline).replace(/\r$/, "");
        buffer = buffer.slice(newline + 1);
        const next = consume(line);
        if (next) yield next;
      }
    }
    buffer += decoder.decode();
    if (buffer) {
      const next = consume(buffer.replace(/\r$/, ""));
      if (next) yield next;
    }
    const next = dispatch();
    if (next) yield next;
  } finally {
    reader.releaseLock();
  }
}

export type UnknownSessionEvent = {
  id: string;
  type: string;
  schema_version?: number;
  time?: string;
  cursor?: { session_id: string; seq: number };
  data: unknown;
};
export type ParsedSessionEvent = { known: true; event: SessionEvent } | { known: false; event: UnknownSessionEvent };

const sessionEventTypes = new Set<SessionEvent["type"]>([
  "session.run.queued", "session.run.started", "session.run.completed", "session.run.failed", "session.run.aborted",
  "session.step.started", "session.step.completed",
  "session.text.delta", "session.text.completed", "session.reasoning.delta", "session.reasoning.completed",
  "session.message.created", "session.tool.called", "session.tool.updated", "session.tool.input.delta",
  "session.tool.progress", "session.tool.completed", "session.tool.failed",
  "session.permission.requested", "session.permission.resolved", "session.structured_output.completed",
  "session.events.synchronized", "session.events.resync_required",
]);

export function parseSessionEvent(value: unknown): ParsedSessionEvent | undefined {
  if (!value || typeof value !== "object") return;
  const event = value as Record<string, unknown>;
  if (typeof event.id !== "string" || typeof event.type !== "string" || event.schema_version !== 1 || !isRecord(event.data)) return;
  const envelope = event as UnknownSessionEvent;
  if (!sessionEventTypes.has(envelope.type as SessionEvent["type"])) return { known: false, event: envelope };
  return { known: true, event: envelope as SessionEvent };
}

export type UnknownRunStreamEvent = { type: string; version: number; data: unknown };
export type ParsedRunStreamEvent = { known: true; event: RunStreamEvent } | { known: false; event: UnknownRunStreamEvent };

const runStreamEventTypes = new Set<RunStreamEvent["type"]>([
  "iteration_start", "iteration_end", "message", "tool_proposed", "tool_authorized", "tool_start",
  "tool_progress", "tool_end", "stream_part", "compaction", "context_transformed", "error",
  "structured_output", "done",
]);

export function parseRunStreamEvent(value: unknown): ParsedRunStreamEvent | undefined {
  if (!value || typeof value !== "object") return;
  const event = value as Record<string, unknown>;
  if (typeof event.type !== "string" || typeof event.version !== "number" || !isRecord(event.data)) return;
  const envelope = event as UnknownRunStreamEvent;
  if (!runStreamEventTypes.has(envelope.type as RunStreamEvent["type"])) return { known: false, event: envelope };
  return { known: true, event: envelope as RunStreamEvent };
}

type StreamOptions = {
  baseUrl?: string;
  fetch?: typeof globalThis.fetch;
  headers?: HeadersInit;
  signal?: AbortSignal;
};

export type SessionStreamOptions = StreamOptions & {
  after?: number;
  limit?: number;
  lastEventID?: number;
};

export async function* streamRun(
  request: components["schemas"]["RunRequest"],
  options: StreamOptions = {},
): AsyncGenerator<ParsedRunStreamEvent> {
  const response = await streamFetch("/run", {
    ...options,
    method: "POST",
    body: JSON.stringify(request),
    headers: { "Content-Type": "application/json", ...options.headers },
  });
  for await (const frame of readSSE(response)) {
    const event = parseRunStreamEvent(frame.data);
    if (event) yield event;
  }
}

export async function* streamSessionEvents(
  sessionID: string,
  options: SessionStreamOptions = {},
): AsyncGenerator<ParsedSessionEvent> {
  const query = new URLSearchParams();
  if (options.after !== undefined) query.set("after", String(options.after));
  if (options.limit !== undefined) query.set("limit", String(options.limit));
  const response = await streamFetch(`/sessions/${encodeURIComponent(sessionID)}/events${query.size ? `?${query}` : ""}`, {
    ...options,
    headers: {
      ...(options.lastEventID !== undefined ? { "Last-Event-ID": String(options.lastEventID) } : {}),
      ...options.headers,
    },
  });
  for await (const frame of readSSE(response)) {
    const event = parseSessionEvent(frame.data);
    if (event) yield event;
  }
}

async function streamFetch(path: string, options: StreamOptions & RequestInit): Promise<Response> {
  const fetcher = options.fetch ?? globalThis.fetch;
  if (!fetcher) throw new Error("fetch is not available; provide options.fetch");
  const headers = new Headers(options.headers);
  headers.set("Accept", "text/event-stream");
  const url = options.baseUrl ? new URL(path, options.baseUrl).toString() : path;
  const response = await fetcher(url, { ...options, headers });
  if (!response.ok) throw await apiErrorFromResponse(response);
  return response;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

export function createWingmanClient(options: ClientOptions = {}) {
  return createClient<paths>(options);
}
