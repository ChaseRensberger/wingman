export interface Message {
	role: "user" | "assistant";
	content: string;
}

export interface StreamEvent {
	Type?: string;
	Text?: string;
	InputJSON?: string;
	Index?: number;
	ContentBlock?: {
		Type?: string;
		ID?: string;
		Name?: string;
	};
	StopReason?: string;
	Error?: string;
}

export interface DoneEvent {
	usage?: { InputTokens: number; OutputTokens: number };
	steps?: number;
}
