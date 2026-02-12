export interface ToolCall {
  id: string
  name: string
  input: string
  result?: string
}

export interface Message {
  role: "user" | "assistant"
  content: string
  toolCalls?: ToolCall[]
}

export interface ContentBlock {
  Type: string
  ID: string
  Name: string
  Text: string
  Input: unknown
}

export interface StreamEvent {
  Type: string
  Text: string
  InputJSON: string
  Index: number
  ContentBlock: ContentBlock | null
  StopReason: string
  Usage: { InputTokens: number; OutputTokens: number } | null
  Error: string | null
}

export interface Agent {
  id: string
  name: string
  instructions: string
  tools: string[]
  provider: ProviderConfig | null
}

export interface ProviderConfig {
  id: string
  model: string
  max_tokens: number
  temperature: number | null
}

export interface Session {
  id: string
  work_dir: string
  history: any[]
  created_at: string
  updated_at: string
}

export interface DoneEvent {
  usage: { InputTokens: number; OutputTokens: number }
  steps: number
}

export type RouteData =
  | { type: "home" }
  | { type: "session"; sessionID: string }
