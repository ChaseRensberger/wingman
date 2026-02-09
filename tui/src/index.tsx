import { createCliRenderer } from "@opentui/core";
import { createRoot, useRenderer, useKeyboard } from "@opentui/react";
import { useState, useEffect, useRef } from "react";

const SERVER_URL = "http://localhost:2323";

interface ToolCall {
	id: string;
	name: string;
	input: Record<string, unknown>;
	result?: string;
}

interface Message {
	role: "user" | "assistant";
	content: string;
	toolCalls?: ToolCall[];
}

interface StreamEvent {
	type: "text" | "tool_use" | "tool_result" | "done" | "error";
	content?: string;
	tool_use_id?: string;
	name?: string;
	input?: Record<string, unknown>;
	result?: string;
}

function formatToolInput(input: Record<string, unknown>): string {
	if (input.command) return String(input.command).slice(0, 60);
	if (input.filePath) return String(input.filePath);
	if (input.pattern) return String(input.pattern);
	if (input.content) return `${String(input.content).slice(0, 40)}...`;
	return JSON.stringify(input).slice(0, 50);
}

function ToolCallDisplay({ tool }: { tool: ToolCall }) {
	const statusColor = tool.result !== undefined ? "#4ade80" : "#fbbf24";
	const icon = tool.result !== undefined ? "✓" : "⋯";

	return (
		<box flexDirection="column" marginBottom={1}>
			<text>
				<span fg={statusColor}>{icon}</span>
				<span fg="#94a3b8"> {tool.name}</span>
				<span fg="#64748b"> {formatToolInput(tool.input)}</span>
			</text>
			{tool.result && (
				<box marginLeft={2}>
					<text fg="#64748b">
						{tool.result.slice(0, 100)}
						{tool.result.length > 100 ? "..." : ""}
					</text>
				</box>
			)}
		</box>
	);
}

function MessageBubble({ message }: { message: Message }) {
	const isUser = message.role === "user";

	return (
		<box flexDirection="column" marginBottom={1}>
			<text fg={isUser ? "#60a5fa" : "#a78bfa"}>
				{isUser ? "You" : "Assistant"}
			</text>
			<box marginLeft={2} marginTop={0}>
				{message.toolCalls?.map((tool) => (
					<ToolCallDisplay key={tool.id} tool={tool} />
				))}
				{message.content && <text fg="#e2e8f0">{message.content}</text>}
			</box>
		</box>
	);
}

