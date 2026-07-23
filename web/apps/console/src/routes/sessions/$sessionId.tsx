import { useEffect, useRef, useState, useCallback } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { wfetch, getClientId } from "@/lib/client";
import { selectGreeting } from "@/lib/greeting";
import { isProviderSelectable } from "@/lib/providers";
import { generateSessionTitle, readSSE, type SessionEvent } from "@/lib/session-stream";
import { showErrorToast } from "@/lib/toast";
import type { Session, Agent, Workspace, Message, ModelCall, Part, Provider, ProviderModel, ToolActivity, ToolCallPart, ToolResultPart, Usage } from "@/lib/types";
import { contextTokenCount, formatContextPercent, formatTokenCount, latestAssistantUsage, splitModelRef } from "@/lib/utils";
import { HexWaveSpinner } from "@/components/hex-wave-spinner";
import { SessionComposer } from "@/components/session-composer";
import { SessionDialogs } from "@/components/session-dialogs";
import { SessionHeader } from "@/components/session-header";
import { SessionTranscript } from "@/components/session-transcript";
import { useTranscriptScroll } from "@/hooks/use-transcript-scroll";

const STREAM_MIN_CHARS_PER_FRAME = 1;
const STREAM_MAX_CHARS_PER_FRAME = 18;
const STREAM_BACKLOG_DIVISOR = 14;
const LAST_AGENT_ID_KEY = "wingman_last_agent_id";
const LAST_MODEL_REF_KEY = "wingman_last_model_ref";
const DEFAULT_SESSION_TITLE = "New session";

type SessionDetailSearch = {
	workspace?: string;
};

type FailedRun = {
	message: string;
	agentId: string;
	modelRef: string;
	error: string;
};

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null && !Array.isArray(value);
}

function buildUserMessage(text: string): Message {
	return {
		role: "user",
		content: [{ type: "text", text } as Part],
	};
}

