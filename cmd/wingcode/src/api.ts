import type { Agent, Session, ProviderConfig } from "./types"

const BASE = "http://localhost:2323"

async function request(path: string, options?: RequestInit): Promise<any> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...options,
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `HTTP ${res.status}`)
  }
  return res.json()
}

export const api = {
  health(): Promise<{ status: string }> {
    return request("/health")
  },

  createAgent(opts: {
    name: string
    instructions: string
    tools: string[]
    provider?: ProviderConfig
  }): Promise<Agent> {
    return request("/agents", {
      method: "POST",
      body: JSON.stringify(opts),
    })
  },

  listAgents(): Promise<Agent[]> {
    return request("/agents")
  },

  getAgent(id: string): Promise<Agent> {
    return request(`/agents/${id}`)
  },

  createSession(workDir: string): Promise<Session> {
    return request("/sessions", {
      method: "POST",
      body: JSON.stringify({ work_dir: workDir }),
    })
  },

  listSessions(): Promise<Session[]> {
    return request("/sessions")
  },

  getSession(id: string): Promise<Session> {
    return request(`/sessions/${id}`)
  },

  setProviderAuth(provider: string, key: string): Promise<any> {
    return request("/provider/auth", {
      method: "PUT",
      body: JSON.stringify({ provider, key }),
    })
  },

  async *streamMessage(
    sessionID: string,
    agentID: string,
    message: string,
    signal?: AbortSignal,
  ): AsyncGenerator<{ event: string; data: any }> {
    const res = await fetch(`${BASE}/sessions/${sessionID}/message/stream`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ agent_id: agentID, message }),
      signal,
    })

    if (!res.ok) {
      const text = await res.text()
      throw new Error(text || `HTTP ${res.status}`)
    }

    if (!res.body) throw new Error("No response body")

    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ""

    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split("\n")
      buffer = lines.pop() || ""

      let currentEvent = ""

      for (const line of lines) {
        if (line.startsWith("event: ")) {
          currentEvent = line.slice(7).trim()
          continue
        }
        if (!line.startsWith("data: ")) continue

        const data = line.slice(6)
        try {
          yield { event: currentEvent, data: JSON.parse(data) }
        } catch {}
      }
    }
  },
}
