import { useEffect, useRef, useState, useCallback } from "react";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { wfetch, getClientId } from "@/lib/client";
import { selectGreeting } from "@/lib/greeting";
import { isProviderSelectable } from "@/lib/providers";
import { showErrorToast } from "@/lib/toast";
import type { Session, Agent, Workspace, Message, ModelCall, Part, Provider, ProviderModel, ToolActivity, ToolCallPart, ToolResultPart, Usage } from "@/lib/types";
import { contextTokenCount, formatContextPercent, formatTokenCount, latestAssistantUsage, splitModelRef } from "@/lib/utils";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@wingman/core/components/core/alert-dialog";
import { Button } from "@wingman/core/components/core/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@wingman/core/components/core/dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuGroup,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@wingman/core/components/core/dropdown-menu";
import { Input } from "@wingman/core/components/core/input";
import { Textarea } from "@wingman/core/components/core/textarea";
import {
	Select,
	SelectTrigger,
	SelectValue,
	SelectContent,
	SelectGroup,
	SelectLabel,
	SelectItem,
} from "@wingman/core/components/core/select";
import { ChatMessage } from "@/components/chat-message";
import { HexWaveSpinner } from "@/components/hex-wave-spinner";
import { RawMessages } from "@/components/raw-messages";
import { SessionContextSheet } from "@/components/session-context-sheet";
import {
	ArrowDownIcon,
	ArrowClockwiseIcon,
	ArrowLeftIcon,
	CheckIcon,
	ClipboardTextIcon,
	CodeIcon,
	ClockIcon,
	CopyIcon,
	DotsThreeIcon,
	FolderIcon,
	PauseIcon,
	PaperPlaneIcon,
	PencilSimpleIcon,
	PlayIcon,
	StopIcon,
	TrashIcon,
	WarningCircleIcon,
} from "@phosphor-icons/react";

const STREAM_MIN_CHARS_PER_FRAME = 1;
const STREAM_MAX_CHARS_PER_FRAME = 18;
const STREAM_BACKLOG_DIVISOR = 14;
const LAST_AGENT_ID_KEY = "wingman_last_agent_id";
const LAST_MODEL_REF_KEY = "wingman_last_model_ref";
const DEFAULT_SESSION_TITLE = "New session";

type SessionDetailSearch = {
	workspace?: string;
};

type SessionEvent = {
	id: string;
	type: string;
	cursor?: {
		session_id: string;
		seq: number;
	};
	data?: Record<string, unknown>;
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
		if (event || data) {
			events.push({ event, data });
		}
	}
	return { events, remainder };
}

async function* readSSE(
	response: Response,
): AsyncGenerator<{ event: string; data: unknown }> {
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
	if (buffer.trim()) {
		const { events } = parseSSE(buffer + "\n\n");
		for (const ev of events) {
			try {
				yield { event: ev.event, data: JSON.parse(ev.data) };
			} catch {
				yield { event: ev.event, data: ev.data };
			}
		}
	}
}

function buildStreamingMessage(text: string): Message {
	return {
		role: "assistant",
		content: [{ type: "text", text } as Part],
	};
}

function buildUserMessage(text: string): Message {
	return {
		role: "user",
		content: [{ type: "text", text } as Part],
	};
}

