export interface Message {
	role: "user" | "assistant";
	content: string;
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
