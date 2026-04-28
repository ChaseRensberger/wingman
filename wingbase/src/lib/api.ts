const LS_BASE_URL_KEY = 'wingman_base_url'

function getBaseURL(): string {
  const fromStorage = localStorage.getItem(LS_BASE_URL_KEY)
  if (fromStorage) return fromStorage
  const fromEnv = import.meta.env.VITE_API_URL
  if (fromEnv) return fromEnv
  return 'http://127.0.0.1:2323'
}

export function setBaseURL(url: string) {
  localStorage.setItem(LS_BASE_URL_KEY, url)
}

export function getStoredBaseURL(): string | null {
  return localStorage.getItem(LS_BASE_URL_KEY)
}

export function clearBaseURL() {
  localStorage.removeItem(LS_BASE_URL_KEY)
}

async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const base = getBaseURL()
  const url = `${base.replace(/\/$/, '')}${path}`
  const res = await fetch(url, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `HTTP ${res.status}`)
  }
  return res
}

export async function apiGet<T>(path: string): Promise<T> {
  const res = await apiFetch(path)
  return res.json() as Promise<T>
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  const res = await apiFetch(path, {
    method: 'POST',
    body: body ? JSON.stringify(body) : undefined,
  })
  return res.json() as Promise<T>
}

export async function apiPut<T>(path: string, body?: unknown): Promise<T> {
  const res = await apiFetch(path, {
    method: 'PUT',
    body: body ? JSON.stringify(body) : undefined,
  })
  return res.json() as Promise<T>
}

export async function apiDelete(path: string): Promise<void> {
  await apiFetch(path, { method: 'DELETE' })
}
