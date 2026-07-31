import { APIError, apiErrorFromResponse } from "./client";
import type { CallTrace, Message, PermissionRequest, Usage } from "./types";

export type SessionEventEnvelope<T extends string = string, D = unknown> = {
	id: string;
	type: T;
	time?: string;
	cursor?: {
		session_id: string;
		seq: number;
	};
	data: D;
};

export type RunEventData = {
	run_id: string;
	status?: "queued" | "running" | "completed" | "failed" | "aborted";
	message?: string;
	error_type?: string;
	error_message?: string;
	usage?: Usage;
	steps?: number;
	started_at?: string;
	completed_at?: string;
	updated_at?: string;
};

export type StepEventData = { run_id: string; step: number; usage?: Usage };
export type ContentDeltaEventData = { run_id: string; step?: number; message_id?: string; part_id?: string; revision?: number; call_id?: string; delta: string };
export type ContentCompletedEventData = { run_id: string; message_id?: string; part_id?: string; revision?: number; text: string };
export type MessageCreatedEventData = { run_id: string; message: Message };
export type ToolEventData = {
	run_id: string;
	tool_use_id?: string;
	call_id?: string;
	tool?: string;
	status?: string;
	input?: Record<string, unknown>;
	output?: string;
	output_delta?: string;
	structured?: unknown;
	metadata?: Record<string, unknown>;
	error?: string;
	step?: number;
	ordinal?: number;
	message_id?: string;
	part_id?: string;
	revision?: number;
	model_call_id?: string;
	proposed_at?: string;
	authorized_at?: string;
	started_at?: string;
	completed_at?: string;
	duration_ms?: number;
};

export type SessionEventDataMap = {
	"session.run.queued": RunEventData;
	"session.run.started": RunEventData;
	"session.run.completed": RunEventData;
	"session.run.failed": RunEventData;
	"session.run.aborted": RunEventData;
	"session.step.started": StepEventData;
	"session.step.completed": StepEventData;
	"session.text.delta": ContentDeltaEventData;
	"session.text.completed": ContentCompletedEventData;
	"session.reasoning.delta": ContentDeltaEventData;
	"session.reasoning.completed": ContentCompletedEventData;
	"session.message.created": MessageCreatedEventData;
	"session.tool.called": ToolEventData;
	"session.tool.updated": ToolEventData;
	"session.tool.input.delta": ContentDeltaEventData;
	"session.tool.progress": ToolEventData;
	"session.tool.completed": ToolEventData;
	"session.tool.failed": ToolEventData;
	"session.permission.requested": PermissionRequest;
	"session.permission.resolved": PermissionRequest;
	"session.structured_output.completed": { run_id: string; schema?: string; raw_json: string; parsed: Record<string, unknown> };
	"session.events.synchronized": { cursor: number; watermark: number };
	"session.events.resync_required": { cursor: number; reason: string };
};

export type SessionEventType = keyof SessionEventDataMap;
export type SessionEvent = { [K in SessionEventType]: SessionEventEnvelope<K, SessionEventDataMap[K]> }[SessionEventType];
export type UnknownSessionEvent = SessionEventEnvelope<string, unknown>;
export type ParsedSessionEvent = { known: true; event: SessionEvent } | { known: false; event: UnknownSessionEvent };

const sessionEventTypes = new Set<SessionEventType>([
	"session.run.queued", "session.run.started", "session.run.completed", "session.run.failed", "session.run.aborted",
	"session.step.started", "session.step.completed",
	"session.text.delta", "session.text.completed", "session.reasoning.delta", "session.reasoning.completed",
	"session.message.created", "session.tool.called", "session.tool.updated", "session.tool.input.delta",
	"session.tool.progress", "session.tool.completed", "session.tool.failed",
	"session.permission.requested", "session.permission.resolved", "session.structured_output.completed",
	"session.events.synchronized", "session.events.resync_required",
]);

export function parseSessionEvent(value: unknown): ParsedSessionEvent | undefined {
	if (!value || typeof value !== "object") return;
	const event = value as Record<string, unknown>;
	if (typeof event.id !== "string" || typeof event.type !== "string" || !("data" in event)) return;
	const envelope = event as UnknownSessionEvent;
	if (!sessionEventTypes.has(envelope.type as SessionEventType)) return { known: false, event: envelope };
	if (!envelope.data || typeof envelope.data !== "object" || Array.isArray(envelope.data)) return;
	return { known: true, event: envelope as SessionEvent };
}

export type RunToolCall = {
	call_id: string;
	tool_use_id?: string;
	message_id?: string;
	part_id?: string;
	model_call_id?: string;
	step?: number;
	ordinal?: number;
	proposed_at?: string;
	authorized_at?: string;
	started_at?: string;
	name: string;
	args: Record<string, unknown>;
};

export type RunToolResult = {
	call_id: string;
	tool_use_id?: string;
	status?: string;
	name: string;
	args: Record<string, unknown>;
	output?: string;
	structured?: unknown;
	error?: string;
	error_type?: string;
	metadata?: Record<string, unknown>;
	is_error: boolean;
	duration?: number;
};

export type RunTurn = {
	step: number;
	model_call_id?: string;
	attempt: number;
	provider_request_id?: string;
	assistant: Message;
	results: RunToolResult[];
	usage: Usage;
	started_at: string;
	completed_at: string;
	trace: CallTrace;
	error?: string;
};

