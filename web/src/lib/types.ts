export interface Agent {
  id: string;
  name: string;
  instructions?: string;
  tools?: string[];
  model_ref?: string;
  options?: Record<string, unknown>;
  output_schema?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface Provider {
  id: string;
  name: string;
  auth_types: ProviderAuthType[];
  auth: ProviderAuthStatus;
  route: ProviderRoute;
}

export interface ProviderRoute {
  base_url?: string;
  base_url_source: "catalog" | "config";
  auth_enabled: boolean;
  auth_source: "default" | "config";
}

export interface ProviderAuthType {
  type: string;
  name?: string;
}

export interface ProviderAuthStatus {
  configured: boolean;
  source: "stored" | "env" | "none" | "disabled";
  env?: string;
}

export interface ProviderModel {
  provider: string;
  id: string;
  context_window?: number;
  max_output?: number;
  tools: boolean;
  images: boolean;
  reasoning: boolean;
  structured_output: boolean;
  input_cost_per_mtok?: number;
  output_cost_per_mtok?: number;
}

export interface ProviderAuthResponse {
  providers: Record<string, { type: string; configured: boolean }>;
  updated_at?: string;
}

export interface Session {
  id: string;
  title?: string;
  work_dir?: string;
  client_id?: string;
  history: Message[];
  latest_model_call?: ModelCall;
  created_at: string;
  updated_at: string;
}

export interface Message {
  role: "user" | "assistant" | "tool";
  content: Part[];
  metadata?: Record<string, unknown>;
  origin?: {
    provider: string;
    api: string;
    model_id: string;
  };
  finish_reason?: string;
  usage?: Usage;
}

export interface Usage {
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  reasoning_tokens?: number;
  cached_input_tokens?: number;
  cache_write_tokens?: number;
}

export interface ModelCall {
  id: string;
  session_id: string;
  assistant_message_id?: string;
  step: number;
  attempt: number;
  status: string;
  model_ref?: string;
  provider?: string;
  api?: string;
  model_id?: string;
  finish_reason?: string;
  stop_reason?: string;
  input_tokens: number;
  output_tokens: number;
  reasoning_tokens?: number;
  cached_input_tokens?: number;
  cache_write_tokens?: number;
  total_tokens: number;
  context_tokens: number;
  context_window?: number;
  context_percent?: number;
  cost?: number;
}

export type Part =
  | TextPart
  | ReasoningPart
  | ImagePart
  | ToolCallPart
  | ToolResultPart
  | OpaquePart;

export interface TextPart {
  type: "text";
  text: string;
  signature?: string;
  provider_options?: unknown;
}

export interface ReasoningPart {
  type: "reasoning";
  reasoning: string;
  signature?: string;
  redacted?: boolean;
  provider_options?: unknown;
}

export interface ImagePart {
  type: "image";
  data: string;
  mime_type: string;
  provider_options?: unknown;
}

export interface ToolCallPart {
  type: "tool_call";
  call_id: string;
  name: string;
  input: Record<string, unknown>;
  signature?: string;
  provider_options?: unknown;
}

export interface ToolResultPart {
  type: "tool_result";
  call_id: string;
  output: Part[];
  is_error?: boolean;
  provider_options?: unknown;
}

export interface OpaquePart {
  type: string;
  [key: string]: unknown;
}

export interface LogEntry {
  raw: string;
  time?: string;
  level?: string;
  msg?: string;
  attrs?: Record<string, unknown>;
}
