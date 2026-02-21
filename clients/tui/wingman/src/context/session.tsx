import { createContext, useContext, useMemo, useRef, useState } from "react";
import { api } from "../api";
import type { Message, DoneEvent, StreamEvent } from "../types";

interface SessionState {
	messages: Message[];
	isStreaming: boolean;
	status: string;
	error: string | null;
	sendMessage: (text: string) => Promise<void>;
}

const SessionContext = createContext<SessionState | null>(null);

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

		try {
			for await (const { event, data } of api.streamMessage(
				props.sessionID,
				props.agentID,
				text,
				abortRef.current.signal,
			)) {
				const type = event || (data as StreamEvent).Type || "";

				if (type === "text_delta") {
					const textDelta = (data as StreamEvent).Text || data.Text;
					if (textDelta) {
						setMessages((prev) => {
							const updated = [...prev];
							const last = updated[updated.length - 1];
							if (last?.role === "assistant") {
								last.content += textDelta;
							}
							return updated;
						});
					}
				} else if (type === "done") {
					const done = data as DoneEvent;
					if (done.usage) {
						setStatus("Ready");
					}
				} else if (type === "error" || (data as StreamEvent).Error) {
					setError((data as StreamEvent).Error || data.Text || "Request failed");
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
