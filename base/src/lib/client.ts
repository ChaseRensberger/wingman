const CLIENT_ID_KEY = "wingman_client_id";

export interface Client {
  id: string;
  name: string;
  created_at: number;
}

export async function ensureClient(): Promise<string> {
  const stored = localStorage.getItem(CLIENT_ID_KEY);
  if (stored) {
    try {
      const res = await fetch(`/clients/${stored}`);
      if (res.status === 404) {
        localStorage.removeItem(CLIENT_ID_KEY);
      } else if (res.ok) {
        return stored;
      }
    } catch {
      return stored;
    }
  }

  const res = await fetch("/clients", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name: "wingbase" }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Failed to register client: ${res.status} ${text}`);
  }
  const client: Client = await res.json();
  localStorage.setItem(CLIENT_ID_KEY, client.id);
  return client.id;
}

export function getClientId(): string | null {
  return localStorage.getItem(CLIENT_ID_KEY);
}

export async function wfetch(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<unknown> {
  const clientId = getClientId();
  const headers = new Headers(init?.headers);
  if (clientId) {
    headers.set("X-Wingman-Client", clientId);
  }
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
