import type { RunStreamEvent, SessionEvent } from "@wingman-actor/client";

import { APIError, apiErrorFromResponse } from "./client";

export type SessionEventEnvelope<T extends string = string, D = unknown> = {
	id: string;
	type: T;
	schema_version?: number;
	time?: string;
	cursor?: {
		session_id: string;
		seq: number;
	};
	data: D;
};

export type SessionEventType = SessionEvent["type"];
export type { SessionEvent };
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
	if (typeof event.id !== "string" || typeof event.type !== "string" || event.schema_version !== 1 || !("data" in event)) return;
	const envelope = event as UnknownSessionEvent;
	if (!sessionEventTypes.has(envelope.type as SessionEventType)) return { known: false, event: envelope };
	if (!envelope.data || typeof envelope.data !== "object" || Array.isArray(envelope.data)) return;
	return { known: true, event: envelope as SessionEvent };
}

export type RunStreamEventType = RunStreamEvent["type"];
export type { RunStreamEvent };
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
		if (!part || typeof part !== "object" || Array.isArray(part)) continue;
		const fields = part as Record<string, unknown>;
		if ((fields.type === "text_delta" || fields.type === "text-delta") && typeof fields.delta === "string") {
			textBuffer += fields.delta;
			const title = sanitizeGeneratedTitle(textBuffer);
			if (title) onTitle(title);
		}
	}
	if (!terminal) {
		throw new APIError(0, "run_failed", "Run stream ended without a terminal event", res.headers.get("X-Request-ID") ?? undefined);
	}
	return sanitizeGeneratedTitle(textBuffer);
}
