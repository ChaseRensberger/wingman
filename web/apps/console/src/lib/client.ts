import { APIError, createWingmanClient, type components } from "@wingman-actor/client";

import type { SessionSummary } from "./types";

export const daemonConnectionFailureEvent = "wingman:connection-failed";
export { APIError };

export function isDaemonConnectionFailure(status: number): boolean {
  return status >= 500;
}

function reportConnectionFailure() {
  if (typeof window !== "undefined") window.dispatchEvent(new Event(daemonConnectionFailureEvent));
}

async function daemonFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  try {
    const response = await fetch(input, { ...init, credentials: "same-origin" });
    if (isDaemonConnectionFailure(response.status)) reportConnectionFailure();
    return response;
  } catch (error) {
    if ((error as Error).name !== "AbortError") reportConnectionFailure();
    throw error;
  }
}

export const client = createWingmanClient({
  baseUrl: globalThis.location?.origin ?? "http://localhost:2424",
  fetch: daemonFetch,
});

async function tokenAPIError(response: Response): Promise<APIError> {
  const text = await response.text();
  try {
    const body = JSON.parse(text) as components["schemas"]["ErrorResponse"];
    return new APIError(
      response.status,
      body.error?.code ?? "request_failed",
      body.error?.message ?? `HTTP ${response.status}`,
      body.error?.request_id ?? response.headers.get("X-Request-ID") ?? undefined,
      body.error?.details ?? [],
    );
  } catch {
    return new APIError(
      response.status,
      "request_failed",
      text || `HTTP ${response.status}`,
      response.headers.get("X-Request-ID") ?? undefined,
    );
  }
}

export async function rotateClientToken(
  id: string,
): Promise<components["schemas"]["CreateClientResponse"]> {
  const response = await fetch(`/clients/${encodeURIComponent(id)}/token`, {
    method: "POST",
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    if (isDaemonConnectionFailure(response.status)) reportConnectionFailure();
    throw await tokenAPIError(response);
  }
  return response.json() as Promise<components["schemas"]["CreateClientResponse"]>;
}

export async function renameSession(
  session: Pick<SessionSummary, "id" | "version">,
  title: string,
): Promise<SessionSummary> {
  return client.sessions.rename(session.id, {
    title,
    expected_version: session.version,
  }) as Promise<SessionSummary>;
}

export async function moveSession(
  session: Pick<SessionSummary, "id" | "version">,
  workingDirectory: string,
): Promise<SessionSummary> {
  return client.sessions.move(session.id, {
    working_directory: workingDirectory,
    expected_version: session.version,
  }) as Promise<SessionSummary>;
}

export async function purgeSession(session: Pick<SessionSummary, "id" | "version">): Promise<void> {
  await client.sessions.delete(session.id, session.version);
}
