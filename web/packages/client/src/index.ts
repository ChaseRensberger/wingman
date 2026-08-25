import createClient from "openapi-fetch";

import type { components, paths } from "./schema";

export type { components, operations, paths } from "./schema";

export type Agent = components["schemas"]["Agent"];
export type Client = components["schemas"]["Client"];
export type CreateAgentRequest = components["schemas"]["CreateAgentRequest"];
export type CreateClientRequest = components["schemas"]["CreateClientRequest"];
export type CreateSessionRequest = components["schemas"]["CreateSessionRequest"];
export type ErrorResponse = components["schemas"]["ErrorResponse"];
export type MessageSessionRequest = components["schemas"]["MessageSessionRequest"];
export type PermissionReplyRequest = components["schemas"]["PermissionReplyRequest"];
export type MessageSessionResponse = components["schemas"]["MessageSessionResponse"];
export type RunRequest = components["schemas"]["RunRequest"];
export type RunStreamEvent = components["schemas"]["RunStreamEvent"];
export type Session = components["schemas"]["Session"];
export type SessionDetail = components["schemas"]["SessionDetail"];
export type SessionEvent = components["schemas"]["SessionEvent"];
export type SessionRun = components["schemas"]["SessionRun"];
export type ToolUse = components["schemas"]["ToolUse"];
export type Workspace = components["schemas"]["Workspace"];

export class APIError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId?: string;
  readonly details: Array<{ field: string; reason: string }>;
  readonly headers: Headers;
  readonly retryAfterMs: number;

  constructor(
    status: number,
    code: string,
    message: string,
    requestId?: string,
    details: Array<{ field: string; reason: string }> = [],
    headers: HeadersInit = {},
  ) {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
    this.details = details;
    this.headers = new Headers(headers);
    this.retryAfterMs = retryAfter(this.headers.get("Retry-After"));
  }
}

export class StreamError extends Error {
  readonly code: "invalid_stream" | "stream_interrupted" | "stream_too_large";

  constructor(code: StreamError["code"], message: string) {
    super(message);
    this.name = "StreamError";
    this.code = code;
  }
}

async function apiErrorFromResponse(response: Response): Promise<APIError> {
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
    response.headers,
  );
}

type APIResult<T> = Promise<{ data?: T; error?: unknown; response: Response }>;

async function requestData<T>(request: APIResult<T>): Promise<T> {
  const { data, error, response } = await request;
  if (!error && response.ok) return data as T;
  const detail =
    error && typeof error === "object" && "error" in error
      ? (error as ErrorResponse).error
      : undefined;
  const fallback = typeof error === "string" && error.trim() ? error : `HTTP ${response.status}`;
  throw new APIError(
    response.status,
    detail?.code ?? "request_failed",
    detail?.message ?? fallback,
    detail?.request_id ?? response.headers.get("X-Request-ID") ?? undefined,
    detail?.details ?? [],
    response.headers,
  );
}

export type SSEEvent = { id?: string; event: string; data: unknown };