function sanitizeGeneratedTitle(title: string): string {
	return title
		.replace(/\s+/g, " ")
		.replace(/^[[\]"'`]+|[[\]"'`.!?]+$/g, "")
		.trim()
		.slice(0, 80);
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

function ThinkingIndicator({ summary }: { summary: string }) {
	return (
		<div className="flex min-h-5 items-center gap-2 px-4 py-2 text-sm font-medium text-muted-foreground">
			<HexWaveSpinner size={16} className="size-3.5 shrink-0" label="Thinking" />
			<span>Thinking</span>
			{summary && <span className="min-w-0 truncate font-normal text-muted-foreground/80">{summary}</span>}
		</div>
	);
}

function eventField<T>(data: unknown, lower: string, upper: string): T | undefined {
	if (!data || typeof data !== "object") return undefined;
	const record = data as Record<string, unknown>;
	return (record[lower] ?? record[upper]) as T | undefined;
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

async function generateSessionTitle(
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

	if (!res.ok) {
		const text = await res.text();
		throw new Error(`HTTP ${res.status}: ${text}`);
	}

	let textBuffer = "";
	let finalTitle = "";
	for await (const ev of readSSE(res)) {
		if (ev.event === "error") {
			const message =
				typeof ev.data === "string"
					? ev.data
					: eventField<{ error?: string }>(ev.data, "data", "Data")?.error;
			throw new Error(message || "Title generation failed");
		}
		if (ev.event === "stream_part") {
			const envelope = ev.data as { data?: unknown; Data?: unknown };
			const data = envelope.data ?? envelope.Data;
			const part = eventField<{ type: string; delta?: string }>(data, "part", "Part");
			if ((part?.type === "text_delta" || part?.type === "text-delta") && part.delta) {
				textBuffer += part.delta;
				const title = sanitizeGeneratedTitle(textBuffer);
				if (title) onTitle(title);
			}
		}
	}

	finalTitle = sanitizeGeneratedTitle(textBuffer);
	return finalTitle;
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
	const [isNearTranscriptBottom, setIsNearTranscriptBottom] = useState(true);
	const [isTranscriptHovered, setIsTranscriptHovered] = useState(false);
	const [isTranscriptScrolling, setIsTranscriptScrolling] = useState(false);
	const [isTranscriptScrollbarDragging, setIsTranscriptScrollbarDragging] = useState(false);
	const [transcriptScrollbar, setTranscriptScrollbar] = useState({ height: 0, top: 0 });
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
	const activeRunRef = useRef<{ sessionId: string; completed: boolean } | null>(null);
	const retryRequestRef = useRef<Omit<FailedRun, "error"> | null>(null);
	const activeSessionIdRef = useRef(sessionId);
	const skipNextSessionLoadRef = useRef(false);
	const scrollRef = useRef<HTMLDivElement>(null);
	const scrollFrameRef = useRef<number | null>(null);
	const scrollbarIdleTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const scrollbarDragRef = useRef<{ pointerId: number; grabOffset: number } | null>(null);
	const stickToBottomRef = useRef(true);
	const streamingTextRef = useRef("");
	const visibleStreamingTextRef = useRef("");
	const streamingTitleRef = useRef("");
	const visibleStreamingTitleRef = useRef("");
	const titleSessionIdRef = useRef(sessionId);

	useEffect(() => {
		activeSessionIdRef.current = sessionId;
		lastEventSeqRef.current = 0;
		activeRunRef.current = null;
		retryRequestRef.current = null;
		stickToBottomRef.current = true;
		setIsNearTranscriptBottom(true);
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
		if (scrollRef.current && stickToBottomRef.current) {
			scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
		}
		updateTranscriptScrollbar();
	}, [session?.history, visibleStreamingText]);

	useEffect(() => {
		const el = scrollRef.current;
		if (!el) return;
		const observer = new ResizeObserver(updateTranscriptScrollbar);
		observer.observe(el);
		if (el.firstElementChild instanceof HTMLElement) observer.observe(el.firstElementChild);
		window.addEventListener("resize", updateTranscriptScrollbar);
		updateTranscriptScrollbar();
		return () => {
			observer.disconnect();
			window.removeEventListener("resize", updateTranscriptScrollbar);
		};
	}, [loading, session?.id]);

	useEffect(() => () => {
		if (scrollFrameRef.current) cancelAnimationFrame(scrollFrameRef.current);
		if (scrollbarIdleTimeoutRef.current) clearTimeout(scrollbarIdleTimeoutRef.current);
	}, []);

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

	function updateTranscriptScrollbar() {
		const el = scrollRef.current;
		if (!el) return;
		const trackPadding = 8;
		const trackHeight = el.clientHeight - trackPadding * 2;
		if (el.scrollHeight <= el.clientHeight || trackHeight <= 0) {
			setTranscriptScrollbar((current) => current.height === 0 ? current : { height: 0, top: 0 });
			return;
		}
		setTranscriptScrollbar((current) => {
			const height = Math.max(32, (el.clientHeight / el.scrollHeight) * trackHeight);
			const maxThumbTop = trackHeight - height;
			const maxScrollTop = el.scrollHeight - el.clientHeight;
			const top = trackPadding + (maxScrollTop > 0 ? (el.scrollTop / maxScrollTop) * maxThumbTop : 0);
			return current.height === height && current.top === top ? current : { height, top };
		});
	}

	function handleTranscriptScroll() {
		setIsTranscriptScrolling(true);
		if (scrollbarIdleTimeoutRef.current) clearTimeout(scrollbarIdleTimeoutRef.current);
		scrollbarIdleTimeoutRef.current = window.setTimeout(() => {
			setIsTranscriptScrolling(false);
		}, 800);
		if (scrollFrameRef.current) return;
		scrollFrameRef.current = requestAnimationFrame(() => {
			scrollFrameRef.current = null;
			const el = scrollRef.current;
			if (!el) return;
			updateTranscriptScrollbar();
			const isNearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
			stickToBottomRef.current = isNearBottom;
			setIsNearTranscriptBottom((current) => current === isNearBottom ? current : isNearBottom);
		});
	}

	function jumpToTranscriptBottom() {
		const el = scrollRef.current;
		if (!el) return;
		stickToBottomRef.current = true;
		setIsNearTranscriptBottom(true);
		el.scrollTop = el.scrollHeight;
	}

	function handleTranscriptScrollbarPointerDown(e: React.PointerEvent<HTMLDivElement>) {
		e.preventDefault();
		const thumb = e.currentTarget;
		thumb.setPointerCapture(e.pointerId);
		scrollbarDragRef.current = {
			pointerId: e.pointerId,
			grabOffset: e.clientY - thumb.getBoundingClientRect().top,
		};
		setIsTranscriptScrollbarDragging(true);
	}

	function handleTranscriptScrollbarPointerMove(e: React.PointerEvent<HTMLDivElement>) {
		const drag = scrollbarDragRef.current;
		const el = scrollRef.current;
		if (!drag || drag.pointerId !== e.pointerId || !el) return;
		const trackPadding = 8;
		const trackHeight = el.clientHeight - trackPadding * 2;
		const maxThumbTop = trackHeight - transcriptScrollbar.height;
		if (maxThumbTop <= 0) return;
		const thumbTop = Math.max(0, Math.min(e.clientY - el.getBoundingClientRect().top - trackPadding - drag.grabOffset, maxThumbTop));
		el.scrollTop = (thumbTop / maxThumbTop) * (el.scrollHeight - el.clientHeight);
	}

	function handleTranscriptScrollbarPointerUp(e: React.PointerEvent<HTMLDivElement>) {
		if (scrollbarDragRef.current?.pointerId !== e.pointerId) return;
		e.currentTarget.releasePointerCapture(e.pointerId);
		scrollbarDragRef.current = null;
		setIsTranscriptScrollbarDragging(false);
	}

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
			const usage = data.usage as Usage | undefined;
			if (usage) setLatestRunUsage(usage);
			activeRunRef.current = activeRunRef.current ? { ...activeRunRef.current, completed: true } : null;
			return;
		}
		if (ev.type === "session.run.failed") {
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
		stickToBottomRef.current = true;
		setIsNearTranscriptBottom(true);
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
			activeRunRef.current = { sessionId: activeSessionId, completed: false };
			void startEventSubscription(activeSessionId);

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
			completed = true;
		} catch (err) {
			if ((err as Error).name !== "AbortError") {
				console.error("Send failed", err);
				setMessageText(outboundText);
				setFailedRun({ message: outboundText, agentId: outboundAgentId, modelRef: outboundModelRef, error: formatSessionError(err) });
			}
		} finally {
			setIsStreaming(false);
			setIsStreamPaused(false);
			setStreamingText("");
			setVisibleStreamingText("");
			setStreamingReasoning("");
			eventControllerRef.current?.abort();
			eventControllerRef.current = null;
			activeRunRef.current = null;
			abortControllerRef.current = null;
			if (!completed && controller.signal.aborted) {
				setMessageText(outboundText);
			}
			await loadSession(activeSessionId);
		}
	}

	function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
		if (e.key === "Enter" && !e.shiftKey) {
			e.preventDefault();
			handleSend();
		}
	}

	function openEditSession() {
		if (!session) return;
		setSessionTitleInput(session.title ?? "");
		setSessionWorkDirInput(session.work_dir ?? "");
		setEditingSession(true);
	}

	async function handleSaveSession(e: React.FormEvent) {
		e.preventDefault();
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
	const modelSelectValue = selectedProvider && selectedModel ? `${selectedProvider}/${selectedModel}` : "";
	const modelSelectLabel = selectedProviderName && selectedModel ? `${selectedProviderName} / ${selectedModel}` : undefined;
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
			<header className="flex h-12 shrink-0 items-center gap-2 border-b px-3 sm:px-4">
				<Button render={<Link to="/sessions" />} nativeButton={false} variant="ghost" size="icon-sm" aria-label="Back to sessions">
					<ArrowLeftIcon className="size-4" />
				</Button>
				<div className="min-w-0 flex-1">
					<h1 className="truncate text-sm font-semibold tracking-tight">{sessionTitle || "Untitled session"}</h1>
				</div>
				<div className="hidden items-center gap-1 sm:flex">
					{workspace && (
						<span className="inline-flex max-w-48 items-center gap-1.5 rounded-md px-2 py-1 text-xs text-muted-foreground" title={workspace.path || "Workspace has no directory"}>
							<FolderIcon className="size-3.5 shrink-0" />
							<span className="truncate">{workspace.name}</span>
						</span>
					)}
				</div>
				<SessionContextSheet session={session} calls={modelCalls} />
				<Button type="button" variant={jsonMode ? "secondary" : "ghost"} size="icon-sm" aria-label={jsonMode ? "Exit JSON mode" : "Enter JSON mode"} title={jsonMode ? "Exit JSON mode" : "JSON mode"} onClick={() => setJSONMode((value) => !value)}>
					<CodeIcon className="size-4" />
				</Button>
				<DropdownMenu>
					<DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" aria-label="Session actions" />}>
						<DotsThreeIcon className="size-4" weight="bold" />
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end" className="w-64">
						<DropdownMenuGroup>
							<DropdownMenuLabel>{isDraft ? "New session" : "Session"}</DropdownMenuLabel>
							<div className="px-1.5 pb-1 text-xs text-muted-foreground">
								<div className="flex items-center gap-1.5"><ClockIcon className="size-3.5" />{isDraft ? "Not saved yet" : new Date(session.created_at).toLocaleString()}</div>
								{session.work_dir && <div className="mt-1 flex items-start gap-1.5"><FolderIcon className="mt-0.5 size-3.5 shrink-0" /><span className="break-all">{session.work_dir}</span></div>}
								<div className="mt-1 flex items-center gap-1.5"><ClipboardTextIcon className="size-3.5" />{contextLabel}</div>
							</div>
						</DropdownMenuGroup>
						<DropdownMenuSeparator />
						{!isDraft && (
							<DropdownMenuItem onClick={() => void copySessionValue(session.id, "id")}>
								{copiedValue === "id" ? <CheckIcon className="size-4" /> : <CopyIcon className="size-4" />}Copy session ID
							</DropdownMenuItem>
						)}
						{session.work_dir && (
							<DropdownMenuItem onClick={() => void copySessionValue(session.work_dir!, "path")}>
								{copiedValue === "path" ? <CheckIcon className="size-4" /> : <CopyIcon className="size-4" />}Copy working directory
							</DropdownMenuItem>
						)}
						{!isDraft && (
							<>
								<DropdownMenuSeparator />
								<DropdownMenuItem onClick={openEditSession}><PencilSimpleIcon className="size-4" />Edit session</DropdownMenuItem>
								<DropdownMenuItem variant="destructive" onClick={() => setDeleteSessionOpen(true)}><TrashIcon className="size-4" />Delete session</DropdownMenuItem>
							</>
						)}
					</DropdownMenuContent>
				</DropdownMenu>
			</header>

			<div
				className="relative min-h-0 flex-1"
				onPointerEnter={() => setIsTranscriptHovered(true)}
				onPointerLeave={() => setIsTranscriptHovered(false)}
			>
				<div
					ref={scrollRef}
					onScroll={handleTranscriptScroll}
					className="h-full overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
				>
				<div className="mx-auto flex min-h-full w-full max-w-4xl flex-col px-3 pt-5 pb-0 sm:px-4 sm:pt-6">
					{jsonMode ? (
						<div className="px-4 pb-6 sm:px-6">
							<RawMessages messages={session.history} />
						</div>
					) : transcriptHistory.length === 0 && !visibleStreamingText ? (
						<div className="flex flex-1 items-start justify-center pt-[25dvh] pb-12 text-center">
							<div>
								<div className="text-2xl font-semibold sm:text-3xl">{greeting}</div>
							</div>
						</div>
					) : (
						<div>
							{transcriptHistory.map((msg, idx) => (
								<ChatMessage key={idx} message={msg} toolCallsById={toolCallsById} toolResultsById={toolResultsById} toolActivitiesById={toolActivities} />
							))}
							{visibleStreamingText && <ChatMessage message={buildStreamingMessage(visibleStreamingText)} isStreaming />}
							{isStreaming && <ThinkingIndicator summary={liveReasoningHeading} />}
							{failedRun && (
								<div className="mx-4 my-5 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm sm:mx-6">
									<div className="flex items-start gap-2">
										<WarningCircleIcon className="mt-0.5 size-4 shrink-0 text-destructive" weight="fill" />
										<div className="min-w-0 flex-1">
											<div className="font-medium text-destructive">Message failed</div>
											<pre className="mt-1 max-h-32 overflow-auto whitespace-pre-wrap font-sans text-xs text-muted-foreground">{failedRun.error}</pre>
										</div>
									</div>
									<div className="mt-3 flex justify-end gap-2">
										<Button size="sm" variant="ghost" type="button" onClick={() => void copyFailedRunError()}>
											{copiedFailedRunError ? <CheckIcon className="size-4" /> : <CopyIcon className="size-4" />}
											{copiedFailedRunError ? "Copied" : "Copy error"}
										</Button>
										<Button size="sm" type="button" onClick={() => void handleSend(undefined, failedRun)} disabled={isStreaming}>
											<ArrowClockwiseIcon className="size-4" />
											Retry
										</Button>
									</div>
								</div>
							)}
						</div>
					)}
				</div>
				</div>
				{transcriptScrollbar.height > 0 && (
					<div
						className={`absolute right-0 z-10 w-3 select-none transition-opacity duration-200 ${isTranscriptHovered || isTranscriptScrolling || isTranscriptScrollbarDragging ? "opacity-100" : "pointer-events-none opacity-0"}`}
						style={{ height: transcriptScrollbar.height, top: transcriptScrollbar.top }}
						onPointerDown={handleTranscriptScrollbarPointerDown}
						onPointerMove={handleTranscriptScrollbarPointerMove}
						onPointerUp={handleTranscriptScrollbarPointerUp}
						onPointerCancel={handleTranscriptScrollbarPointerUp}
					>
						<div className="absolute inset-y-0 left-1/2 w-1 -translate-x-1/2 rounded-full bg-border/70 transition-colors hover:bg-foreground/40" />
					</div>
				)}
			</div>

			<form onSubmit={handleSend} className="shrink-0 px-3 pb-3 sm:px-4 sm:pb-4">
				<div className="relative mx-auto max-w-4xl rounded-xl border bg-card p-2 shadow-lg shadow-primary/10">
					{!isNearTranscriptBottom && (
						<Button
							className="absolute -top-10 right-2 z-20 rounded-full shadow-md"
							size="icon-sm"
							type="button"
							onClick={jumpToTranscriptBottom}
							aria-label="Jump to latest message"
							title="Jump to latest message"
						>
							<ArrowDownIcon className="size-4" />
						</Button>
					)}
					<Textarea
						value={messageText}
						onChange={(e) => setMessageText(e.target.value)}
						onKeyDown={handleKeyDown}
						placeholder="Ask anything..."
						className="min-h-20 resize-none border-0 bg-transparent shadow-none focus-visible:ring-0 sm:min-h-24"
						disabled={isStreaming}
					/>
					<div className="mt-2 flex items-center justify-between gap-2 border-t pt-2">
						<div className="flex min-w-0 flex-wrap items-center gap-1.5">
							<Select value={selectedAgent} onValueChange={(v) => {
								const agentId = v ?? "";
								setSelectedAgent(agentId);
								persistLastAgentId(agentId);
							}}>
								<SelectTrigger className="h-8 w-40 border-0 bg-muted/60 text-xs shadow-none sm:w-56"><SelectValue placeholder="Select agent">{selectedAgentName}</SelectValue></SelectTrigger>
								<SelectContent>{agents.map((a) => <SelectItem key={a.id} value={a.id}>{a.name}</SelectItem>)}</SelectContent>
							</Select>
							<Select value={modelSelectValue} onValueChange={(v) => {
								const modelRef = splitModelRef(v ?? "");
								setSelectedProvider(modelRef.provider);
								setSelectedModel(modelRef.model);
								persistLastModelRef(v ?? "");
							}} disabled={!hasModels}>
								<SelectTrigger className="h-8 w-44 border-0 bg-muted/60 text-xs shadow-none sm:w-72"><SelectValue placeholder="Select model">{modelSelectLabel}</SelectValue></SelectTrigger>
								<SelectContent>{selectableProviders.map((provider) => <SelectGroup key={provider.id}><SelectLabel>{provider.name}</SelectLabel>{(models[provider.id] ?? []).map((model) => <SelectItem key={`${provider.id}/${model.id}`} value={`${provider.id}/${model.id}`}>{model.id}</SelectItem>)}</SelectGroup>)}</SelectContent>
							</Select>
						</div>
						<div className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
							{isStreaming ? (
								<>
									<Button size="icon-sm" variant="secondary" type="button" onClick={isStreamPaused ? handleResumeStream : handlePauseStream} aria-label={isStreamPaused ? "Resume stream" : "Pause stream"} title={isStreamPaused ? "Resume stream" : "Pause stream"}>{isStreamPaused ? <PlayIcon className="size-4" /> : <PauseIcon className="size-4" />}</Button>
									<Button size="sm" variant="destructive" type="button" onClick={handleAbort}><StopIcon className="size-4" /><span className="hidden sm:inline">Abort</span></Button>
								</>
							) : (
								<Button size="icon-sm" type="submit" aria-label="Send message" title="Send message" disabled={!messageText.trim() || !selectedAgent}>
									<PaperPlaneIcon className="size-4" />
								</Button>
							)}
						</div>
					</div>
				</div>
			</form>

			<Dialog open={editingSession} onOpenChange={setEditingSession}>
				<DialogContent>
					<form onSubmit={handleSaveSession} className="grid gap-4">
						<DialogHeader><DialogTitle>Edit session</DialogTitle><DialogDescription>Changing the working directory removes the workspace link.</DialogDescription></DialogHeader>
						<div className="grid gap-3">
							<label className="grid gap-1 text-sm font-medium">Name<Input value={sessionTitleInput} onChange={(e) => setSessionTitleInput(e.target.value)} placeholder="Session name" /></label>
							<label className="grid gap-1 text-sm font-medium">Working directory<Input value={sessionWorkDirInput} onChange={(e) => setSessionWorkDirInput(e.target.value)} placeholder="Optional working directory" /></label>
						</div>
						<DialogFooter><Button type="button" variant="outline" onClick={() => setEditingSession(false)} disabled={savingSession}>Cancel</Button><Button type="submit" disabled={savingSession}>{savingSession ? "Saving..." : "Save changes"}</Button></DialogFooter>
					</form>
				</DialogContent>
			</Dialog>

			<AlertDialog open={deleteSessionOpen} onOpenChange={setDeleteSessionOpen}>
				<AlertDialogContent>
					<AlertDialogHeader><AlertDialogTitle>Delete session?</AlertDialogTitle><AlertDialogDescription>This will permanently delete {session.title || session.id}. This action cannot be undone.</AlertDialogDescription></AlertDialogHeader>
					<AlertDialogFooter><AlertDialogCancel disabled={deletingSession}>Cancel</AlertDialogCancel><AlertDialogAction variant="destructive" disabled={deletingSession} onClick={handleDeleteSession}>{deletingSession ? "Deleting..." : "Delete"}</AlertDialogAction></AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}
