export type Message = TextMessage | ToolMessage;

export interface TextMessage {
	role: "user" | "assistant";
	content: string;
}

export interface ToolMessage {
	role: "tool";
	toolID?: string;
	toolName?: string;
	input?: string;
	output?: string;
	status: "running" | "done" | "error";
}

export interface StreamEvent {
	type?: string;
	text?: string;
	input_json?: string;
	index?: number;
	content_block?: {
		type?: string;
		id?: string;
		name?: string;
	};
	stop_reason?: string;
	error?: string;
}

export interface DoneEvent {
	usage?: { input_tokens: number; output_tokens: number };
	steps?: number;
}

export interface StoredContentBlock {
	type: "text" | "tool_use" | "tool_result";
	text?: string;
	id?: string;
	name?: string;
	input?: unknown;
	tool_use_id?: string;
	content?: string;
	is_error?: boolean;
}

export interface StoredMessage {
	role: "user" | "assistant";
	content: StoredContentBlock[];
}