function App() {
	const renderer = useRenderer();
	const [input, setInput] = useState("");
	const [messages, setMessages] = useState<Message[]>([]);
	const [isStreaming, setIsStreaming] = useState(false);
	const [sessionId, setSessionId] = useState<string | null>(null);
	const [agentId, setAgentId] = useState<string | null>(null);
	const [error, setError] = useState<string | null>(null);
	const [status, setStatus] = useState("Initializing...");
	const abortRef = useRef<AbortController | null>(null);

	useEffect(() => {
		initSession();
	}, []);

	async function initSession() {
		try {
			setStatus("Creating agent...");
			const agentRes = await fetch(`${SERVER_URL}/agents`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({
					name: "CodeAssistant",
					instructions:
						"You are a helpful coding assistant. Be concise. When writing code, use the write tool. When editing, use the edit tool. Always show what you're doing.",
					tools: ["bash", "read", "write", "edit", "glob", "grep"],
					max_tokens: 4096,
					max_steps: 20,
				}),
			});
			if (!agentRes.ok) throw new Error("Failed to create agent");
			const agent = await agentRes.json();
			setAgentId(agent.id);

			setStatus("Creating session...");
			const sessionRes = await fetch(`${SERVER_URL}/sessions`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ work_dir: process.cwd() }),
			});
			if (!sessionRes.ok) throw new Error("Failed to create session");
			const session = await sessionRes.json();
			setSessionId(session.id);
			setStatus("Ready");
		} catch (e) {
			setError(e instanceof Error ? e.message : "Setup failed");
			setStatus("Error");
		}
	}

	async function sendMessage() {
		if (!input.trim() || !sessionId || !agentId || isStreaming) return;

		const userMessage: Message = { role: "user", content: input };
		setMessages((prev) => [...prev, userMessage]);
		setInput("");
		setIsStreaming(true);
		setStatus("Thinking...");

		const assistantMessage: Message = {
			role: "assistant",
			content: "",
			toolCalls: [],
		};
		setMessages((prev) => [...prev, assistantMessage]);

		try {
			abortRef.current = new AbortController();
			const res = await fetch(
				`${SERVER_URL}/sessions/${sessionId}/message/stream`,
				{
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({ agent_id: agentId, prompt: input }),
					signal: abortRef.current.signal,
				}
			);

			if (!res.ok) throw new Error("Request failed");
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

				for (const line of lines) {
					if (!line.startsWith("data: ")) continue;
					const data = line.slice(6);
					if (data === "[DONE]") continue;

					try {
						const event: StreamEvent = JSON.parse(data);

						if (event.type === "text" && event.content) {
							setMessages((prev) => {
								const updated = [...prev];
								const last = updated[updated.length - 1];
								if (last) last.content += event.content;
								return updated;
							});
						} else if (event.type === "tool_use") {
							setStatus(`Using ${event.name}...`);
							setMessages((prev) => {
								const updated = [...prev];
								const last = updated[updated.length - 1];
								if (last && event.tool_use_id) {
									last.toolCalls = [
										...(last.toolCalls || []),
										{
											id: event.tool_use_id,
											name: event.name || "unknown",
											input: event.input || {},
										},
									];
								}
								return updated;
							});
						} else if (event.type === "tool_result") {
							setMessages((prev) => {
								const updated = [...prev];
								const last = updated[updated.length - 1];
								if (last?.toolCalls && event.tool_use_id) {
									const tool = last.toolCalls.find(
										(t) => t.id === event.tool_use_id
									);
									if (tool) tool.result = event.result || "";
								}
								return updated;
							});
						} else if (event.type === "error") {
							setError(event.content || "Unknown error");
						}
					} catch {}
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
	}

	useKeyboard((key) => {
		if (key.name === "escape") {
			if (isStreaming && abortRef.current) {
				abortRef.current.abort();
			} else {
				renderer.destroy();
			}
		}
		if (key.name === "return" && !isStreaming) {
			sendMessage();
		}
	});

	return (
		<box flexDirection="column" width="100%" height="100%">
			<box
				height={1}
				backgroundColor="#1e293b"
				paddingLeft={1}
				paddingRight={1}
				justifyContent="space-between"
			>
				<text>
					<span fg="#60a5fa">Wingman</span>
					<span fg="#64748b"> | {status}</span>
				</text>
				<text fg="#64748b">ESC to {isStreaming ? "cancel" : "exit"}</text>
			</box>

			{error && (
				<box backgroundColor="#7f1d1d" padding={1}>
					<text fg="#fecaca">{error}</text>
				</box>
			)}

			<scrollbox flexGrow={1} padding={1} focused={!isStreaming}>
				{messages.length === 0 ? (
					<box justifyContent="center" alignItems="center" flexGrow={1}>
						<text fg="#64748b">
							Ask me to write code, edit files, or run commands...
						</text>
					</box>
				) : (
					messages.map((msg, i) => <MessageBubble key={i} message={msg} />)
				)}
			</scrollbox>

			<box
				height={3}
				backgroundColor="#1e293b"
				padding={1}
				borderTop
				borderColor="#334155"
			>
				<input
					value={input}
					onChange={setInput}
					placeholder={
						isStreaming ? "Waiting for response..." : "Type a message..."
					}
					focused={!isStreaming}
					flexGrow={1}
					textColor="#e2e8f0"
					backgroundColor="#1e293b"
					focusedBackgroundColor="#1e293b"
					placeholderColor="#64748b"
				/>
			</box>
		</box>
	);
}

const renderer = await createCliRenderer();
createRoot(renderer).render(<App />);
