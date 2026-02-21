import { getWingmanUrl } from "./config";

async function request(path: string, options?: RequestInit): Promise<any> {
	const base = getWingmanUrl();
	const res = await fetch(`${base}${path}`, {
		headers: { "Content-Type": "application/json" },
		...options,
	});
	if (!res.ok) {
		const text = await res.text();
		throw new Error(text || `HTTP ${res.status}`);
	}
	return res.json();
}

export const api = {
	health(): Promise<{ status: string }> {
		return request("/health");
	},

	createAgent(opts: {
		name: string;
		instructions: string;
		tools: string[];
		provider?: { id: string; model: string; max_tokens: number; temperature: number | null };
	}): Promise<{ id: string; name?: string }> {
		return request("/agents", {
			method: "POST",
			body: JSON.stringify(opts),
		});
	},

	listAgents(): Promise<Array<{ id: string; name: string }>> {
		return request("/agents");
	},

	createSession(workDir: string): Promise<{ id: string }> {
		return request("/sessions", {
			method: "POST",
			body: JSON.stringify({ work_dir: workDir }),
		});
	},

	setProviderAuth(provider: string, key: string): Promise<any> {
		return request("/provider/auth", {
			method: "PUT",
			body: JSON.stringify({ provider, key }),
		});
	},

	async *streamMessage(
		sessionID: string,
		agentID: string,
		message: string,
		signal?: AbortSignal,
	): AsyncGenerator<{ event: string; data: any }> {
		const base = getWingmanUrl();
		const res = await fetch(`${base}/sessions/${sessionID}/message/stream`, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ agent_id: agentID, message }),
			signal,
		});

		if (!res.ok) {
			const text = await res.text();
			throw new Error(text || `HTTP ${res.status}`);
		}

		if (!res.body) throw new Error("No response body");

		const reader = res.body.getReader();
		const decoder = new TextDecoder();
		let buffer = "";

		while (true) {
			const { done, value } = await reader.read();
			if (done) break;

			buffer += decoder.decode(value, { stream: true });
			const lines = buffer.split("\n");
			buffer = lines.pop() || "";

			let currentEvent = "";
			for (const line of lines) {
				if (line.startsWith("event: ")) {
					currentEvent = line.slice(7).trim();
					continue;
				}
				if (!line.startsWith("data: ")) continue;

				const data = line.slice(6);
				try {
					yield { event: currentEvent, data: JSON.parse(data) };
				} catch {
					// ignore JSON parse errors
				}
			}
		}
	},
};
