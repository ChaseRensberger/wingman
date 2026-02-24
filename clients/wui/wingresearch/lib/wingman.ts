const STORAGE_KEY = "wingresearch-base-url"
const DEFAULT_BASE_URL = "http://127.0.0.1:2323"

export type FormationDefinition = {
  name: string
  version?: number
  description?: string
  defaults?: {
    work_dir?: string
  }
  nodes: Array<Record<string, unknown>>
  edges?: Array<Record<string, unknown>>
}

export type FormationRecord = {
  id: string
  name: string
  version: number
  definition: FormationDefinition
  created_at: string
  updated_at: string
}

export type FormationReport = {
  path: string
  content: string
}

export type FormationRunEvent = {
  type: "run_start" | "node_start" | "node_output" | "edge_emit" | "node_end" | "node_error" | "run_end" | "tool_call"
  node_id?: string
  from?: string
  to?: string
  count?: number
  worker?: string
  tool?: string
  call_id?: string
  output?: Record<string, unknown>
  error?: string
  status?: string
  ts: string
}

export function getBaseUrl(): string {
  if (typeof window === "undefined") {
    return DEFAULT_BASE_URL
  }
  return localStorage.getItem(STORAGE_KEY) || DEFAULT_BASE_URL
}

export function setBaseUrl(url: string) {
  if (typeof window === "undefined") {
    return
  }
  localStorage.setItem(STORAGE_KEY, url)
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${getBaseUrl()}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
  })

  if (!response.ok) {
    const body = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error((body as { error?: string }).error || response.statusText)
  }

  return response.json() as Promise<T>
}

async function streamRequest(path: string, body: unknown, signal: AbortSignal, onEvent: (event: FormationRunEvent) => void) {
  const response = await fetch(`${getBaseUrl()}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  })

  if (!response.ok) {
    const errorBody = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error((errorBody as { error?: string }).error || response.statusText)
  }

  if (!response.body) {
    throw new Error("No stream body returned from server")
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ""

  while (true) {
    const { done, value } = await reader.read()
    if (done) {
      break
    }

    buffer += decoder.decode(value, { stream: true })
    const chunks = buffer.split("\n\n")
    buffer = chunks.pop() || ""

    for (const chunk of chunks) {
      const lines = chunk.split("\n")
      let eventType = ""
      let data = ""

      for (const line of lines) {
        if (line.startsWith("event:")) {
          eventType = line.slice(6).trim()
        }
        if (line.startsWith("data:")) {
          data += line.slice(5).trim()
        }
      }

      if (!eventType || !data) {
        continue
      }

      let parsed: FormationRunEvent
      try {
        parsed = JSON.parse(data) as FormationRunEvent
      } catch {
        continue
      }

      onEvent(parsed)
    }
  }
}

export const api = {
  listFormations: () => request<FormationRecord[]>("/formations"),
  createFormation: (definition: FormationDefinition) =>
    request<FormationRecord>("/formations", {
      method: "POST",
      body: JSON.stringify(definition),
    }),
  updateFormation: (id: string, definition: FormationDefinition) =>
    request<FormationRecord>(`/formations/${id}`, {
      method: "PUT",
      body: JSON.stringify(definition),
    }),
  getFormationReport: (id: string) => request<FormationReport>(`/formations/${id}/report`),
  runFormationStream: (
    formationID: string,
    body: { inputs: Record<string, unknown> },
    signal: AbortSignal,
    onEvent: (event: FormationRunEvent) => void
  ) => streamRequest(`/formations/${formationID}/run/stream`, body, signal, onEvent),
}
