import type { Session } from "./types";

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
    const text = await res.text();
    throw new Error(`HTTP ${res.status}: ${text}`);
  }
  return res.json();
}

export async function renameSession(session: Pick<Session, "id" | "version">, title: string): Promise<Session> {
  return wfetch(`/sessions/${session.id}/rename`, {
    method: "POST",
    body: JSON.stringify({ title, expected_version: session.version }),
  }) as Promise<Session>;
}

export async function moveSession(session: Pick<Session, "id" | "version">, workingDirectory: string): Promise<Session> {
  return wfetch(`/sessions/${session.id}/move`, {
    method: "POST",
    body: JSON.stringify({ working_directory: workingDirectory, expected_version: session.version }),
  }) as Promise<Session>;
}
