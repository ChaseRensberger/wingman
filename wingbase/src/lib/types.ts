export interface ProviderMeta {
  id: string
  name: string
  auth_types: string[]
}

export interface ModelDTO {
  provider: string
  id: string
  context_window?: number
  max_output?: number
  tools: boolean
  images: boolean
  reasoning: boolean
  structured_output: boolean
  input_cost_per_mtok?: number
  output_cost_per_mtok?: number
}

export interface Agent {
  id: string
  name: string
  instructions?: string
  tools?: string[]
  provider?: string
  model?: string
  options?: Record<string, unknown>
  output_schema?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface MessageOrigin {
  provider: string
  api: string
  model_id: string
}

export interface Part {
  type: string
  [key: string]: unknown
}

export interface Message {
  role: 'user' | 'assistant' | 'tool' | 'system'
  content: Part[]
  metadata?: Record<string, unknown>
  origin?: MessageOrigin
  finish_reason?: string
}

export interface Session {
  id: string
  title?: string
  work_dir?: string
  history: Message[]
  created_at: string
  updated_at: string
}

export interface MessageSessionResponse {
  response: string
  tool_calls: ToolCallResult[]
  usage: Usage
  steps: number
}

export interface ToolCallResult {
  call_id: string
  name: string
  input: Record<string, unknown>
  output: string
  is_error: boolean
}

export interface Usage {
  input_tokens: number
  output_tokens: number
  total_tokens: number
  reasoning_tokens?: number
  cached_input_tokens?: number
  cache_write_tokens?: number
}

export interface ProvidersAuthResponse {
  providers: Record<string, ProviderAuthInfo>
  updated_at?: string
}

export interface ProviderAuthInfo {
  type: string
  configured: boolean
}

export interface StreamEvent {
  type: string
  version: number
  data: unknown
}
