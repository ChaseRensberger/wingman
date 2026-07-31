import type { SessionSummary } from "./types";

type ErrorResponse = {
  error?: {
    code?: string;
    message?: string;
    request_id?: string;
    details?: Array<{ field: string; reason: string }>;
  };
};

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
    error?.details,
  );
}

export async function wfetch(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<unknown> {
  const headers = new Headers(init?.headers);
  if (typeof init?.body === "string" && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(input, { ...init, headers });
  if (!res.ok) {
    throw await apiErrorFromResponse(res);
  }
  return res.json();
}

export async function renameSession(session: Pick<SessionSummary, "id" | "version">, title: string): Promise<SessionSummary> {
  return wfetch(`/sessions/${session.id}/rename`, {
    method: "POST",
    body: JSON.stringify({ title, expected_version: session.version }),
  }) as Promise<SessionSummary>;
}

export async function moveSession(session: Pick<SessionSummary, "id" | "version">, workingDirectory: string): Promise<SessionSummary> {
  return wfetch(`/sessions/${session.id}/move`, {
    method: "POST",
    body: JSON.stringify({ working_directory: workingDirectory, expected_version: session.version }),
  }) as Promise<SessionSummary>;
}

export async function purgeSession(session: Pick<SessionSummary, "id" | "version">): Promise<void> {
  await wfetch(`/sessions/${session.id}?expected_version=${session.version}`, { method: "DELETE" });
}
