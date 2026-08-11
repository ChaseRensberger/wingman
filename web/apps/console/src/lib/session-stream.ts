import { APIError } from "@wingman-actor/client";
import { client } from "@/lib/client";

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

	let textBuffer = "";
	let terminal = false;
	for await (const parsed of client.run.stream({
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
		}, { signal })) {
		if (!parsed.known) continue;
		const event = parsed.event;
		if (event.type === "error") {
			const failure = event.data;
			throw new APIError(0, failure.code || "run_failed", failure.message || "Title generation failed", failure.request_id);
		}
		if (event.type === "done") {
			terminal = true;
			continue;
		}
		if (event.type !== "stream_part") continue;
		const part = event.data.part;
		if (!part || typeof part !== "object" || Array.isArray(part)) continue;
		const fields = part as Record<string, unknown>;
		if ((fields.type === "text_delta" || fields.type === "text-delta") && typeof fields.delta === "string") {
			textBuffer += fields.delta;
			const title = sanitizeGeneratedTitle(textBuffer);
			if (title) onTitle(title);
		}
	}
	if (!terminal) {
		throw new APIError(0, "run_failed", "Run stream ended without a terminal event");
	}
	return sanitizeGeneratedTitle(textBuffer);
}
