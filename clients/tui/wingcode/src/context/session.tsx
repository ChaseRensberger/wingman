import { createContext, useContext, useMemo, useRef, useState } from "react";
import { api } from "../api";
import type { Message, DoneEvent, StreamEvent, StoredMessage, ToolMessage } from "../types";

interface SessionState {
	messages: Message[];
	isStreaming: boolean;
	status: string;
	error: string | null;
	sendMessage: (text: string) => Promise<void>;
}

const SessionContext = createContext<SessionState | null>(null);

function formatJSON(value: unknown) {
	if (value === undefined || value === null) return "";
	try {
		return JSON.stringify(value, null, 2);
	} catch {
		return String(value);
	}
}

function buildMessagesFromHistory(history: StoredMessage[]): Message[] {
	const result: Message[] = [];
	const toolIndexByID = new Map<string, number>();

	for (const msg of history) {
		for (const block of msg.content || []) {
			if (block.type === "text") {
				result.push({ role: msg.role, content: block.text || "" });
				continue;
			}

			if (block.type === "tool_use") {
				const toolMessage: ToolMessage = {
					role: "tool",
					toolID: block.id,
					toolName: block.name,
					input: formatJSON(block.input),
					status: "running",
				};
				result.push(toolMessage);
				if (block.id) {
					toolIndexByID.set(block.id, result.length - 1);
				}
				continue;
			}

			if (block.type === "tool_result") {
				const toolID = block.tool_use_id || "";
				const existingIndex = toolID ? toolIndexByID.get(toolID) : undefined;
				if (existingIndex !== undefined) {
					const existing = result[existingIndex];
					if (existing?.role === "tool") {
						existing.output = block.content || "";
						existing.status = block.is_error ? "error" : "done";
					}
				} else {
					result.push({
						role: "tool",
						toolID: block.tool_use_id,
						output: block.content || "",
						status: block.is_error ? "error" : "done",
					});
				}
			}
		}
	}

	return result;
}

export function SessionProvider(props: {
	children: React.ReactNode;
	agentID: string;
	sessionID: string;
}) {
	const [messages, setMessages] = useState<Message[]>([]);
	const [isStreaming, setIsStreaming] = useState(false);
	const [status, setStatus] = useState("Ready");
	const [error, setError] = useState<string | null>(null);
	const abortRef = useRef<AbortController | null>(null);
	const toolIndexByIDRef = useRef<Record<string, number>>({});
	const toolIDByBlockIndexRef = useRef<Record<number, string>>({});
	const toolInputByIDRef = useRef<Record<string, string>>({});

	const sendMessage = async (text: string) => {
		if (!text.trim() || isStreaming) return;
		setError(null);
		setIsStreaming(true);
		setStatus("Thinking...");
		toolIndexByIDRef.current = {};
		toolIDByBlockIndexRef.current = {};
		toolInputByIDRef.current = {};

		const userMessage: Message = { role: "user", content: text };
		const assistantMessage: Message = { role: "assistant", content: "" };
		setMessages((prev) => [...prev, userMessage, assistantMessage]);

		abortRef.current?.abort();
		abortRef.current = new AbortController();

		const syncHistory = async () => {
			try {
				const session = await api.getSession(props.sessionID);
				const history = (session.history || []) as StoredMessage[];
				setMessages(buildMessagesFromHistory(history));
			} catch (err) {
				setError(err instanceof Error ? err.message : "Failed to load session history");
			}
		};

		try {
			for await (const { event, data } of api.streamMessage(
				props.sessionID,
				props.agentID,
				text,
				abortRef.current.signal,
			)) {
				const payload = data as StreamEvent;
				const type = event || payload.type || "";

				if (type === "text_delta") {
					const textDelta = payload.text;
					if (textDelta) {
						setMessages((prev) => {
							const updated = [...prev];
							for (let i = updated.length - 1; i >= 0; i -= 1) {
								const message = updated[i];
								if (message?.role === "assistant") {
									message.content += textDelta;
									break;
								}
							}
							return updated;
						});
					}
				} else if (type === "message_start") {
					setMessages((prev) => {
						if (prev[prev.length - 1]?.role === "assistant") return prev;
						return [...prev, { role: "assistant", content: "" }];
					});
				} else if (type === "content_block_start" && payload.content_block?.type === "tool_use") {
					const toolID = payload.content_block.id || `tool_${payload.index ?? Date.now()}`;
					const toolName = payload.content_block.name || "tool";
					if (payload.index !== undefined) {
						toolIDByBlockIndexRef.current[payload.index] = toolID;
					}
					setMessages((prev) => {
						const next: Message[] = [...prev];
						const toolMessage: ToolMessage = {
							role: "tool",
							toolID,
							toolName,
							status: "running",
						};
						next.push(toolMessage);
						toolIndexByIDRef.current[toolID] = next.length - 1;
						return next;
					});
				} else if (type === "input_json_delta" && payload.index !== undefined) {
					const toolID = toolIDByBlockIndexRef.current[payload.index];
					if (!toolID) continue;
					const nextInput = (toolInputByIDRef.current[toolID] || "") + (payload.input_json || "");
					toolInputByIDRef.current[toolID] = nextInput;
					setMessages((prev) => {
						const updated = [...prev];
						const index = toolIndexByIDRef.current[toolID];
						if (index === undefined) return updated;
						const message = updated[index];
						if (message?.role === "tool") {
							message.input = nextInput;
						}
						return updated;
					});
				} else if (type === "done") {
					const done = payload as DoneEvent;
					if (done.usage) {
						setStatus("Ready");
					}
					void syncHistory();
				} else if (type === "error" || payload.error) {
					const raw = typeof data === "string" ? data : "";
					setError(payload.error || raw || "Request failed");
				}
			}
		} catch (e) {
			if ((e as Error).name !== "AbortError") {
				setError(e instanceof Error ? e.message : "Request failed");
			}
		} finally {
			setIsStreaming(false);
			setStatus("Ready");
			abortRef.current = null;
		}
	};

	const value = useMemo(
		() => ({ messages, isStreaming, status, error, sendMessage }),
		[messages, isStreaming, status, error],
	);

	return <SessionContext.Provider value={value}>{props.children}</SessionContext.Provider>;
}

export function useSession() {
	const ctx = useContext(SessionContext);
	if (!ctx) throw new Error("useSession must be used within SessionProvider");
	return ctx;
}
