import { useEffect, useRef, useState, useCallback } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { wfetch } from "@/lib/client";
import { selectGreeting } from "@/lib/greeting";
import { isProviderSelectable } from "@/lib/providers";
import { agentExists, buildUserMessage, modelRefExists, persistLastAgentId, persistLastModelRef, shouldAutoGenerateTitle, LAST_AGENT_ID_KEY, LAST_MODEL_REF_KEY } from "@/lib/session-detail";
import { generateSessionTitle } from "@/lib/session-stream";
import { showErrorToast } from "@/lib/toast";
import type { Session, Agent, Workspace, ModelCall, Provider, ProviderModel, ToolCallPart, ToolResultPart } from "@/lib/types";
import { contextTokenCount, formatContextPercent, formatTokenCount, latestAssistantUsage, splitModelRef } from "@/lib/utils";
import { HexWaveSpinner } from "@/components/hex-wave-spinner";
import { SessionComposer } from "@/components/session-composer";
import { SessionDialogs } from "@/components/session-dialogs";
import { SessionHeader } from "@/components/session-header";
import { SessionTranscript } from "@/components/session-transcript";
import { useSessionRun, type FailedRun } from "@/hooks/use-session-run";
import { useStreamReveal } from "@/hooks/use-stream-reveal";
import { useTranscriptScroll } from "@/hooks/use-transcript-scroll";

type SessionDetailSearch = {
	workspace?: string;
};

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
	const [streamingTitle, setStreamingTitle] = useState("");
	const [isTitleStreaming, setIsTitleStreaming] = useState(false);
	const [copiedFailedRunError, setCopiedFailedRunError] = useState(false);
	const [editingSession, setEditingSession] = useState(false);
	const [savingSession, setSavingSession] = useState(false);
	const [deleteSessionOpen, setDeleteSessionOpen] = useState(false);
	const [deletingSession, setDeletingSession] = useState(false);
	const [copiedValue, setCopiedValue] = useState<"id" | "path" | "">("");
	const [jsonMode, setJSONMode] = useState(false);
	const activeSessionIdRef = useRef(sessionId);
	const skipNextSessionLoadRef = useRef(false);
	const composerRef = useRef<HTMLTextAreaElement>(null);
	const titleSessionIdRef = useRef(sessionId);

	useEffect(() => {
		activeSessionIdRef.current = sessionId;
		setJSONMode(false);
		if (titleSessionIdRef.current !== sessionId) {
			setStreamingTitle("");
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

	const run = useSessionRun({ sessionId, loadSession, setSession });
	const visibleStreamingText = useStreamReveal(run.streamingText, run.isStreaming);
	const visibleStreamingTitle = useStreamReveal(streamingTitle, isTitleStreaming);
	const transcriptScroll = useTranscriptScroll(`${session?.id}:${session?.history.length}:${visibleStreamingText.length}:${jsonMode}`, `${loading}:${session?.id}`);

	useEffect(() => {
		transcriptScroll.reset();
	}, [sessionId]);

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
		if (!isDraft || loading || run.isStreaming || session?.history.length) return;
		const frame = requestAnimationFrame(() => composerRef.current?.focus());
		return () => cancelAnimationFrame(frame);
	}, [isDraft, run.isStreaming, loading, session?.history.length]);

	async function copyFailedRunError() {
		if (!run.failedRun) return;
		try {
			await navigator.clipboard.writeText(run.failedRun.error);
			setCopiedFailedRunError(true);
			window.setTimeout(() => setCopiedFailedRunError(false), 1200);
		} catch (err) {
			showErrorToast(err, "Could not copy error");
		}
	}

	async function handleAbort() {
		await run.abort(activeSessionIdRef.current);
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
		setMessageText("");
		transcriptScroll.reset();
		setSession((prev) => {
			if (!prev) return prev;
			return { ...prev, history: [...prev.history, buildUserMessage(outboundText)] };
		});

		const controller = run.begin({ message: outboundText, agentId: outboundAgentId, modelRef: outboundModelRef });
		let accepted = false;
		let activeSessionId = sessionId;
		let titlePromise: Promise<string> | null = null;

		if (shouldGenerateTitle && outboundModelRef) {
			titleSessionIdRef.current = sessionId;
			setIsTitleStreaming(true);
			setStreamingTitle("");
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

			await run.captureCursor(activeSessionId);

			const headers = new Headers({
				"Content-Type": "application/json",
			});
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
			run.start(activeSessionId, admitted.run_id);
			accepted = true;
		} catch (err) {
			if ((err as Error).name !== "AbortError") {
				console.error("Send failed", err);
				setMessageText(outboundText);
				await run.fail(err, activeSessionId);
			}
		} finally {
			run.finishSubmission();
			if (!accepted && controller.signal.aborted) {
				setMessageText(outboundText);
			}
		}
	}

	function openEditSession() {
		if (!session) return;
		setEditingSession(true);
	}

	async function handleSaveSession(title: string, workDir: string) {
		if (!session || isDraft) return;
		setSavingSession(true);
		try {
			const workingDirectoryChanged = workDir.trim() !== (session.work_dir ?? "");
			const updated = (await wfetch(`/sessions/${session.id}`, {
				method: "PUT",
				body: JSON.stringify({
					title: title.trim(),
					...(workingDirectoryChanged ? { working_directory: workDir.trim() } : {}),
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
	const latestUsage = latestAssistantUsage(session?.history ?? []) ?? run.latestRunUsage;
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
	const modelCallsByMessageId = new Map(modelCalls.filter((call) => call.assistant_message_id).map((call) => [call.assistant_message_id!, call]));
	const agentNames = new Map(agents.map((agent) => [agent.id, agent.name]));
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
			<SessionTranscript messages={transcriptHistory} rawMessages={session.history} jsonMode={jsonMode} greeting={greeting} streamingText={visibleStreamingText} streamingReasoning={run.streamingReasoning} isStreaming={run.isStreaming} toolCallsById={toolCallsById} toolResultsById={toolResultsById} toolActivitiesById={run.toolActivities} modelCallsByMessageId={modelCallsByMessageId} agentNames={agentNames} failedRun={run.failedRun} copiedFailedRunError={copiedFailedRunError} onCopyFailedRunError={() => void copyFailedRunError()} onRetry={() => void handleSend(undefined, run.failedRun ?? undefined)} scroll={transcriptScroll} />
			<SessionComposer composerRef={composerRef} messageText={messageText} selectedAgent={selectedAgent} selectedAgentName={selectedAgentName} selectedProvider={selectedProvider} selectedModel={selectedModel} selectedProviderName={selectedProviderName} agents={agents} providers={selectableProviders} models={models} hasModels={hasModels} isStreaming={run.isStreaming} isNearTranscriptBottom={transcriptScroll.isNearBottom} onMessageChange={setMessageText} onAgentChange={(agentId) => { setSelectedAgent(agentId); persistLastAgentId(agentId); }} onModelChange={(modelRef) => { const ref = splitModelRef(modelRef); setSelectedProvider(ref.provider); setSelectedModel(ref.model); persistLastModelRef(modelRef); }} onSubmit={() => void handleSend()} onAbort={handleAbort} onJumpToBottom={transcriptScroll.jumpToBottom} />
			<SessionDialogs session={session} editing={editingSession} saving={savingSession} deleteOpen={deleteSessionOpen} deleting={deletingSession} onEditingChange={setEditingSession} onDeleteOpenChange={setDeleteSessionOpen} onSave={(title, workDir) => void handleSaveSession(title, workDir)} onDelete={() => void handleDeleteSession()} />
		</div>
	);
}
