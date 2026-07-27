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

export interface ProviderOAuthAttempt {
  id: string;
  method: "browser" | "device";
  status: "pending" | "completed" | "failed" | "cancelled";
  url?: string;
  instructions?: string;
  error?: string;
}

export interface Session {
  id: string;
  title?: string;
  work_dir?: string;
  workspace_id?: string;
  client_id?: string;
  history: Message[];
  latest_model_call?: ModelCall;
  created_at: string;
  updated_at: string;
}

export interface QuestionRequest {
  id: string;
  session_id: string;
  questions: Array<{ question: string; header: string; options: Array<{ label: string; description: string }>; multiple?: boolean; custom?: boolean }>;
}

export interface Workspace {
  id: string;
  name: string;
  path: string;
  client_id?: string;
  created_at: string;
  updated_at: string;
}

export interface DirectoryListing {
	path: string;
	parent?: string;
	entries: Array<{ name: string; path: string }>;
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
  agent_id?: string;
  model_ref?: string;
  provider?: string;
  api?: string;
  model_id?: string;
  finish_reason?: string;
  stop_reason?: string;
  error_type?: string;
  error_message?: string;
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
  trace?: CallTrace;
  started_at?: string;
  completed_at?: string;
}

export interface CallTrace {
  version: string;
  model: { provider?: string; id?: string; api?: string };
  api?: string;
  provider?: string;
  capabilities: { thinking?: boolean };
  runtime: { current_date: boolean };
  tools?: Array<{ name: string; schema_hash: string; schema_bytes: number }>;
  messages: { count: number; by_role: Record<string, number>; part_kinds: Record<string, number> };
  system: { sha256: string; bytes: number };
  lowered?: { reasoning_summary_auto?: boolean };
}

export type Part =
  | TextPart
  | ReasoningPart
  | ImagePart
  | ToolPart
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

export interface ToolPart {
  type: "tool";
  call_id: string;
  name: string;
  state: "pending" | "running" | "completed" | "error";
  input: Record<string, unknown>;
  output?: string;
  metadata?: Record<string, unknown>;
  error?: string;
  started_at?: number;
  completed_at?: number;
}

export interface ToolActivity {
  call_id: string;
  tool: string;
  status: "pending" | "running" | "completed" | "error";
  input?: Record<string, unknown>;
  input_text?: string;
  output?: string;
  metadata?: Record<string, unknown>;
  error?: string;
  started_at?: string;
  completed_at?: string;
  duration_ms?: number;
}

export interface ToolResultPart {
	type: "tool_result";
	call_id: string;
	name?: string;
  output: Part[];
  is_error?: boolean;
  metadata?: Record<string, unknown>;
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

export interface ToolCatalogItem {
  name: string;
  description?: string;
  input_schema?: Record<string, unknown>;
  source: "native" | "plugin" | "mcp" | string;
  plugin?: string;
  server?: string;
  remote_name?: string;
  status?: string;
}

export interface ToolsResponse {
  tools: ToolCatalogItem[];
}

export interface MCPServer {
  name: string;
  type: string;
  status: string;
  error?: string;
  tools?: string[];
  tool_count: number;
}

export interface MCPResponse {
  servers: MCPServer[];
}
