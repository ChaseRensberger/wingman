import { useEffect, useEffectEvent, useRef, useState, type Dispatch, type SetStateAction } from "react";

import { getClientId, wfetch } from "@/lib/client";
import { formatSessionError, isRecord } from "@/lib/session-detail";
import { readSSE, type SessionEvent } from "@/lib/session-stream";
import type { Message, Session, ToolActivity, Usage } from "@/lib/types";

export type FailedRun = { message: string; agentId: string; modelRef: string; error: string };
export type SessionRunRequest = Omit<FailedRun, "error">;

type Options = {
	sessionId: string;
	loadSession: (id?: string) => Promise<void>;
	setSession: Dispatch<SetStateAction<Session | null>>;
};

async function latestSessionEventSeq(sessionId: string): Promise<number> {
	let after = 0;
	for (;;) {
		const page = await wfetch(`/sessions/${sessionId}/events/history?after=${after}&limit=500`) as { data?: SessionEvent[]; has_more?: boolean };
		const events = page.data ?? [];
		if (events.length === 0) return after;
		after = events.at(-1)?.cursor?.seq ?? after;
		if (!page.has_more) return after;
	}
}

export function useSessionRun({ sessionId, loadSession, setSession }: Options) {
	const load = useEffectEvent(loadSession);
	const submissionControllerRef = useRef<AbortController | null>(null);
	const eventControllerRef = useRef<AbortController | null>(null);
	const lastEventSeqRef = useRef(0);
	const activeRunRef = useRef<{ sessionId: string; runId?: string; completed: boolean } | null>(null);
	const requestRef = useRef<SessionRunRequest | null>(null);
	const [isStreaming, setIsStreaming] = useState(false);
	const [isStreamPaused, setIsStreamPaused] = useState(false);
	const [streamingText, setStreamingText] = useState("");
	const [streamingReasoning, setStreamingReasoning] = useState("");
	const [latestRunUsage, setLatestRunUsage] = useState<Usage>();
	const [failedRun, setFailedRun] = useState<FailedRun | null>(null);
	const [toolActivities, setToolActivities] = useState<Map<string, ToolActivity>>(() => new Map());

	function reset() {
		eventControllerRef.current?.abort();
		eventControllerRef.current = null;
		activeRunRef.current = null;
		requestRef.current = null;
		setIsStreaming(false);
		setIsStreamPaused(false);
		setStreamingReasoning("");
		setStreamingText("");
	}

	useEffect(() => {
		// Creating a draft session changes the route before its admission request finishes.
		if (submissionControllerRef.current) return;
		lastEventSeqRef.current = 0;
		setFailedRun(null);
		setToolActivities(new Map());
		reset();
		return () => eventControllerRef.current?.abort();
	}, [sessionId]);

	function applySessionEvent(ev: SessionEvent) {
		if (typeof ev.cursor?.seq === "number" && ev.cursor.seq > lastEventSeqRef.current) lastEventSeqRef.current = ev.cursor.seq;
		const data = ev.data ?? {};
		if (ev.type === "session.tool.updated") {
			const callID = typeof data.call_id === "string" ? data.call_id : "";
			const tool = typeof data.tool === "string" ? data.tool : "";
			const status = data.status;
			if (!callID || !tool || !["pending", "running", "completed", "error"].includes(String(status))) return;
			setToolActivities((previous) => {
				const next = new Map(previous);
				next.set(callID, { call_id: callID, tool, status: status as ToolActivity["status"], input: isRecord(data.input) ? data.input : undefined, output: typeof data.output === "string" ? data.output : undefined, metadata: isRecord(data.metadata) ? data.metadata : undefined, error: typeof data.error === "string" ? data.error : undefined, started_at: typeof data.started_at === "string" ? data.started_at : undefined, completed_at: typeof data.completed_at === "string" ? data.completed_at : undefined, duration_ms: typeof data.duration_ms === "number" ? data.duration_ms : undefined });
				return next;
			});
			return;
		}
		if (ev.type === "session.text.delta") {
			const delta = typeof data.delta === "string" ? data.delta : "";
			if (delta) setStreamingText((previous) => previous + delta);
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
			setSession((previous) => previous ? { ...previous, history: [...previous.history, message] } : previous);
			if (message.role === "assistant") setStreamingText("");
			return;
		}
		if (ev.type === "session.run.completed") {
			if (activeRunRef.current?.runId && data.run_id !== activeRunRef.current.runId) return;
			const usage = data.usage as Usage | undefined;
			if (usage) setLatestRunUsage(usage);
			if (activeRunRef.current) activeRunRef.current = { ...activeRunRef.current, completed: true };
			return;
		}
		if (ev.type === "session.run.failed") {
			if (activeRunRef.current?.runId && data.run_id !== activeRunRef.current.runId) return;
			if (activeRunRef.current) activeRunRef.current = { ...activeRunRef.current, completed: true };
			throw new Error(typeof data.error === "string" ? data.error : "Run failed");
		}
	}

	async function subscribe(id: string, signal: AbortSignal) {
		const headers = new Headers();
		const clientId = getClientId();
		if (clientId) headers.set("X-Wingman-Client", clientId);
		const response = await fetch(`/sessions/${id}/events?after=${lastEventSeqRef.current}`, { headers, signal });
		if (!response.ok) throw new Error(`HTTP ${response.status}: ${await response.text()}`);
		for await (const event of readSSE(response)) {
			if (!event.event || event.event.startsWith(":")) continue;
			applySessionEvent(event.data as SessionEvent);
			if (activeRunRef.current?.completed) return;
		}
	}

	async function subscribeAndFinish(id: string, controller: AbortController) {
		try {
			await subscribe(id, controller.signal);
		} catch (err) {
			if ((err as Error).name !== "AbortError") {
				console.error("Event stream failed", err);
				const request = requestRef.current;
				if (request) setFailedRun({ ...request, error: formatSessionError(err) });
			}
		} finally {
			if (activeRunRef.current?.sessionId === id && activeRunRef.current.completed) {
				reset();
				await load(id);
			}
		}
	}

	function begin(request: SessionRunRequest) {
		const controller = new AbortController();
		submissionControllerRef.current = controller;
		requestRef.current = request;
		setFailedRun(null);
		setIsStreaming(true);
		setIsStreamPaused(false);
		setStreamingReasoning("");
		setLatestRunUsage(undefined);
		setStreamingText("");
		return controller;
	}

	async function captureCursor(id: string) {
		lastEventSeqRef.current = await latestSessionEventSeq(id);
	}

	function start(id: string, runID: string) {
		activeRunRef.current = { sessionId: id, runId: runID, completed: false };
		const controller = new AbortController();
		eventControllerRef.current?.abort();
		eventControllerRef.current = controller;
		setIsStreamPaused(false);
		void subscribeAndFinish(id, controller);
	}

	async function fail(err: unknown, id: string) {
		const request = requestRef.current;
		if (request) setFailedRun({ ...request, error: formatSessionError(err) });
		reset();
		await load(id);
	}

	function pause() {
		eventControllerRef.current?.abort();
		eventControllerRef.current = null;
		setIsStreamPaused(true);
	}

	function resume() {
		const activeRun = activeRunRef.current;
		if (!activeRun || activeRun.completed) return;
		const controller = new AbortController();
		eventControllerRef.current = controller;
		setIsStreamPaused(false);
		void subscribeAndFinish(activeRun.sessionId, controller);
	}

	async function abort(id: string) {
		try {
			await wfetch(`/sessions/${id}/abort`, { method: "POST" });
		} catch (err) {
			console.error("Abort failed", err);
		}
		submissionControllerRef.current?.abort();
		submissionControllerRef.current = null;
		reset();
		await load(id);
	}

	function finishSubmission() {
		submissionControllerRef.current = null;
	}

	return { isStreaming, isStreamPaused, streamingText, streamingReasoning, latestRunUsage, failedRun, toolActivities, begin, captureCursor, start, fail, pause, resume, abort, finishSubmission };
}
