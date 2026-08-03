export interface Agent {
  id: string;
  name: string;
  instructions?: string;
  tools?: string[];
  permissions?: Array<{
    action: string;
    resource: string;
    effect: "allow" | "ask" | "deny";
  }>;
  model_ref?: string;
  options?: Record<string, unknown>;
  output_schema?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface AgentSpec {
  id?: string;
  name: string;
  instructions?: string;
  tools?: string[];
  permissions?: Agent["permissions"];
  model_ref?: string;
  options?: Record<string, unknown>;
  output_schema?: Record<string, unknown>;
}

export interface Client {
  id: string;
  name: string;
  created_at: string;
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

export interface PluginDiagnostic {
  source: string;
  level?: string;
  message: string;
  fields?: Record<string, unknown>;
}

export interface PluginStatus {
  id: string;
  name?: string;
  path: string;
  tools?: string[];
  running: boolean;
  error?: string;
  protocol_version?: number;
  plugin_version?: string;
  capabilities?: string[];
  status: "running" | "degraded" | "failed" | "stopped";
  pid?: number;
  started_at?: string;
  exited_at?: string;
  last_health_at?: string;
  health_message?: string;
  diagnostics?: PluginDiagnostic[];
}

export interface PluginsResponse {
  plugins: PluginStatus[];
  errors?: Array<{ path: string; error: string }>;
}

export interface SessionSummary {
  id: string;
  version: number;
  title?: string;
  work_dir?: string;
  workspace_id?: string;
  client_id?: string;
  created_at: string;
  updated_at: string;
}

export interface Session extends SessionSummary {
  history: Message[];
  latest_model_call?: ModelCall;
}

export interface SessionRun {
  id: string;
  session_id: string;
  request_id?: string;
  admitted_version: number;
  work_dir?: string;
  workspace_id?: string;
  client_id?: string;
  sequence: number;
  status: "queued" | "running" | "completed" | "failed" | "aborted";
  message: string;
  agent: Agent;
  error_type?: string;
  error_message?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  updated_at: string;
}

export interface PermissionRequest {
  id: string;
  session_id: string;
  run_id?: string;
  tool_use_id?: string;
  call_id?: string;
  action: string;
  resources: string[];
  status: "pending" | "approved" | "rejected" | "timed_out" | "interrupted";
  response?: "once" | "always" | "reject";
  error_type?: string;
  error_message?: string;
  created_at: string;
  resolved_at?: string;
  updated_at: string;
}

export interface PermissionGrant {
  id: string;
  session_id: string;
  action: string;
  resource: string;
  created_at: string;
}

export interface ToolUse {
  id: string;
  session_id: string;
  run_id?: string;
  model_call_id?: string;
  assistant_message_id?: string;
  part_id?: string;
  step: number;
  ordinal: number;
  call_id?: string;
  name: string;
  status: "proposed" | "authorized" | "started" | "completed" | "failed" | "interrupted" | "declined";
  input?: Record<string, unknown>;
  output?: string;
  structured?: unknown;
  metadata?: Record<string, unknown>;
  error_type?: string;
  error_message?: string;
  proposed_at?: string;
  authorized_at?: string;
  started_at?: string;
  completed_at?: string;
  created_at?: string;
  updated_at?: string;
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
  id?: string;
  revision?: number;
  state?: "in_progress" | "completed" | "failed";
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
  run_id?: string;
  assistant_message_id?: string;
  step: number;
  attempt: number;
  status: string;
  agent_id?: string;
  model_ref?: string;
  provider?: string;
  provider_request_id?: string;
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
  id?: string;
  type: "text";
  text: string;
  signature?: string;
  provider_options?: unknown;
}

export interface ReasoningPart {
  id?: string;
  type: "reasoning";
  reasoning: string;
  signature?: string;
  redacted?: boolean;
  provider_options?: unknown;
}

export interface ImagePart {
  id?: string;
  type: "image";
  url?: string;
  base64?: string;
  media_type?: string;
  provider_metadata?: Record<string, unknown>;
}

export interface ToolCallPart {
  id?: string;
  type: "tool_call";
  tool_use_id?: string;
  call_id: string;
  name: string;
  input: Record<string, unknown>;
  signature?: string;
  provider_options?: unknown;
}

export interface ToolPart {
  id?: string;
  type: "tool";
  tool_use_id?: string;
  call_id: string;
  name: string;
  state: "pending" | "running" | "completed" | "error";
  input: Record<string, unknown>;
  input_raw?: string;
  output?: string;
	output_parts?: Part[];
	structured?: unknown;
  metadata?: Record<string, unknown>;
  error?: string;
  started_at?: number;
  completed_at?: number;
}

export interface ToolActivity {
  tool_use_id?: string;
  run_id?: string;
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
	id?: string;
	type: "tool_result";
  tool_use_id?: string;
	call_id: string;
	name?: string;
  output: Part[];
	structured?: unknown;
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
  output_schema?: Record<string, unknown>;
  sequential?: boolean;
  directory_scoped?: boolean;
  permission?: {
    action: string;
    resource_fields?: string[];
  };
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
