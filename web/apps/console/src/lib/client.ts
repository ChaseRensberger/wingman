import {
  APIError,
  apiData as clientAPIData,
  apiErrorFromResponse,
  createWingmanClient,
  type components,
} from "@wingman-actor/client";

import type { SessionSummary } from "./types";

export const api = createWingmanClient({ credentials: "same-origin" });
export const daemonConnectionFailureEvent = "wingman:connection-failed";
export { APIError, apiErrorFromResponse };

export function isDaemonConnectionFailure(status: number): boolean {
  return status >= 500;
}

function reportConnectionFailure() {
  if (typeof window !== "undefined") window.dispatchEvent(new Event(daemonConnectionFailureEvent));
}

type APIResult<T> = {
  data?: T;
  error?: unknown;
  response: Response;
};

export async function apiData<T>(request: Promise<APIResult<T>>): Promise<T> {
  try {
    return await clientAPIData(request);
  } catch (error) {
    if ((error as Error).name !== "AbortError" && (!(error instanceof APIError) || isDaemonConnectionFailure(error.status))) {
      reportConnectionFailure();
    }
    throw error;
  }
}

export async function rotateClientToken(id: string): Promise<components["schemas"]["CreateClientResponse"]> {
  const response = await fetch(`/clients/${encodeURIComponent(id)}/token`, {
    method: "POST",
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    if (isDaemonConnectionFailure(response.status)) reportConnectionFailure();
    throw await apiErrorFromResponse(response);
  }
  return response.json() as Promise<components["schemas"]["CreateClientResponse"]>;
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