export async function* readSSE(
  response: Response,
  maxEventBytes = 1 << 20,
): AsyncGenerator<SSEEvent> {
  if (!response.body)
    throw new APIError(response.status, "stream_unavailable", "response has no stream body");
  if (!Number.isSafeInteger(maxEventBytes) || maxEventBytes <= 0) {
    throw new RangeError("maximum SSE event size must be a positive integer");
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  const encoder = new TextEncoder();
  let buffer = "";
  let event = "";
  let id: string | undefined;
  let data: string[] = [];
  let eventBytes = 0;
  const dispatch = (): SSEEvent | undefined => {
    if (data.length === 0) return;
    const raw = data.join("\n");
    const result: SSEEvent = { id, event: event || "message", data: raw };
    try {
      result.data = JSON.parse(raw);
    } catch {
      /* SSE permits non-JSON data. */
    }
    event = "";
    data = [];
    eventBytes = 0;
    return result;
  };
  const consume = (line: string): SSEEvent | undefined => {
    eventBytes += encoder.encode(line).byteLength + 1;
    if (eventBytes > maxEventBytes) {
      throw new StreamError("stream_too_large", `server-sent event exceeds ${maxEventBytes} bytes`);
    }
    if (line === "") return dispatch();
    if (line.startsWith(":")) return;
    const separator = line.indexOf(":");
    const field = separator < 0 ? line : line.slice(0, separator);
    const value = separator < 0 ? "" : line.slice(separator + 1).replace(/^ /, "");
    if (field === "event") event = value;
    if (field === "id" && !value.includes("\0")) id = value;
    if (field === "data") data.push(value);
  };
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      while (true) {
        const newline = buffer.indexOf("\n");
        if (newline < 0) break;
        const next = consume(buffer.slice(0, newline).replace(/\r$/, ""));
        buffer = buffer.slice(newline + 1);
        if (next) yield next;
      }
      if (encoder.encode(buffer).byteLength + eventBytes > maxEventBytes) {
        throw new StreamError(
          "stream_too_large",
          `server-sent event exceeds ${maxEventBytes} bytes`,
        );
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
export type ParsedSessionEvent =
  | { known: true; event: SessionEvent }
  | { known: false; event: UnknownSessionEvent };

const sessionEventTypes = new Set<SessionEvent["type"]>([
  "session.run.queued",
  "session.run.started",
  "session.run.completed",
  "session.run.failed",
  "session.run.aborted",
  "session.step.started",
  "session.step.completed",
  "session.text.delta",
  "session.text.completed",
  "session.reasoning.delta",
  "session.reasoning.completed",
  "session.message.created",
  "session.tool.called",
  "session.tool.updated",
  "session.tool.input.delta",
  "session.tool.progress",
  "session.tool.completed",
  "session.tool.failed",
  "session.permission.requested",
  "session.permission.resolved",
  "session.structured_output.completed",
  "session.events.synchronized",
  "session.events.resync_required",
]);

export function parseSessionEvent(value: unknown): ParsedSessionEvent | undefined {
  if (!value || typeof value !== "object") return;
  const event = value as Record<string, unknown>;
  if (
    typeof event.id !== "string" ||
    typeof event.type !== "string" ||
    event.schema_version !== 1 ||
    !isRecord(event.data)
  )
    return;
  const envelope = event as UnknownSessionEvent;
  return sessionEventTypes.has(envelope.type as SessionEvent["type"])
    ? { known: true, event: envelope as SessionEvent }
    : { known: false, event: envelope };
}

export type UnknownRunStreamEvent = {
  type: string;
  version: number;
  data: unknown;
};
export type ParsedRunStreamEvent =
  | { known: true; event: RunStreamEvent }
  | { known: false; event: UnknownRunStreamEvent };

const runStreamEventTypes = new Set<RunStreamEvent["type"]>([
  "iteration_start",
  "iteration_end",
  "message",
  "tool_proposed",
  "tool_authorized",
  "tool_start",
  "tool_progress",
  "tool_end",
  "stream_part",
  "compaction",
  "context_transformed",
  "error",
  "structured_output",
  "done",
]);

export function parseRunStreamEvent(value: unknown): ParsedRunStreamEvent | undefined {
  if (!value || typeof value !== "object") return;
  const event = value as Record<string, unknown>;
  if (typeof event.type !== "string" || typeof event.version !== "number" || !isRecord(event.data))
    return;
  const envelope = event as UnknownRunStreamEvent;
  return runStreamEventTypes.has(envelope.type as RunStreamEvent["type"])
    ? { known: true, event: envelope as RunStreamEvent }
    : { known: false, event: envelope };
}

export type WingmanClientOptions = {
  baseUrl: string;
  username?: string;
  password?: string;
  clientName?: string;
  headers?: HeadersInit;
  fetch?: typeof globalThis.fetch;
  maxSSEEventBytes?: number;
};
export type SessionEventStreamOptions = {
  after?: number;
  limit?: number;
  lastEventID?: number;
  signal?: AbortSignal;
};
export type RunStreamOptions = { signal?: AbortSignal };
export type ReadinessOptions = { signal?: AbortSignal };

export function createWingmanClient(options: WingmanClientOptions) {
  const baseUrl = originURL(options.baseUrl);
  const maxSSEEventBytes = options.maxSSEEventBytes ?? 1 << 20;
  if (!Number.isSafeInteger(maxSSEEventBytes) || maxSSEEventBytes <= 0) {
    throw new RangeError("maximum SSE event size must be a positive integer");
  }
  const headers = new Headers(options.headers);
  if (options.password !== undefined)
    headers.set(
      "Authorization",
      `Basic ${base64((options.username ?? "wingman") + ":" + options.password)}`,
    );
  if (options.clientName !== undefined) headers.set("X-Wingman-Client", options.clientName);
  const api = createClient<paths>({
    baseUrl,
    headers,
    fetch: options.fetch,
  });
  const streamConfig = {
    baseUrl,
    headers,
    fetch: options.fetch,
    maxSSEEventBytes,
  };

  return {
    agents: {
      list: () => requestData(api.GET("/agents")),
      create: (request: CreateAgentRequest) => requestData(api.POST("/agents", { body: request })),
      get: (id: string) => requestData(api.GET("/agents/{id}", { params: { path: { id } } })),
      update: (id: string, request: components["schemas"]["UpdateAgentRequest"]) =>
        requestData(api.PUT("/agents/{id}", { params: { path: { id } }, body: request })),
      delete: (id: string) => requestData(api.DELETE("/agents/{id}", { params: { path: { id } } })),
    },
    clients: {
      list: () => requestData(api.GET("/clients")),
      create: (request: CreateClientRequest) =>
        requestData(api.POST("/clients", { body: request })),
      get: (id: string) => requestData(api.GET("/clients/{id}", { params: { path: { id } } })),
      ensure: async (id: string, name: string) => {
        const request = { id: id.trim(), name: name.trim() };
        try {
          return (await requestData(api.POST("/clients", { body: request }))).client;
        } catch (error) {
          if (!(error instanceof APIError) || error.code !== "conflict") {
            throw error;
          }
        }
        const existing = await requestData(
          api.GET("/clients/{id}", { params: { path: { id: request.id } } }),
        );
        if (existing.id !== request.id || existing.name !== request.name) {
          throw new APIError(
            409,
            "conflict",
            `client ${request.id} does not match the requested identity`,
          );
        }
        return existing;
      },
    },
    sessions: {
      list: () => requestData(api.GET("/sessions")),
      create: (request: CreateSessionRequest) =>
        requestData(api.POST("/sessions", { body: request })),
      get: (id: string) => requestData(api.GET("/sessions/{id}", { params: { path: { id } } })),
      delete: (id: string, expectedVersion: number) =>
        requestData(
          api.DELETE("/sessions/{id}", {
            params: {
              path: { id },
              query: { expected_version: expectedVersion },
            },
          }),
        ),
      abort: (id: string) =>
        requestData(api.POST("/sessions/{id}/abort", { params: { path: { id } } })),
      listEvents: (id: string, query?: { after?: number; limit?: number }) =>
        requestData(
          api.GET("/sessions/{id}/events/history", {
            params: { path: { id }, query },
          }),
        ),
      streamEvents: (id: string, streamOptions?: SessionEventStreamOptions) =>
        streamSessionEvents(id, streamConfig, streamOptions),
      message: (id: string, request: MessageSessionRequest) =>
        requestData(
          api.POST("/sessions/{id}/message", {
            params: { path: { id } },
            body: request,
          }),
        ),
      admit: (id: string, request: MessageSessionRequest) => {
        if (!request.request_id) {
          throw new TypeError(
            "message admission requires a request_id; call newMessageAdmission and persist its result before retrying",
          );
        }
        return requestData(
          api.POST("/sessions/{id}/message", {
            params: { path: { id } },
            body: request,
          }),
        );
      },
      modelCalls: {
        list: (id: string) =>
          requestData(api.GET("/sessions/{id}/model-calls", { params: { path: { id } } })),
      },
      move: (id: string, request: components["schemas"]["MoveSessionRequest"]) =>
        requestData(
          api.POST("/sessions/{id}/move", {
            params: { path: { id } },
            body: request,
          }),
        ),
      permissionGrants: {
        list: (id: string) =>
          requestData(
            api.GET("/sessions/{id}/permission-grants", {
              params: { path: { id } },
            }),
          ),
      },
      permissionRequests: {
        list: (id: string) =>
          requestData(
            api.GET("/sessions/{id}/permission-requests", {
              params: { path: { id } },
            }),
          ),
        reply: (id: string, requestID: string, request: PermissionReplyRequest) =>
          requestData(
            api.POST("/sessions/{id}/permission-requests/{requestID}/reply", {
              params: { path: { id, requestID } },
              body: request,
            }),
          ),
      },
      rename: (id: string, request: components["schemas"]["RenameSessionRequest"]) =>
        requestData(
          api.POST("/sessions/{id}/rename", {
            params: { path: { id } },
            body: request,
          }),
        ),
      runs: {
        list: (id: string) =>
          requestData(api.GET("/sessions/{id}/runs", { params: { path: { id } } })),
        get: (id: string, runID: string) =>
          requestData(
            api.GET("/sessions/{id}/runs/{runID}", {
              params: { path: { id, runID } },
            }),
          ),
        abort: (id: string, runID: string) =>
          requestData(
            api.POST("/sessions/{id}/runs/{runID}/abort", {
              params: { path: { id, runID } },
            }),
          ),
      },
      toolUses: {
        list: (id: string) =>
          requestData(api.GET("/sessions/{id}/tool-uses", { params: { path: { id } } })),
      },
    },
    run: {
      stream: (request: RunRequest, streamOptions?: RunStreamOptions) =>
        streamRun(request, streamConfig, streamOptions),
    },
    workspaces: {
      list: () => requestData(api.GET("/workspaces")),
      create: (request: components["schemas"]["CreateWorkspaceRequest"]) =>
        requestData(api.POST("/workspaces", { body: request })),
      get: (id: string) => requestData(api.GET("/workspaces/{id}", { params: { path: { id } } })),
      update: (id: string, request: components["schemas"]["UpdateWorkspaceRequest"]) =>
        requestData(
          api.PUT("/workspaces/{id}", {
            params: { path: { id } },
            body: request,
          }),
        ),
      delete: (id: string) =>
        requestData(api.DELETE("/workspaces/{id}", { params: { path: { id } } })),
      sessions: {
        list: (id: string) =>
          requestData(api.GET("/workspaces/{id}/sessions", { params: { path: { id } } })),
      },
    },
    catalog: {
      get: () => requestData(api.GET("/catalog")),
      logo: (id: string) =>
        requestData(api.GET("/catalog/labs/{id}/logo", { params: { path: { id } } })),
    },
    current: {
      service: () => requestData(api.GET("/")),
      client: () => requestData(api.GET("/client")),
    },
    diagnostics: { get: () => requestData(api.GET("/diagnostics")) },
    filesystem: {
      directories: (path?: string) =>
        requestData(api.GET("/filesystem/directories", { params: { query: { path } } })),
    },
    health: {
      get: () => requestData(api.GET("/health")),
      ready: (options?: ReadinessOptions) => requestData(api.GET("/ready", options)),
    },
    logs: { list: () => requestData(api.GET("/logs")) },
    mcp: {
      list: () => requestData(api.GET("/mcp")),
      authorize: (name: string) =>
        requestData(api.POST("/mcp/{name}/auth", { params: { path: { name } } })),
      logout: (name: string) =>
        requestData(api.DELETE("/mcp/{name}/auth", { params: { path: { name } } })),
      connect: (name: string) =>
        requestData(api.POST("/mcp/{name}/connect", { params: { path: { name } } })),
      disconnect: (name: string) =>
        requestData(api.POST("/mcp/{name}/disconnect", { params: { path: { name } } })),
    },
    plugins: {
      list: () => requestData(api.GET("/plugins")),
      reload: () => requestData(api.POST("/plugins/reload")),
    },
    providers: {
      list: () => requestData(api.GET("/provider")),
      auth: {
        get: () => requestData(api.GET("/provider/auth")),
        set: (request: components["schemas"]["SetProvidersAuthRequest"]) =>
          requestData(api.PUT("/provider/auth", { body: request })),
        delete: (provider: string) =>
          requestData(
            api.DELETE("/provider/auth/{provider}", {
              params: { path: { provider } },
            }),
          ),
      },
      get: (name: string) =>
        requestData(api.GET("/provider/{name}", { params: { path: { name } } })),
      models: {
        list: (name: string) =>
          requestData(api.GET("/provider/{name}/models", { params: { path: { name } } })),
        get: (name: string, model: string) =>
          requestData(
            api.GET("/provider/{name}/models/{model}", {
              params: { path: { name, model } },
            }),
          ),
      },
      oauth: {
        authorize: (name: string, request: components["schemas"]["ProviderOAuthRequest"]) =>
          requestData(
            api.POST("/provider/{name}/oauth/authorize", {
              params: { path: { name } },
              body: request,
            }),
          ),
        get: (name: string, attempt: string) =>
          requestData(
            api.GET("/provider/{name}/oauth/{attempt}", {
              params: { path: { name, attempt } },
            }),
          ),
        cancel: (name: string, attempt: string) =>
          requestData(
            api.DELETE("/provider/{name}/oauth/{attempt}", {
              params: { path: { name, attempt } },
            }),
          ),
      },
    },
    tools: { list: () => requestData(api.GET("/tools")) },
  };
}

async function* streamRun(
  request: RunRequest,
  config: StreamConfig,
  options?: RunStreamOptions,
): AsyncGenerator<ParsedRunStreamEvent> {
  const response = await streamFetch("/run", config, {
    method: "POST",
    body: JSON.stringify(request),
    headers: { "Content-Type": "application/json" },
    signal: options?.signal,
  });
  let terminal = false;
  for await (const frame of readSSE(response, config.maxSSEEventBytes)) {
    const event = parseRunStreamEvent(frame.data);
    if (!event) continue;
    if (event.known && (event.event.type === "done" || event.event.type === "error")) {
      terminal = true;
    }
    yield event;
  }
  if (!terminal) {
    throw new StreamError("stream_interrupted", "run stream ended without a terminal event");
  }
}

async function* streamSessionEvents(
  id: string,
  config: StreamConfig,
  options?: SessionEventStreamOptions,
): AsyncGenerator<ParsedSessionEvent> {
  const query = new URLSearchParams();
  if (options?.after !== undefined) query.set("after", String(options.after));
  if (options?.limit !== undefined) query.set("limit", String(options.limit));
  const response = await streamFetch(
    `/sessions/${encodeURIComponent(id)}/events${query.size ? `?${query}` : ""}`,
    config,
    {
      headers:
        options?.lastEventID === undefined
          ? undefined
          : { "Last-Event-ID": String(options.lastEventID) },
      signal: options?.signal,
    },
  );
  for await (const frame of readSSE(response, config.maxSSEEventBytes)) {
    const event = parseSessionEvent(frame.data);
    if (event) yield event;
  }
}

type StreamConfig = Pick<
  WingmanClientOptions,
  "baseUrl" | "headers" | "fetch" | "maxSSEEventBytes"
>;

async function streamFetch(
  path: string,
  config: StreamConfig,
  init: RequestInit,
): Promise<Response> {
  const fetcher = config.fetch ?? globalThis.fetch;
  if (!fetcher) throw new Error("fetch is not available; provide options.fetch");
  const headers = new Headers(config.headers);
  new Headers(init.headers).forEach((value, name) => headers.set(name, value));
  headers.set("Accept", "text/event-stream");
  const response = await fetcher(new URL(path, config.baseUrl).toString(), {
    ...init,
    headers,
  });
  if (!response.ok) throw await apiErrorFromResponse(response);
  const contentType = response.headers.get("Content-Type")?.split(";", 1)[0]?.trim().toLowerCase();
  if (contentType !== "text/event-stream") {
    response.body?.cancel();
    throw new StreamError(
      "invalid_stream",
      `expected text/event-stream response, got ${JSON.stringify(contentType ?? "")}`,
    );
  }
  return response;
}

export function newMessageAdmission(request: MessageSessionRequest): MessageSessionRequest {
  if (request.request_id) return request;
  return { ...request, request_id: crypto.randomUUID() };
}

function originURL(value: string): string {
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new TypeError(`base URL must be an HTTP or HTTPS origin: ${JSON.stringify(value)}`);
  }
  if (
    (url.protocol !== "http:" && url.protocol !== "https:") ||
    !url.host ||
    url.username ||
    url.password ||
    url.search ||
    url.hash ||
    (url.pathname !== "" && url.pathname !== "/")
  ) {
    throw new TypeError(`base URL must be an HTTP or HTTPS origin: ${JSON.stringify(value)}`);
  }
  return url.origin;
}

function retryAfter(value: string | null): number {
  if (!value) return 0;
  if (/^\d+$/.test(value)) return Number(value) * 1000;
  const at = Date.parse(value);
  return Number.isNaN(at) ? 0 : Math.max(0, at - Date.now());
}

function base64(value: string): string {
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}
