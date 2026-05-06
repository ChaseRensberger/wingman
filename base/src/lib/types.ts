export interface Agent {
  id: string;
  name: string;
  instructions?: string;
  tools?: string[];
  provider?: string;
  model?: string;
  options?: Record<string, unknown>;
  output_schema?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface Session {
  id: string;
  title?: string;
  work_dir?: string;
  client_id?: string;
  history: Message[];
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
