import { useEffect, useRef, useState, useCallback } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { wfetch, getClientId } from "@/lib/client";
import { isProviderSelectable } from "@/lib/providers";
import { showErrorToast } from "@/lib/toast";
import type { Session, Agent, Workspace, Message, Part, Provider, ProviderModel, ToolCallPart, ToolResultPart, Usage } from "@/lib/types";
import { contextTokenCount, formatContextPercent, formatTokenCount, latestAssistantUsage, splitModelRef } from "@/lib/utils";
import { Alert, AlertDescription, AlertTitle } from "@wingman/core/components/core/alert";
import { Button } from "@wingman/core/components/core/button";
import { Textarea } from "@wingman/core/components/core/textarea";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
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
import { StopIcon } from "@phosphor-icons/react";

const STREAM_MIN_CHARS_PER_FRAME = 1;
const STREAM_MAX_CHARS_PER_FRAME = 18;
const STREAM_BACKLOG_DIVISOR = 14;
const LAST_AGENT_ID_KEY = "wingman_last_agent_id";
const LAST_MODEL_REF_KEY = "wingman_last_model_ref";
const DEFAULT_SESSION_TITLE = "New session";

type SessionDetailSearch = {
  workspace?: string;
};

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

function eventField<T>(data: unknown, lower: string, upper: string): T | undefined {
  if (!data || typeof data !== "object") return undefined;
  const record = data as Record<string, unknown>;
  return (record[lower] ?? record[upper]) as T | undefined;
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
  const [session, setSession] = useState<Session | null>(null);
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [loading, setLoading] = useState(true);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [models, setModels] = useState<Record<string, ProviderModel[]>>({});
  const [selectedAgent, setSelectedAgent] = useState("");
  const [selectedProvider, setSelectedProvider] = useState("");
  const [selectedModel, setSelectedModel] = useState("");
  const [messageText, setMessageText] = useState("");
  const [streamingText, setStreamingText] = useState("");
  const [visibleStreamingText, setVisibleStreamingText] = useState("");
  const [streamingTitle, setStreamingTitle] = useState("");
  const [visibleStreamingTitle, setVisibleStreamingTitle] = useState("");
  const [isTitleStreaming, setIsTitleStreaming] = useState(false);
  const [isStreaming, setIsStreaming] = useState(false);
  const [latestRunUsage, setLatestRunUsage] = useState<Usage | undefined>();
  const [error, setError] = useState("");
  const abortControllerRef = useRef<AbortController | null>(null);
  const activeSessionIdRef = useRef(sessionId);
  const skipNextSessionLoadRef = useRef(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const stickToBottomRef = useRef(true);
  const streamingTextRef = useRef("");
  const visibleStreamingTextRef = useRef("");
  const streamingTitleRef = useRef("");
  const visibleStreamingTitleRef = useRef("");
  const titleSessionIdRef = useRef(sessionId);

  useEffect(() => {
    activeSessionIdRef.current = sessionId;
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
      return;
    }

    try {
      const data = (await wfetch(`/sessions/${id}`)) as Session;
      setSession(data);
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
        const [sessData, agentsData, providerData] = await Promise.all([
          isDraft ? Promise.resolve(null) : wfetch(`/sessions/${sessionId}`) as Promise<Session>,
          wfetch("/agents") as Promise<Agent[]>,
          wfetch("/provider") as Promise<Provider[]>,
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
  }, [session?.history, visibleStreamingText]);

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

  function handleTranscriptScroll() {
    const el = scrollRef.current;
    if (!el) return;
    stickToBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  }

  function handleTranscriptWheel(e: React.WheelEvent<HTMLDivElement>) {
    if (e.deltaY < 0) {
      stickToBottomRef.current = false;
    }
  }

  function handleTranscriptTouchMove() {
    stickToBottomRef.current = false;
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
    setIsStreaming(false);
    setStreamingText("");
    setVisibleStreamingText("");
    await loadSession();
  }

  async function handleSend(e?: React.FormEvent) {
    if (e) e.preventDefault();
    if (!messageText.trim() || !selectedAgent) return;

    const outboundText = messageText.trim();
    const outboundModelRef = selectedProvider && selectedModel ? `${selectedProvider}/${selectedModel}` : "";
    const shouldGenerateTitle = shouldAutoGenerateTitle(session);
    persistLastAgentId(selectedAgent);
    persistLastModelRef(outboundModelRef);
    setError("");
    setMessageText("");
    setSession((prev) => {
      if (!prev) return prev;
      return { ...prev, history: [...prev.history, buildUserMessage(outboundText)] };
    });

    const controller = new AbortController();
    abortControllerRef.current = controller;
    setIsStreaming(true);
    setStreamingText("");
    setVisibleStreamingText("");
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

      const headers = new Headers({
        "Content-Type": "application/json",
      });
      const clientId = getClientId();
      if (clientId) {
        headers.set("X-Wingman-Client", clientId);
      }

      const res = await fetch(`/sessions/${activeSessionId}/message/stream`, {
        method: "POST",
        headers,
        body: JSON.stringify({
          agent_id: selectedAgent,
          model_ref: outboundModelRef,
          message: outboundText,
        }),
        signal: controller.signal,
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(`HTTP ${res.status}: ${text}`);
      }

      let textBuffer = "";
      for await (const ev of readSSE(res)) {
        if (ev.event === "error") {
          const message =
            typeof ev.data === "string"
              ? ev.data
              : eventField<{ error?: string }>(ev.data, "data", "Data")?.error;
          throw new Error(message || "Stream failed");
        }
        if (ev.event === "done") {
          const envelope = ev.data as {
            data?: unknown;
            Data?: unknown;
          };
          const data = envelope.data ?? envelope.Data;
          const usage = eventField<Usage>(data, "usage", "Usage");
          if (usage) {
            setLatestRunUsage(usage);
          }
          completed = true;
          break;
        }
        if (ev.event === "stream_part") {
          const envelope = ev.data as {
            type: string;
            version: number;
            data?: unknown;
            Data?: unknown;
          };
          const data = envelope.data ?? envelope.Data;
          const part = eventField<{ type: string; delta?: string }>(data, "part", "Part");
          if ((part?.type === "text_delta" || part?.type === "text-delta") && part.delta) {
            textBuffer += part.delta;
            setStreamingText(textBuffer);
          }
        }
        if (ev.event === "message") {
          const envelope = ev.data as {
            type: string;
            version: number;
            data?: unknown;
            Data?: unknown;
          };
          const data = envelope.data ?? envelope.Data;
          const message = eventField<Message>(data, "message", "Message");
          if (message) {
            setSession((prev) => {
              if (!prev) return prev;
              return { ...prev, history: [...prev.history, message] };
            });
            if (message.role === "assistant") {
              textBuffer = "";
              streamingTextRef.current = "";
              visibleStreamingTextRef.current = "";
              setStreamingText("");
              setVisibleStreamingText("");
            }
          }
        }
      }
      completed = true;
    } catch (err) {
      if ((err as Error).name !== "AbortError") {
        console.error("Send failed", err);
        setMessageText(outboundText);
        setError(formatSessionError(err));
      }
    } finally {
      setIsStreaming(false);
      setStreamingText("");
      setVisibleStreamingText("");
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
  const toolCallsById = new Map<string, ToolCallPart>();
  const toolResultsById = new Map<string, true>();
  for (const msg of session?.history ?? []) {
    for (const part of msg.content) {
      if (part.type === "tool_call") {
        const toolCall = part as ToolCallPart;
        toolCallsById.set(toolCall.call_id, toolCall);
      } else if (part.type === "tool_result") {
        const toolResult = part as ToolResultPart;
        toolResultsById.set(toolResult.call_id, true);
      }
    }
  }

  if (loading) {
    return <div className="px-4 py-6 text-sm text-muted-foreground">Loading...</div>;
  }

  if (!session) {
    return <div className="px-4 py-6 text-sm text-muted-foreground">Session not found.</div>;
  }

  return (
    <div className="mx-auto flex h-[calc(100vh-57px)] max-w-5xl flex-col px-4">
      <div className="border-b py-4">
        <PageBreadcrumb
          items={workspace ? [
            { label: "Sessions", to: "/sessions" },
            { label: workspace.name, to: `/sessions?workspace=${workspace.id}` },
            { label: sessionTitle || session.id.slice(0, 8) },
          ] : [
            { label: "Sessions", to: "/sessions" },
            { label: sessionTitle || session.id.slice(0, 8) },
          ]}
        />
        <div className="mt-4 flex items-start justify-between gap-4">
          <div className="min-w-0">
            <h1 className="truncate text-lg font-semibold tracking-tight">
              {sessionTitle || "Untitled session"}
            </h1>
            <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
              <span>{isDraft ? "Not saved yet" : new Date(session.created_at).toLocaleString()}</span>
              <span className="max-w-full truncate">{session.work_dir || "-"}</span>
              <span>
                {contextWindow
                  ? `${contextTokenLabel} / ${formatTokenCount(contextWindow)} context${contextPercent ? ` (${contextPercent})` : ""}`
                  : `${contextTokenLabel} context`}
              </span>
            </div>
          </div>
        </div>
      </div>

      {error && (
        <Alert variant="destructive" className="mt-4">
          <AlertTitle>Message failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div
        ref={scrollRef}
        onScroll={handleTranscriptScroll}
        onWheel={handleTranscriptWheel}
        onTouchMove={handleTranscriptTouchMove}
        className="flex-1 overflow-y-auto py-2"
      >
        {session.history.length === 0 && !visibleStreamingText ? (
          <div className="flex h-full items-center justify-center py-12 text-center">
            <div>
              <div className="text-sm font-medium">No messages yet</div>
              <div className="mt-1 text-xs text-muted-foreground">Pick an agent and start the session below.</div>
            </div>
          </div>
        ) : (
          <div>
            {session.history.map((msg, idx) => (
              <ChatMessage key={idx} message={msg} toolCallsById={toolCallsById} toolResultsById={toolResultsById} />
            ))}
            {visibleStreamingText && (
              <ChatMessage message={buildStreamingMessage(visibleStreamingText)} isStreaming />
            )}
          </div>
        )}
      </div>

      <form
        onSubmit={handleSend}
        className="border-t py-3"
      >
        <div className="rounded-xl border bg-card p-2 shadow-sm shadow-primary/5">
          <Textarea
            value={messageText}
            onChange={(e) => setMessageText(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type a message..."
            className="min-h-24 resize-none border-0 bg-transparent shadow-none focus-visible:ring-0"
            disabled={isStreaming}
          />
          <div className="mt-2 flex items-center justify-between gap-3 border-t pt-2">
            <div className="flex flex-wrap items-center gap-2">
              <Select
                value={selectedAgent}
                onValueChange={(v) => {
                  const agentId = v ?? "";
                  setSelectedAgent(agentId);
                  persistLastAgentId(agentId);
                }}
              >
                <SelectTrigger className="h-8 w-56 border-0 bg-muted/60 text-xs shadow-none">
                  <SelectValue placeholder="Select agent">
                    {selectedAgentName}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {agents.map((a) => (
                    <SelectItem key={a.id} value={a.id}>
                      {a.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select
                value={modelSelectValue}
                onValueChange={(v) => {
                  const modelRef = splitModelRef(v ?? "");
                  setSelectedProvider(modelRef.provider);
                  setSelectedModel(modelRef.model);
                  persistLastModelRef(v ?? "");
                }}
                disabled={!hasModels}
              >
                <SelectTrigger className="h-8 w-72 border-0 bg-muted/60 text-xs shadow-none">
                  <SelectValue placeholder="Select model">
                    {modelSelectLabel}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {selectableProviders.map((provider) => (
                    <SelectGroup key={provider.id}>
                      <SelectLabel>{provider.name}</SelectLabel>
                      {(models[provider.id] ?? []).map((model) => (
                        <SelectItem key={`${provider.id}/${model.id}`} value={`${provider.id}/${model.id}`}>
                          {model.id}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              {isStreaming ? (
                <Button size="sm" variant="destructive" type="button" onClick={handleAbort}>
                  <StopIcon className="size-4" />
                  Abort
                </Button>
              ) : (
                <span>Enter to send, Shift+Enter for newline</span>
              )}
            </div>
          </div>
        </div>
      </form>
    </div>
  );
}
