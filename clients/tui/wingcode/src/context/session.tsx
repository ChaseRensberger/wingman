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

function buildMessagesFromHistory(history: StoredMessage[]): Message[] {
	const result: Message[] = [];

	for (const msg of history) {
		for (const block of msg.content || []) {
			if (block.type === "text") {
				result.push({ role: msg.role, content: block.text || "" });
				continue;
			}

			if (block.type === "tool_use") {
				const toolMessage: ToolMessage = {
					role: "tool",
					toolName: block.name,
					status: "done",
				};
				result.push(toolMessage);
				continue;
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

	const sendMessage = async (text: string) => {
		if (!text.trim() || isStreaming) return;
		setError(null);
		setIsStreaming(true);
		setStatus("Thinking...");

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
				} else if (type === "content_block_start") {
					const block = payload.content_block;
					if (block?.type === "tool_use") {
						setStatus("Using tool...");
						setMessages((prev) => [
							...prev,
							{ role: "tool" as const, toolName: block.name, status: "running" as const },
						]);
					} else if (block?.type === "text") {
						// New text block after a tool call — ensure there's an assistant message to append to
						setMessages((prev) => {
							const last = prev[prev.length - 1];
							if (last?.role === "assistant") return prev;
							return [...prev, { role: "assistant", content: "" }];
						});
					}
				} else if (type === "content_block_stop") {
					// Mark the most recent running tool as done
					setMessages((prev) => {
						const updated = [...prev];
						for (let i = updated.length - 1; i >= 0; i -= 1) {
							const msg = updated[i];
							if (msg?.role === "tool" && msg.status === "running") {
								updated[i] = { ...msg, status: "done" };
								break;
							}
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