export type RunStreamEventDataMap = {
	iteration_start: { step: number };
	iteration_end: { step: number; turn: RunTurn };
	message: { step?: number; message: Message };
	tool_proposed: { call: RunToolCall };
	tool_authorized: { call: RunToolCall };
	tool_start: { call: RunToolCall };
	tool_progress: { call_id: string; tool_use_id?: string; name: string; output_delta?: string; metadata?: Record<string, unknown> };
	tool_end: { result: RunToolResult };
	stream_part: { step: number; message_id?: string; part_id?: string; revision?: number; part: { type: string; delta?: string } };
	compaction: { step: number; phase: string; original_count: number; new_count: number; head?: Message };
	context_transformed: { step: number; phase: string; original_count: number; new_count: number; head?: Message };
	error: { code: string; message: string; details?: Array<{ field?: string; code?: string; message: string }>; request_id?: string };
	structured_output: { schema?: string; raw_json: string; parsed: Record<string, unknown> };
	done: { usage: Usage; steps: number };
};

export type RunStreamEventType = keyof RunStreamEventDataMap;
export type RunStreamEvent = { [K in RunStreamEventType]: { type: K; version: number; data: RunStreamEventDataMap[K] } }[RunStreamEventType];
export type UnknownRunStreamEvent = { type: string; version: number; data: unknown };
export type ParsedRunStreamEvent = { known: true; event: RunStreamEvent } | { known: false; event: UnknownRunStreamEvent };

const runStreamEventTypes = new Set<RunStreamEventType>([
	"iteration_start", "iteration_end", "message", "tool_proposed", "tool_authorized", "tool_start",
	"tool_progress", "tool_end", "stream_part", "compaction", "context_transformed", "error",
	"structured_output", "done",
]);

export function parseRunStreamEvent(value: unknown): ParsedRunStreamEvent | undefined {
	if (!value || typeof value !== "object") return;
	const event = value as Record<string, unknown>;
	if (typeof event.type !== "string" || typeof event.version !== "number" || !("data" in event)) return;
	const envelope = event as UnknownRunStreamEvent;
	if (!runStreamEventTypes.has(envelope.type as RunStreamEventType)) return { known: false, event: envelope };
	if (!envelope.data || typeof envelope.data !== "object" || Array.isArray(envelope.data)) return;
	return { known: true, event: envelope as RunStreamEvent };
}

function parseSSE(buffer: string): {
	events: Array<{ id?: string; event: string; data: string }>;
	remainder: string;
} {
	const events: Array<{ id?: string; event: string; data: string }> = [];
	const chunks = buffer.split("\n\n");
	const remainder = chunks.pop() ?? "";
	for (const chunk of chunks) {
		const lines = chunk.split("\n");
		let id: string | undefined;
		let event = "";
		let data = "";
		for (const line of lines) {
			if (line.startsWith("id:")) {
				id = line.slice(3).replace(/^ /, "");
			} else if (line.startsWith("event: ")) {
				event = line.slice(7);
			} else if (line.startsWith("data: ")) {
				data = line.slice(6);
			}
		}
		if (event || data) events.push({ id, event, data });
	}
	return { events, remainder };
}

export async function* readSSE(response: Response): AsyncGenerator<{ id?: string; event: string; data: unknown }> {
	const reader = response.body!.getReader();
	const decoder = new TextDecoder();
	let buffer = "";
	while (true) {
		const { done, value } = await reader.read();
		if (done) break;
		buffer += decoder.decode(value, { stream: true });
		const { events, remainder } = parseSSE(buffer);
		buffer = remainder;
		for (const ev of events) {
			try {
				yield { id: ev.id, event: ev.event, data: JSON.parse(ev.data) };
			} catch {
				yield { id: ev.id, event: ev.event, data: ev.data };
			}
		}
	}
	if (!buffer.trim()) return;
	for (const ev of parseSSE(buffer + "\n\n").events) {
		try {
			yield { id: ev.id, event: ev.event, data: JSON.parse(ev.data) };
		} catch {
			yield { id: ev.id, event: ev.event, data: ev.data };
		}
	}
}

function sanitizeGeneratedTitle(title: string): string {
	return title
		.replace(/\s+/g, " ")
		.replace(/^[[\]"'`]+|[[\]"'`.!?]+$/g, "")
		.trim()
		.slice(0, 80);
}

export async function generateSessionTitle(
	message: string,
	modelRef: string,
	signal: AbortSignal,
	onTitle: (title: string) => void,
): Promise<string> {
	if (!modelRef) return "";

  const res = await fetch("/run", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
		body: JSON.stringify({
			agent: {
				id: "session_title_generator",
				name: "Session Title Generator",
				instructions: [
					"Generate a concise, specific title for a chat session from the user's first message.",
					"Use 3 to 7 words.",
					"Respond with only the title text.",
					"Do not use JSON, markdown, quotes, labels, or trailing punctuation.",
				].join("\n"),
				tools: [],
			},
			model_ref: modelRef,
			message,
		}),
		signal,
	});
	if (!res.ok) throw await apiErrorFromResponse(res);

	let textBuffer = "";
	let terminal = false;
	for await (const ev of readSSE(res)) {
		const parsed = parseRunStreamEvent(ev.data);
		if (!parsed?.known) continue;
		if (parsed.event.type === "error") {
			const failure = parsed.event.data;
			throw new APIError(0, failure.code || "run_failed", failure.message || "Title generation failed", failure.request_id);
		}
		if (parsed.event.type === "done") {
			terminal = true;
			continue;
		}
		if (parsed.event.type !== "stream_part") continue;
		const part = parsed.event.data.part;
		if ((part?.type === "text_delta" || part?.type === "text-delta") && part.delta) {
			textBuffer += part.delta;
			const title = sanitizeGeneratedTitle(textBuffer);
			if (title) onTitle(title);
		}
	}
	if (!terminal) {
		throw new APIError(0, "run_failed", "Run stream ended without a terminal event", res.headers.get("X-Request-ID") ?? undefined);
	}
	return sanitizeGeneratedTitle(textBuffer);
}
