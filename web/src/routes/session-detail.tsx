import { useEffect, useRef, useState, useCallback } from "react";
import { useParams, Link } from "@tanstack/react-router";
import { wfetch, getClientId } from "@/lib/client";
import type { Session, Agent, Message, Part } from "@/lib/types";
import { Button } from "@/components/core/button";
import { Textarea } from "@/components/core/textarea";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/core/select";
import { ChatMessage } from "@/components/chat-message";
import { ArrowLeftIcon, StopIcon, PaperPlaneRightIcon } from "@phosphor-icons/react";

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

export default function SessionDetailPage() {
  const { sessionId } = useParams({ from: "/sessions/$sessionId" });
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(true);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [selectedAgent, setSelectedAgent] = useState("");
  const [messageText, setMessageText] = useState("");
  const [streamingText, setStreamingText] = useState("");
  const [isStreaming, setIsStreaming] = useState(false);
  const abortControllerRef = useRef<AbortController | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  const loadSession = useCallback(async () => {
    try {
      const data = (await wfetch(`/sessions/${sessionId}`)) as Session;
      setSession(data);
    } catch (err) {
      console.error("Failed to load session", err);
      alert(String(err));
    } finally {
      setLoading(false);
    }
  }, [sessionId]);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const [sessData, agentsData] = await Promise.all([
          wfetch(`/sessions/${sessionId}`) as Promise<Session>,
          wfetch("/agents") as Promise<Agent[]>,
        ]);
        if (!cancelled) {
          setSession(sessData);
          setAgents(agentsData);
          if (agentsData.length > 0) {
            setSelectedAgent(agentsData[0].id);
          }
        }
      } catch (err) {
        console.error("Failed to load session/agents", err);
        alert(String(err));
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    load();
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [session?.history, streamingText]);

  async function handleAbort() {
    try {
      await wfetch(`/sessions/${sessionId}/abort`, { method: "POST" });
    } catch (err) {
      console.error("Abort failed", err);
    }
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    setIsStreaming(false);
    setStreamingText("");
    await loadSession();
  }

  async function handleSend(e?: React.FormEvent) {
    if (e) e.preventDefault();
    if (!messageText.trim() || !selectedAgent) return;

    const controller = new AbortController();
    abortControllerRef.current = controller;
    setIsStreaming(true);
    setStreamingText("");

    try {
      const headers = new Headers({
        "Content-Type": "application/json",
      });
      const clientId = getClientId();
      if (clientId) {
        headers.set("X-Wingman-Client", clientId);
      }

      const res = await fetch(`/sessions/${sessionId}/message/stream`, {
        method: "POST",
        headers,
        body: JSON.stringify({ agent_id: selectedAgent, message: messageText.trim() }),
        signal: controller.signal,
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(`HTTP ${res.status}: ${text}`);
      }

      let textBuffer = "";
      for await (const ev of readSSE(res)) {
        if (ev.event === "error") {
          console.error("Stream error:", ev.data);
          continue;
        }
        if (ev.event === "done") {
          break;
        }
        if (ev.event === "stream_part") {
          const envelope = ev.data as {
            type: string;
            version: number;
            data: { step: number; part: { type: string; delta?: string } };
          };
          const part = envelope.data?.part;
          if (part?.type === "text-delta" && part.delta) {
            textBuffer += part.delta;
            setStreamingText(textBuffer);
          }
        }
        if (ev.event === "message") {
          const envelope = ev.data as {
            type: string;
            version: number;
            data: { message: Message };
          };
          if (envelope.data?.message) {
            setSession((prev) => {
              if (!prev) return prev;
              return { ...prev, history: [...prev.history, envelope.data.message] };
            });
          }
        }
      }
    } catch (err) {
      if ((err as Error).name !== "AbortError") {
        console.error("Send failed", err);
        alert(String(err));
      }
    } finally {
      setIsStreaming(false);
      setStreamingText("");
      abortControllerRef.current = null;
      setMessageText("");
      await loadSession();
    }
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      handleSend();
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
      <div className="flex items-center gap-2 border-b py-3 text-sm">
        <Link to="/sessions" className="text-muted-foreground hover:underline">
          <ArrowLeftIcon className="size-4 inline" />
          Sessions
        </Link>
        <span className="text-muted-foreground">›</span>
        <span className="font-medium">{session.title || session.id.slice(0, 8)}</span>
      </div>

      <div className="flex items-start justify-between gap-4 border-b py-3">
        <div className="grid grid-cols-2 gap-x-8 gap-y-1 text-xs text-muted-foreground">
          <div>
            <span className="font-medium text-foreground">Workdir:</span>{" "}
            {session.work_dir || "—"}
          </div>
          <div>
            <span className="font-medium text-foreground">Created:</span>{" "}
            {new Date(session.created_at).toLocaleString()}
          </div>
        </div>
        <Button
          size="sm"
          variant="destructive"
          onClick={handleAbort}
        >
          <StopIcon className="size-4" />
          Abort
        </Button>
      </div>

      <div ref={scrollRef} className="flex-1 overflow-y-auto py-4">
        {session.history.map((msg, idx) => (
          <ChatMessage key={idx} message={msg} />
        ))}
        {streamingText && (
          <ChatMessage message={buildStreamingMessage(streamingText)} />
        )}
      </div>

      <form
        onSubmit={handleSend}
        className="flex flex-col gap-2 border-t py-3"
      >
        <div className="flex items-center gap-2">
            <Select value={selectedAgent} onValueChange={(v) => setSelectedAgent(v ?? "")}>
            <SelectTrigger className="h-8 w-48 text-xs">
              <SelectValue placeholder="Select agent" />
            </SelectTrigger>
            <SelectContent>
              {agents.map((a) => (
                <SelectItem key={a.id} value={a.id}>
                  {a.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <span className="text-xs text-muted-foreground">
            {isStreaming ? "Streaming..." : "Cmd+Enter to send"}
          </span>
        </div>
        <div className="flex gap-2">
          <Textarea
            value={messageText}
            onChange={(e) => setMessageText(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type a message..."
            className="min-h-[80px] flex-1"
            disabled={isStreaming}
          />
          <Button
            type="submit"
            disabled={isStreaming || !messageText.trim() || !selectedAgent}
            className="self-end"
          >
            <PaperPlaneRightIcon className="size-4" />
          </Button>
        </div>
      </form>
    </div>
  );
}