function cleanReasoningHeading(value: string): string {
	return value
		.replace(/`([^`]+)`/g, "$1")
		.replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
		.replace(/[*_~]+/g, "")
		.replace(/<[^>]+>/g, " ")
		.replace(/\s+/g, " ")
		.trim()
		.slice(0, 120);
}

function reasoningHeading(reasoning: string): string {
	const markdown = reasoning.replace(/\r\n?/g, "\n");
	const heading = markdown.match(/<h[1-6][^>]*>([\s\S]*?)<\/h[1-6]>|^\s{0,3}#{1,6}[ \t]+(.+?)(?:[ \t]+#+[ \t]*)?$/m);
	return cleanReasoningHeading(heading?.[1] ?? heading?.[2] ?? "");
}

async function latestSessionEventSeq(sessionId: string): Promise<number> {
	let after = 0;
	for (; ;) {
		const page = await wfetch(`/sessions/${sessionId}/events/history?after=${after}&limit=500`) as { data?: SessionEvent[]; has_more?: boolean };
		const events = page.data ?? [];
		if (events.length === 0) return after;
		after = events.at(-1)?.cursor?.seq ?? after;
		if (!page.has_more) return after;
	}
}

function modelRefExists(models: Record<string, ProviderModel[]>, modelRef: string): boolean {
	const ref = splitModelRef(modelRef);
	return Boolean(ref.provider && ref.model && models[ref.provider]?.some((model) => model.id === ref.model));
}

function agentExists(agents: Agent[], agentId: string): boolean {
	return Boolean(agentId && agents.some((agent) => agent.id === agentId));
}

function persistLastAgentId(agentId: string) {
	if (agentId) {
		localStorage.setItem(LAST_AGENT_ID_KEY, agentId);
	}
}

function persistLastModelRef(modelRef: string) {
	if (modelRef) {
		localStorage.setItem(LAST_MODEL_REF_KEY, modelRef);
	}
}

function formatSessionError(err: unknown): string {
	const message = String(err instanceof Error ? err.message : err);
	if (message.includes("requires a working directory, but session has none")) {
		return "This session has no working directory. The selected agent tried to use a tool that requires one. Create a new session with a working directory to use this agent.";
	}
	return message.replace(/^Error:\s*/, "");
}

function shouldAutoGenerateTitle(session: Session | null): boolean {
	if (!session || session.history.length > 0) return false;
	const title = (session.title ?? "").trim();
	return title === "" || title === DEFAULT_SESSION_TITLE;
}

export const Route = createFileRoute("/sessions/$sessionId")({
	validateSearch: (search: Record<string, unknown>): SessionDetailSearch => ({
		workspace: typeof search.workspace === "string" ? search.workspace : undefined,
	}),
	component: SessionDetailPage,
});

function SessionDetailPage() {
	const { sessionId } = Route.useParams();
	const { workspace: draftWorkspaceId } = Route.useSearch();
	const navigate = useNavigate();
	const isDraft = sessionId === "new";
	const [greeting] = useState(() => selectGreeting());
	const [session, setSession] = useState<Session | null>(null);
	const [workspace, setWorkspace] = useState<Workspace | null>(null);
	const [loading, setLoading] = useState(true);
	const [agents, setAgents] = useState<Agent[]>([]);
	const [providers, setProviders] = useState<Provider[]>([]);
	const [models, setModels] = useState<Record<string, ProviderModel[]>>({});
	const [modelCalls, setModelCalls] = useState<ModelCall[]>([]);
	const [selectedAgent, setSelectedAgent] = useState("");
	const [selectedProvider, setSelectedProvider] = useState("");
	const [selectedModel, setSelectedModel] = useState("");
	const [messageText, setMessageText] = useState("");
	const [streamingText, setStreamingText] = useState("");
	const [visibleStreamingText, setVisibleStreamingText] = useState("");
	const [streamingReasoning, setStreamingReasoning] = useState("");
	const [streamingTitle, setStreamingTitle] = useState("");
	const [visibleStreamingTitle, setVisibleStreamingTitle] = useState("");
	const [isTitleStreaming, setIsTitleStreaming] = useState(false);
	const [isStreaming, setIsStreaming] = useState(false);
	const [isStreamPaused, setIsStreamPaused] = useState(false);
	const [latestRunUsage, setLatestRunUsage] = useState<Usage | undefined>();
	const [failedRun, setFailedRun] = useState<FailedRun | null>(null);
	const [toolActivities, setToolActivities] = useState<Map<string, ToolActivity>>(() => new Map());
	const [copiedFailedRunError, setCopiedFailedRunError] = useState(false);
	const [editingSession, setEditingSession] = useState(false);
	const [sessionTitleInput, setSessionTitleInput] = useState("");
	const [sessionWorkDirInput, setSessionWorkDirInput] = useState("");
	const [savingSession, setSavingSession] = useState(false);
	const [deleteSessionOpen, setDeleteSessionOpen] = useState(false);
	const [deletingSession, setDeletingSession] = useState(false);
	const [copiedValue, setCopiedValue] = useState<"id" | "path" | "">("");
	const [jsonMode, setJSONMode] = useState(false);
	const abortControllerRef = useRef<AbortController | null>(null);
	const eventControllerRef = useRef<AbortController | null>(null);
	const lastEventSeqRef = useRef(0);
	const activeRunRef = useRef<{ sessionId: string; runId?: string; completed: boolean } | null>(null);
	const retryRequestRef = useRef<Omit<FailedRun, "error"> | null>(null);
	const activeSessionIdRef = useRef(sessionId);
	const skipNextSessionLoadRef = useRef(false);
	const composerRef = useRef<HTMLTextAreaElement>(null);
	const streamingTextRef = useRef("");
	const visibleStreamingTextRef = useRef("");
	const streamingTitleRef = useRef("");
	const visibleStreamingTitleRef = useRef("");
	const titleSessionIdRef = useRef(sessionId);
	const transcriptScroll = useTranscriptScroll(`${session?.id}:${session?.history.length}:${visibleStreamingText.length}:${jsonMode}`, `${loading}:${session?.id}`);

	useEffect(() => {
		activeSessionIdRef.current = sessionId;
		lastEventSeqRef.current = 0;
		activeRunRef.current = null;
		retryRequestRef.current = null;
		transcriptScroll.reset();
		setFailedRun(null);
		setToolActivities(new Map());
		setJSONMode(false);
		eventControllerRef.current?.abort();
		eventControllerRef.current = null;
		setIsStreamPaused(false);
		setStreamingReasoning("");
		if (titleSessionIdRef.current !== sessionId) {
			setStreamingTitle("");
			setVisibleStreamingTitle("");
			setIsTitleStreaming(false);
		}
	}, [sessionId]);

	const loadSession = useCallback(async (id = sessionId) => {
		if (id === "new") {
			const now = new Date().toISOString();
			setSession({
				id: "new",
				title: "New session",
				workspace_id: draftWorkspaceId,
				history: [],
				created_at: now,
				updated_at: now,
			});
			if (draftWorkspaceId) {
				try {
					setWorkspace((await wfetch(`/workspaces/${draftWorkspaceId}`)) as Workspace);
				} catch {
					setWorkspace(null);
				}
			} else {
				setWorkspace(null);
			}
			setLoading(false);
			setModelCalls([]);
			return;
		}

		try {
			const [data, calls] = await Promise.all([
				wfetch(`/sessions/${id}`) as Promise<Session>,
				wfetch(`/sessions/${id}/model-calls`) as Promise<ModelCall[]>,
			]);
			setSession(data);
			setModelCalls(calls);
			if (data.workspace_id) {
				setWorkspace((await wfetch(`/workspaces/${data.workspace_id}`)) as Workspace);
			} else {
				setWorkspace(null);
			}
		} catch (err) {
			console.error("Failed to load session", err);
			showErrorToast(err);
		} finally {
			setLoading(false);
		}
	}, [draftWorkspaceId, sessionId]);

	useEffect(() => {
		if (skipNextSessionLoadRef.current) {
			skipNextSessionLoadRef.current = false;
			return;
		}

		let cancelled = false;
		async function load() {
			try {
				const [sessData, agentsData, providerData, callsData] = await Promise.all([
					isDraft ? Promise.resolve(null) : wfetch(`/sessions/${sessionId}`) as Promise<Session>,
					wfetch("/agents") as Promise<Agent[]>,
					wfetch("/provider") as Promise<Provider[]>,
					isDraft ? Promise.resolve([] as ModelCall[]) : wfetch(`/sessions/${sessionId}/model-calls`) as Promise<ModelCall[]>,
				]);
				const selectableProviders = providerData.filter(isProviderSelectable);
				const modelEntries = await Promise.all(
					selectableProviders.map(async (provider) => {
						try {
							const data = (await wfetch(`/provider/${provider.id}/models`)) as Record<string, ProviderModel>;
							return [provider.id, Object.values(data).sort((a, b) => a.id.localeCompare(b.id))] as const;
						} catch {
							return [provider.id, []] as const;
						}
					}),
				);
				if (!cancelled) {
					if (sessData) {
						setSession(sessData);
					} else {
						const now = new Date().toISOString();
						setSession({
							id: "new",
							title: "New session",
							workspace_id: draftWorkspaceId,
							history: [],
							created_at: now,
							updated_at: now,
						});
					}
					if (sessData?.workspace_id || (isDraft && draftWorkspaceId)) {
						try {
							setWorkspace((await wfetch(`/workspaces/${sessData?.workspace_id ?? draftWorkspaceId}`)) as Workspace);
						} catch {
							setWorkspace(null);
						}
					} else {
						setWorkspace(null);
					}
					const modelMap = Object.fromEntries(modelEntries);
					setAgents(agentsData);
					setProviders(providerData);
					setModels(modelMap);
					setModelCalls(callsData);
					if (agentsData.length > 0) {
						const storedAgentId = localStorage.getItem(LAST_AGENT_ID_KEY) ?? "";
						const initialAgent = agentExists(agentsData, storedAgentId)
							? agentsData.find((agent) => agent.id === storedAgentId)!
							: agentsData[0];
						setSelectedAgent(initialAgent.id);
						const storedModelRef = localStorage.getItem(LAST_MODEL_REF_KEY) ?? "";
						const initialModelRef = modelRefExists(modelMap, storedModelRef)
							? storedModelRef
							: initialAgent.model_ref;
						const modelRef = splitModelRef(initialModelRef);
						setSelectedProvider(modelRef.provider);
						setSelectedModel(modelRef.model);
					}
				}
			} catch (err) {
				console.error("Failed to load session/agents", err);
				showErrorToast(err);
			} finally {
				if (!cancelled) setLoading(false);
			}
		}
		load();
		return () => {
			cancelled = true;
		};
	}, [draftWorkspaceId, isDraft, sessionId]);

	useEffect(() => {
		if (!isDraft || loading || isStreaming || session?.history.length) return;
		const frame = requestAnimationFrame(() => composerRef.current?.focus());
		return () => cancelAnimationFrame(frame);
	}, [isDraft, isStreaming, loading, session?.history.length]);

	useEffect(() => {
		streamingTextRef.current = streamingText;
	}, [streamingText]);

	useEffect(() => {
		visibleStreamingTextRef.current = visibleStreamingText;
	}, [visibleStreamingText]);

	useEffect(() => {
		streamingTitleRef.current = streamingTitle;
	}, [streamingTitle]);

	useEffect(() => {
		visibleStreamingTitleRef.current = visibleStreamingTitle;
	}, [visibleStreamingTitle]);

	useEffect(() => {
		if (!isStreaming && !streamingText) return;

		let frameId = 0;
		const tick = () => {
			const target = streamingTextRef.current;
			const visible = visibleStreamingTextRef.current;

			if (visible.length < target.length) {
				const backlog = target.length - visible.length;
				const charsThisFrame = Math.min(
					STREAM_MAX_CHARS_PER_FRAME,
					Math.max(STREAM_MIN_CHARS_PER_FRAME, Math.ceil(backlog / STREAM_BACKLOG_DIVISOR)),
				);
				const next = target.slice(0, visible.length + charsThisFrame);
				visibleStreamingTextRef.current = next;
				setVisibleStreamingText(next);
			}

			if (isStreaming || visibleStreamingTextRef.current.length < streamingTextRef.current.length) {
				frameId = requestAnimationFrame(tick);
			}
		};

		frameId = requestAnimationFrame(tick);
		return () => cancelAnimationFrame(frameId);
	}, [isStreaming, streamingText]);

	useEffect(() => {
		if (!isTitleStreaming && !streamingTitle) return;

		let frameId = 0;
		const tick = () => {
			const target = streamingTitleRef.current;
			const visible = visibleStreamingTitleRef.current;

			if (visible.length < target.length) {
				const backlog = target.length - visible.length;
				const charsThisFrame = Math.min(
					STREAM_MAX_CHARS_PER_FRAME,
					Math.max(STREAM_MIN_CHARS_PER_FRAME, Math.ceil(backlog / STREAM_BACKLOG_DIVISOR)),
				);
				const next = target.slice(0, visible.length + charsThisFrame);
				visibleStreamingTitleRef.current = next;
				setVisibleStreamingTitle(next);
			}

			if (isTitleStreaming || visibleStreamingTitleRef.current.length < streamingTitleRef.current.length) {
				frameId = requestAnimationFrame(tick);
			}
		};

		frameId = requestAnimationFrame(tick);
		return () => cancelAnimationFrame(frameId);
	}, [isTitleStreaming, streamingTitle]);

	async function copyFailedRunError() {
		if (!failedRun) return;
		try {
			await navigator.clipboard.writeText(failedRun.error);
			setCopiedFailedRunError(true);
			window.setTimeout(() => setCopiedFailedRunError(false), 1200);
		} catch (err) {
			showErrorToast(err, "Could not copy error");
		}
	}

	function applySessionEvent(ev: SessionEvent) {
		if (typeof ev.cursor?.seq === "number" && ev.cursor.seq > lastEventSeqRef.current) {
			lastEventSeqRef.current = ev.cursor.seq;
		}
		const data = ev.data ?? {};
		if (ev.type === "session.tool.updated") {
			const callID = typeof data.call_id === "string" ? data.call_id : "";
			const tool = typeof data.tool === "string" ? data.tool : "";
			const status = data.status;
			if (!callID || !tool || !["pending", "running", "completed", "error"].includes(String(status))) return;
			setToolActivities((previous) => {
				const next = new Map(previous);
				next.set(callID, {
					call_id: callID,
					tool,
					status: status as ToolActivity["status"],
					input: isRecord(data.input) ? data.input : undefined,
					output: typeof data.output === "string" ? data.output : undefined,
					metadata: isRecord(data.metadata) ? data.metadata : undefined,
					error: typeof data.error === "string" ? data.error : undefined,
					started_at: typeof data.started_at === "string" ? data.started_at : undefined,
					completed_at: typeof data.completed_at === "string" ? data.completed_at : undefined,
					duration_ms: typeof data.duration_ms === "number" ? data.duration_ms : undefined,
				});
				return next;
			});
			return;
		}
		if (ev.type === "session.text.delta") {
			const delta = typeof data.delta === "string" ? data.delta : "";
			if (delta) {
				const next = streamingTextRef.current + delta;
				streamingTextRef.current = next;
				setStreamingText(next);
			}
			return;
		}
		if (ev.type === "session.reasoning.delta") {
			const delta = typeof data.delta === "string" ? data.delta : "";
			if (delta) setStreamingReasoning((previous) => previous + delta);
			return;
		}
		if (ev.type === "session.reasoning.completed") {
			const text = typeof data.text === "string" ? data.text : "";
			if (text) setStreamingReasoning(text);
			return;
		}
		if (ev.type === "session.message.created") {
			const message = data.message as Message | undefined;
			if (!message) return;
			setSession((prev) => {
				if (!prev) return prev;
				return { ...prev, history: [...prev.history, message] };
			});
			if (message.role === "assistant") {
				streamingTextRef.current = "";
				visibleStreamingTextRef.current = "";
				setStreamingText("");
				setVisibleStreamingText("");
			}
			return;
		}
		if (ev.type === "session.run.completed") {
			if (activeRunRef.current?.runId && data.run_id !== activeRunRef.current.runId) return;
			const usage = data.usage as Usage | undefined;
			if (usage) setLatestRunUsage(usage);
			activeRunRef.current = activeRunRef.current ? { ...activeRunRef.current, completed: true } : null;
			return;
		}
		if (ev.type === "session.run.failed") {
			if (activeRunRef.current?.runId && data.run_id !== activeRunRef.current.runId) return;
			const error = typeof data.error === "string" ? data.error : "Run failed";
			activeRunRef.current = activeRunRef.current ? { ...activeRunRef.current, completed: true } : null;
			throw new Error(error);
		}
	}

	async function subscribeSessionEvents(sessionId: string, signal: AbortSignal): Promise<void> {
		const headers = new Headers();
		const clientId = getClientId();
		if (clientId) headers.set("X-Wingman-Client", clientId);
		const res = await fetch(`/sessions/${sessionId}/events?after=${lastEventSeqRef.current}`, { headers, signal });
		if (!res.ok) {
			const text = await res.text();
			throw new Error(`HTTP ${res.status}: ${text}`);
		}
		for await (const ev of readSSE(res)) {
			if (!ev.event || ev.event.startsWith(":")) continue;
			applySessionEvent(ev.data as SessionEvent);
			if (activeRunRef.current?.completed) return;
		}
	}

	async function startEventSubscription(sessionId: string) {
		eventControllerRef.current?.abort();
		const controller = new AbortController();
		eventControllerRef.current = controller;
		setIsStreamPaused(false);
		try {
			await subscribeSessionEvents(sessionId, controller.signal);
		} catch (err) {
			if ((err as Error).name !== "AbortError") {
				console.error("Event stream failed", err);
				const request = retryRequestRef.current;
				if (request) setFailedRun({ ...request, error: formatSessionError(err) });
			}
		} finally {
			if (activeRunRef.current?.sessionId === sessionId && activeRunRef.current.completed) {
				setIsStreaming(false);
				setIsStreamPaused(false);
				setStreamingText("");
				setVisibleStreamingText("");
				setStreamingReasoning("");
				activeRunRef.current = null;
				await loadSession(sessionId);
			}
		}
	}

	function handlePauseStream() {
		eventControllerRef.current?.abort();
		eventControllerRef.current = null;
		setIsStreamPaused(true);
	}

	function handleResumeStream() {
		const run = activeRunRef.current;
		if (!run || run.completed) return;
		void startEventSubscription(run.sessionId);
	}

	async function handleAbort() {
		try {
			await wfetch(`/sessions/${activeSessionIdRef.current}/abort`, { method: "POST" });
		} catch (err) {
			console.error("Abort failed", err);
		}
		if (abortControllerRef.current) {
			abortControllerRef.current.abort();
			abortControllerRef.current = null;
		}
		eventControllerRef.current?.abort();
		eventControllerRef.current = null;
		activeRunRef.current = null;
		setIsStreaming(false);
		setIsStreamPaused(false);
		setStreamingText("");
		setVisibleStreamingText("");
		setStreamingReasoning("");
		await loadSession();
	}

	async function handleSend(e?: React.FormEvent, retry?: Omit<FailedRun, "error">) {
		if (e) e.preventDefault();
		const outboundText = retry?.message ?? messageText.trim();
		const outboundAgentId = retry?.agentId ?? selectedAgent;
		const outboundModelRef = retry?.modelRef ?? (selectedProvider && selectedModel ? `${selectedProvider}/${selectedModel}` : "");
		if (!outboundText || !outboundAgentId) return;

		const shouldGenerateTitle = shouldAutoGenerateTitle(session);
		persistLastAgentId(outboundAgentId);
		persistLastModelRef(outboundModelRef);
		setFailedRun(null);
		setMessageText("");
		transcriptScroll.reset();
		setSession((prev) => {
			if (!prev) return prev;
			return { ...prev, history: [...prev.history, buildUserMessage(outboundText)] };
		});

		const controller = new AbortController();
		abortControllerRef.current = controller;
		retryRequestRef.current = { message: outboundText, agentId: outboundAgentId, modelRef: outboundModelRef };
		setIsStreaming(true);
		setIsStreamPaused(false);
		setStreamingText("");
		setVisibleStreamingText("");
		setStreamingReasoning("");
		setLatestRunUsage(undefined);
		let completed = false;
		let activeSessionId = sessionId;
		let titlePromise: Promise<string> | null = null;

		if (shouldGenerateTitle && outboundModelRef) {
			titleSessionIdRef.current = sessionId;
			setIsTitleStreaming(true);
			setStreamingTitle("");
			setVisibleStreamingTitle("");
			titlePromise = generateSessionTitle(outboundText, outboundModelRef, controller.signal, (title) => {
				if (!title) return;
				setStreamingTitle(title);
			}).catch((err) => {
				if ((err as Error).name !== "AbortError") {
					console.warn("Session title generation failed", err);
				}
				return "";
			}).finally(() => {
				setIsTitleStreaming(false);
			});
		}

		const persistGeneratedTitle = (id: string) => {
			if (!titlePromise) return;
			void titlePromise.then(async (title) => {
				if (!title || titleSessionIdRef.current !== id) return;
				try {
					const updated = (await wfetch(`/sessions/${id}`, {
						method: "PUT",
						body: JSON.stringify({ title }),
					})) as Session;
					setSession((prev) => prev && prev.id === id ? { ...prev, title: updated.title } : prev);
				} catch (err) {
					console.warn("Failed to persist generated session title", err);
				}
			});
		};

		try {
			if (isDraft) {
				const created = (await wfetch("/sessions", {
					method: "POST",
					body: JSON.stringify(draftWorkspaceId ? { workspace_id: draftWorkspaceId } : {}),
				})) as Session;
				activeSessionId = created.id;
				activeSessionIdRef.current = created.id;
				if (titlePromise) titleSessionIdRef.current = created.id;
				skipNextSessionLoadRef.current = true;
				setSession({ ...created, history: [buildUserMessage(outboundText)] });
				navigate({ to: "/sessions/$sessionId", params: { sessionId: created.id }, replace: true });
			}
			persistGeneratedTitle(activeSessionId);

			lastEventSeqRef.current = await latestSessionEventSeq(activeSessionId);

			const headers = new Headers({
				"Content-Type": "application/json",
			});
			const clientId = getClientId();
			if (clientId) {
				headers.set("X-Wingman-Client", clientId);
			}

			const res = await fetch(`/sessions/${activeSessionId}/message`, {
				method: "POST",
				headers,
				body: JSON.stringify({
					agent_id: outboundAgentId,
					model_ref: outboundModelRef,
					message: outboundText,
				}),
				signal: controller.signal,
			});

			if (!res.ok) {
				const text = await res.text();
				throw new Error(`HTTP ${res.status}: ${text}`);
			}
			const admitted = await res.json() as { run_id?: string; status?: string };
			if (!admitted.run_id || admitted.status !== "queued") {
				throw new Error("Message was not accepted for execution");
			}
			activeRunRef.current = { sessionId: activeSessionId, runId: admitted.run_id, completed: false };
			void startEventSubscription(activeSessionId);
			completed = true;
		} catch (err) {
			if ((err as Error).name !== "AbortError") {
				console.error("Send failed", err);
				setMessageText(outboundText);
				setFailedRun({ message: outboundText, agentId: outboundAgentId, modelRef: outboundModelRef, error: formatSessionError(err) });
			}
		} finally {
			if (!completed) {
				setIsStreaming(false);
				setIsStreamPaused(false);
				setStreamingText("");
				setVisibleStreamingText("");
				setStreamingReasoning("");
				eventControllerRef.current?.abort();
				eventControllerRef.current = null;
				activeRunRef.current = null;
			}
			abortControllerRef.current = null;
			if (!completed && controller.signal.aborted) {
				setMessageText(outboundText);
			}
			if (!completed) await loadSession(activeSessionId);
		}
	}

	function openEditSession() {
		if (!session) return;
		setSessionTitleInput(session.title ?? "");
		setSessionWorkDirInput(session.work_dir ?? "");
		setEditingSession(true);
	}

	async function handleSaveSession() {
		if (!session || isDraft) return;
		setSavingSession(true);
		try {
			const workingDirectoryChanged = sessionWorkDirInput.trim() !== (session.work_dir ?? "");
			const updated = (await wfetch(`/sessions/${session.id}`, {
				method: "PUT",
				body: JSON.stringify({
					title: sessionTitleInput.trim(),
					...(workingDirectoryChanged ? { working_directory: sessionWorkDirInput.trim() } : {}),
				}),
			})) as Session;
			setSession((prev) => prev && prev.id === updated.id ? { ...prev, ...updated } : prev);
			if (workingDirectoryChanged) setWorkspace(null);
			setEditingSession(false);
		} catch (err) {
			showErrorToast(err);
		} finally {
			setSavingSession(false);
		}
	}

	async function handleDeleteSession() {
		if (!session || isDraft) return;
		setDeletingSession(true);
		try {
			await wfetch(`/sessions/${session.id}`, { method: "DELETE" });
			navigate({ to: "/sessions" });
		} catch (err) {
			showErrorToast(err);
		} finally {
			setDeletingSession(false);
		}
	}

	async function copySessionValue(value: string, kind: "id" | "path") {
		try {
			await navigator.clipboard.writeText(value);
			setCopiedValue(kind);
			window.setTimeout(() => setCopiedValue(""), 1200);
		} catch (err) {
			showErrorToast(err, "Could not copy");
		}
	}

	const selectedAgentName = agents.find((a) => a.id === selectedAgent)?.name;
	const selectableProviders = providers.filter(isProviderSelectable);
	const selectedProviderName = selectableProviders.find((provider) => provider.id === selectedProvider)?.name;
	const selectedModelInfo = (models[selectedProvider] ?? []).find((model) => model.id === selectedModel);
	const hasModels = Object.values(models).some((providerModels) => providerModels.length > 0);
	const latestUsage = latestAssistantUsage(session?.history ?? []) ?? latestRunUsage;
	const persistedCall = session?.latest_model_call;
	const sessionTitle = visibleStreamingTitle || (streamingTitle || isTitleStreaming ? "Generating title..." : session?.title);
	const contextTokens = persistedCall?.context_tokens ?? contextTokenCount(latestUsage);
	const contextWindow = persistedCall?.context_window || selectedModelInfo?.context_window;
	const contextPercent = persistedCall?.context_percent
		? `${Math.round(persistedCall.context_percent)}%`
		: formatContextPercent(contextTokens, contextWindow);
	const contextTokenLabel = contextTokens > 0 ? formatTokenCount(contextTokens) : "0k";
	const contextLabel = contextWindow
		? `${contextTokenLabel} / ${formatTokenCount(contextWindow)} context${contextPercent ? ` (${contextPercent})` : ""}`
		: `${contextTokenLabel} context`;
	const toolCallsById = new Map<string, ToolCallPart>();
	const toolResultsById = new Map<string, ToolResultPart>();
	for (const msg of session?.history ?? []) {
		for (const part of msg.content) {
			if (part.type === "tool_call") {
				const toolCall = part as ToolCallPart;
				toolCallsById.set(toolCall.call_id, toolCall);
			} else if (part.type === "tool_result") {
				const toolResult = part as ToolResultPart;
				toolResultsById.set(toolResult.call_id, toolResult);
			}
		}
	}
	const transcriptHistory = (session?.history ?? []).filter((message) => message.role !== "tool");
	const liveReasoningHeading = reasoningHeading(streamingReasoning);

	if (loading) {
		return (
			<div className="flex items-center gap-3 px-4 py-6 text-sm text-muted-foreground">
				<HexWaveSpinner size={24} />
				<span>Loading...</span>
			</div>
		);
	}

	if (!session) {
		return <div className="px-4 py-6 text-sm text-muted-foreground">Session not found.</div>;
	}

	return (
		<div className="relative flex h-full min-h-0 flex-col bg-background">
			<SessionHeader session={session} workspace={workspace} calls={modelCalls} isDraft={isDraft} title={sessionTitle} contextLabel={contextLabel} jsonMode={jsonMode} copiedValue={copiedValue} onJsonModeChange={() => setJSONMode((value) => !value)} onCopy={(value, kind) => void copySessionValue(value, kind)} onEdit={openEditSession} onDelete={() => setDeleteSessionOpen(true)} />
			<SessionTranscript messages={transcriptHistory} rawMessages={session.history} jsonMode={jsonMode} greeting={greeting} streamingText={visibleStreamingText} isStreaming={isStreaming} reasoningHeading={liveReasoningHeading} toolCallsById={toolCallsById} toolResultsById={toolResultsById} toolActivitiesById={toolActivities} failedRun={failedRun} copiedFailedRunError={copiedFailedRunError} onCopyFailedRunError={() => void copyFailedRunError()} onRetry={() => void handleSend(undefined, failedRun ?? undefined)} scroll={transcriptScroll} />
			<SessionComposer composerRef={composerRef} messageText={messageText} selectedAgent={selectedAgent} selectedAgentName={selectedAgentName} selectedProvider={selectedProvider} selectedModel={selectedModel} selectedProviderName={selectedProviderName} agents={agents} providers={selectableProviders} models={models} hasModels={hasModels} isStreaming={isStreaming} isStreamPaused={isStreamPaused} isNearTranscriptBottom={transcriptScroll.isNearBottom} onMessageChange={setMessageText} onAgentChange={(agentId) => { setSelectedAgent(agentId); persistLastAgentId(agentId); }} onModelChange={(modelRef) => { const ref = splitModelRef(modelRef); setSelectedProvider(ref.provider); setSelectedModel(ref.model); persistLastModelRef(modelRef); }} onSubmit={() => void handleSend()} onPause={handlePauseStream} onResume={handleResumeStream} onAbort={handleAbort} onJumpToBottom={transcriptScroll.jumpToBottom} />
			<SessionDialogs sessionTitle={session.title ?? ""} sessionId={session.id} editing={editingSession} saving={savingSession} deleteOpen={deleteSessionOpen} deleting={deletingSession} titleInput={sessionTitleInput} workDirInput={sessionWorkDirInput} onEditingChange={setEditingSession} onDeleteOpenChange={setDeleteSessionOpen} onTitleChange={setSessionTitleInput} onWorkDirChange={setSessionWorkDirInput} onSave={() => void handleSaveSession()} onDelete={() => void handleDeleteSession()} />
		</div>
	);
}
