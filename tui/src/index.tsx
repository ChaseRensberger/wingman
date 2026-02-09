import { createCliRenderer } from "@opentui/core";
import { createRoot, useRenderer, useKeyboard } from "@opentui/react";
import { useState, useEffect, useRef } from "react";

const SERVER_URL = "http://localhost:2323";

interface ToolCall {
	id: string;
	name: string;
	input: string;
	result?: string;
}

interface Message {
	role: "user" | "assistant";
	content: string;
	toolCalls?: ToolCall[];
}

interface ContentBlock {
	Type: string;
	ID: string;
	Name: string;
	Text: string;
	Input: unknown;
}

interface StreamEvent {
	Type: string;
	Text: string;
	InputJSON: string;
	Index: number;
	ContentBlock: ContentBlock | null;
	StopReason: string;
	Usage: unknown;
	Error: string | null;
}

function formatToolInput(input: string): string {
	try {
		const parsed = JSON.parse(input);
		if (parsed.command) return String(parsed.command).slice(0, 60);
		if (parsed.filePath) return String(parsed.filePath);
		if (parsed.pattern) return String(parsed.pattern);
		if (parsed.content) return `${String(parsed.content).slice(0, 40)}...`;
		return input.slice(0, 50);
	} catch {
		return input.slice(0, 50);
	}
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

	if (isUser) {
		return (
			<box marginBottom={1} justifyContent="flex-end">
				<box
					backgroundColor="#1e3a5f"
					paddingLeft={2}
					paddingRight={2}
					paddingTop={1}
					paddingBottom={1}
					borderStyle="rounded"
					border
					borderColor="#3b82f6"
				>
					<text fg="#e2e8f0">{message.content}</text>
				</box>
			</box>
		);
	}

	return (
		<box flexDirection="column" marginBottom={1}>
			{message.toolCalls?.map((tool) => (
				<ToolCallDisplay key={tool.id} tool={tool} />
			))}
			{message.content && <text fg="#e2e8f0">{message.content}</text>}
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
				body: JSON.stringify({ work_dir: "/home/chase/wingman-test" }),
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
		const prompt = input;
		setMessages((prev) => [...prev, userMessage]);
		setInput("");
		setIsStreaming(true);
		setStatus("Thinking...");
		setError(null);

		const assistantMessage: Message = {
			role: "assistant",
			content: "",
			toolCalls: [],
		};
		setMessages((prev) => [...prev, assistantMessage]);

		const currentToolInputs = new Map<number, string>();
		const currentToolIds = new Map<number, string>();
		const currentToolNames = new Map<number, string>();

		try {
			abortRef.current = new AbortController();
			const res = await fetch(
				`${SERVER_URL}/sessions/${sessionId}/message/stream`,
				{
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({ agent_id: agentId, prompt }),
					signal: abortRef.current.signal,
				}
			);

			if (!res.ok) {
				const errText = await res.text();
				throw new Error(errText || `HTTP ${res.status}`);
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

				let currentEventType = "";

				for (const line of lines) {
					if (line.startsWith("event: ")) {
						currentEventType = line.slice(7).trim();
						continue;
					}

					if (!line.startsWith("data: ")) continue;
					const data = line.slice(6);

					try {
						const event: StreamEvent = JSON.parse(data);

						if (
							currentEventType === "text_delta" ||
							event.Type === "text_delta"
						) {
							if (event.Text) {
								setMessages((prev) => {
									const updated = [...prev];
									const last = updated[updated.length - 1];
									if (last) last.content += event.Text;
									return updated;
								});
							}
						} else if (
							currentEventType === "content_block_start" ||
							event.Type === "content_block_start"
						) {
							if (event.ContentBlock?.Type === "tool_use") {
								const toolId = event.ContentBlock.ID || `tool_${event.Index}`;
								const toolName = event.ContentBlock.Name || "unknown";
								currentToolIds.set(event.Index, toolId);
								currentToolNames.set(event.Index, toolName);
								currentToolInputs.set(event.Index, "");

								setStatus(`Using ${toolName}...`);
								setMessages((prev) => {
									const updated = [...prev];
									const last = updated[updated.length - 1];
									if (last) {
										last.toolCalls = [
											...(last.toolCalls || []),
											{
												id: toolId,
												name: toolName,
												input: "",
											},
										];
									}
									return updated;
								});
							}
						} else if (
							currentEventType === "input_json_delta" ||
							event.Type === "input_json_delta"
						) {
							if (event.InputJSON) {
								const existing = currentToolInputs.get(event.Index) || "";
								currentToolInputs.set(event.Index, existing + event.InputJSON);

								const toolId = currentToolIds.get(event.Index);
								if (toolId) {
									setMessages((prev) => {
										const updated = [...prev];
										const last = updated[updated.length - 1];
										if (last?.toolCalls) {
											const tool = last.toolCalls.find((t) => t.id === toolId);
											if (tool) {
												tool.input = currentToolInputs.get(event.Index) || "";
											}
										}
										return updated;
									});
								}
							}
						} else if (
							currentEventType === "tool_result" ||
							event.Type === "tool_result"
						) {
							const toolId = currentToolIds.get(event.Index);
							if (toolId && event.Text) {
								setMessages((prev) => {
									const updated = [...prev];
									const last = updated[updated.length - 1];
									if (last?.toolCalls) {
										const tool = last.toolCalls.find((t) => t.id === toolId);
										if (tool) tool.result = event.Text;
									}
									return updated;
								});
							}
						} else if (
							currentEventType === "message_stop" ||
							event.Type === "message_stop"
						) {
							break;
						} else if (currentEventType === "error" || event.Error) {
							setError(event.Error || event.Text || "Unknown error");
						}
					} catch { }
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
				flexDirection="row"
				justifyContent="space-between"
			>
				<box>
					<text>
						<span fg="#60a5fa">Wingman</span>
						<span fg="#64748b"> | {status}</span>
					</text>
				</box>
				<box>
					<text fg="#64748b">ESC to {isStreaming ? "cancel" : "exit"}</text>
				</box>
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
