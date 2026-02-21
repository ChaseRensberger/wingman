const STORAGE_KEY = "wingman-admin-base-url";
const DEFAULT_BASE_URL = "http://localhost:9999";

export function getBaseUrl(): string {
  return localStorage.getItem(STORAGE_KEY) || DEFAULT_BASE_URL;
}

export function setBaseUrl(url: string) {
  localStorage.setItem(STORAGE_KEY, url);
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${getBaseUrl()}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || res.statusText);
  }
  return res.json();
}

export type ProviderConfig = {
  id: string;
  model?: string;
  max_tokens?: number;
  temperature?: number;
};

export type Agent = {
  id: string;
  name: string;
  instructions: string;
  tools: string[];
  provider?: ProviderConfig;
  created_at: string;
  updated_at: string;
};

export type CreateAgentRequest = {
  name: string;
  instructions?: string;
  tools?: string[];
  provider?: ProviderConfig;
};

export type UpdateAgentRequest = {
  name?: string;
  instructions?: string;
  tools?: string[];
  provider?: ProviderConfig;
};

export type Session = {
  id: string;
  work_dir: string;
  history: unknown[];
  created_at: string;
  updated_at: string;
};

export type CreateSessionRequest = {
  work_dir?: string;
};

export type ProviderAuthInfo = {
  type: string;
  configured: boolean;
};

export type ProvidersAuthResponse = {
  providers: Record<string, ProviderAuthInfo>;
  updated_at?: string;
};

export type AuthCredential = {
  type: string;
  key?: string;
  access_token?: string;
  refresh_token?: string;
  expires_at?: string;
};

export type SetProvidersAuthRequest = {
  providers: Record<string, AuthCredential>;
};

export const api = {
  health: () => request<{ status: string }>("/health"),

  listAgents: () => request<Agent[]>("/agents"),
  getAgent: (id: string) => request<Agent>(`/agents/${id}`),
  createAgent: (data: CreateAgentRequest) =>
    request<Agent>("/agents", { method: "POST", body: JSON.stringify(data) }),
  updateAgent: (id: string, data: UpdateAgentRequest) =>
    request<Agent>(`/agents/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  deleteAgent: (id: string) =>
    request<{ status: string }>(`/agents/${id}`, { method: "DELETE" }),

  listSessions: () => request<Session[]>("/sessions"),
  getSession: (id: string) => request<Session>(`/sessions/${id}`),
  createSession: (data: CreateSessionRequest) =>
    request<Session>("/sessions", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  deleteSession: (id: string) =>
    request<{ status: string }>(`/sessions/${id}`, { method: "DELETE" }),

  getProvidersAuth: () => request<ProvidersAuthResponse>("/provider/auth"),
  setProvidersAuth: (data: SetProvidersAuthRequest) =>
    request<{ status: string }>("/provider/auth", {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  deleteProviderAuth: (provider: string) =>
    request<{ status: string }>(`/provider/auth/${provider}`, {
      method: "DELETE",
    }),
};
