import { createWingmanClient, type ErrorResponse } from "@wingman-actor/client";

import type { SessionSummary } from "./types";

export const api = createWingmanClient({ credentials: "same-origin" });
export const daemonConnectionFailureEvent = "wingman:connection-failed";

export function isDaemonConnectionFailure(status: number): boolean {
  return status >= 500;
}

function reportConnectionFailure() {
  if (typeof window !== "undefined") window.dispatchEvent(new Event(daemonConnectionFailureEvent));
}

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

export async function apiErrorFromResponse(res: Response): Promise<APIError> {
  const text = await res.text();
  let body: ErrorResponse | undefined;
  try {
    body = JSON.parse(text) as ErrorResponse;
  } catch {
    // Preserve a useful fallback for proxies and pre-contract servers.
  }
  const error = body?.error;
  return new APIError(
    res.status,
    error?.code ?? "request_failed",
    error?.message ?? (text || `HTTP ${res.status}`),
    error?.request_id ?? res.headers.get("X-Request-ID") ?? undefined,
    error?.details ?? [],
  );
}

type APIResult<T> = {
  data?: T;
  error?: unknown;
  response: Response;
};

export async function apiData<T>(request: Promise<APIResult<T>>): Promise<T> {
  let result: APIResult<T>;
  try {
    result = await request;
  } catch (error) {
    if ((error as Error).name !== "AbortError") reportConnectionFailure();
    throw error;
  }
  const { data, error, response } = result;
  if (error || !response.ok) {
    if (isDaemonConnectionFailure(response.status)) reportConnectionFailure();
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
  return data as T;
}

export async function renameSession(session: Pick<SessionSummary, "id" | "version">, title: string): Promise<SessionSummary> {
  return apiData(api.POST("/sessions/{id}/rename", {
    params: { path: { id: session.id } },
    body: { title, expected_version: session.version },
  })) as Promise<SessionSummary>;
}

export async function moveSession(session: Pick<SessionSummary, "id" | "version">, workingDirectory: string): Promise<SessionSummary> {
  return apiData(api.POST("/sessions/{id}/move", {
    params: { path: { id: session.id } },
    body: { working_directory: workingDirectory, expected_version: session.version },
  })) as Promise<SessionSummary>;
}

export async function purgeSession(session: Pick<SessionSummary, "id" | "version">): Promise<void> {
  await apiData(api.DELETE("/sessions/{id}", {
    params: { path: { id: session.id }, query: { expected_version: session.version } },
  }));
}
