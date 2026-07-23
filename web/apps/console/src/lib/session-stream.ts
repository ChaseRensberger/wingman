import { getClientId } from "@/lib/client";

export type SessionEvent = {
	id: string;
	type: string;
	cursor?: {
		session_id: string;
		seq: number;
	};
	data?: Record<string, unknown>;
};

function parseSSE(buffer: string): {
	events: Array<{ event: string; data: string }>;
	remainder: string;
} {
	const events: Array<{ event: string; data: string }> = [];
	const chunks = buffer.split("\n\n");
	const remainder = chunks.pop() ?? "";
	for (const chunk of chunks) {
		const lines = chunk.split("\n");
		let event = "";
		let data = "";
		for (const line of lines) {
			if (line.startsWith("event: ")) {
				event = line.slice(7);
			} else if (line.startsWith("data: ")) {
				data = line.slice(6);
			}
		}
		if (event || data) events.push({ event, data });
	}
	return { events, remainder };
}

export async function* readSSE(response: Response): AsyncGenerator<{ event: string; data: unknown }> {
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
				yield { event: ev.event, data: JSON.parse(ev.data) };
			} catch {
				yield { event: ev.event, data: ev.data };
			}
		}
	}
	if (!buffer.trim()) return;
	for (const ev of parseSSE(buffer + "\n\n").events) {
		try {
			yield { event: ev.event, data: JSON.parse(ev.data) };
		} catch {
			yield { event: ev.event, data: ev.data };
		}
	}
}

export function eventField<T>(data: unknown, lower: string, upper: string): T | undefined {
	if (!data || typeof data !== "object") return undefined;
	const record = data as Record<string, unknown>;
	return (record[lower] ?? record[upper]) as T | undefined;
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

	const headers = new Headers({ "Content-Type": "application/json" });
	const clientId = getClientId();
	if (clientId) headers.set("X-Wingman-Client", clientId);
	const res = await fetch("/run", {
		method: "POST",
		headers,
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
	if (!res.ok) throw new Error(`HTTP ${res.status}: ${await res.text()}`);

	let textBuffer = "";
	for await (const ev of readSSE(res)) {
		if (ev.event === "error") {
			const error = typeof ev.data === "string" ? ev.data : eventField<{ error?: string }>(ev.data, "data", "Data")?.error;
			throw new Error(error || "Title generation failed");
		}
		if (ev.event !== "stream_part") continue;
		const envelope = ev.data as { data?: unknown; Data?: unknown };
		const part = eventField<{ type: string; delta?: string }>(envelope.data ?? envelope.Data, "part", "Part");
		if ((part?.type === "text_delta" || part?.type === "text-delta") && part.delta) {
			textBuffer += part.delta;
			const title = sanitizeGeneratedTitle(textBuffer);
			if (title) onTitle(title);
		}
	}
	return sanitizeGeneratedTitle(textBuffer);
}
