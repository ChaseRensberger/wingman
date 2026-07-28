import { useEffect, useEffectEvent, useRef, useState, type Dispatch, type SetStateAction } from "react";

import { wfetch } from "@/lib/client";
import { formatSessionError, isRecord } from "@/lib/session-detail";
import { readSSE, type SessionEvent } from "@/lib/session-stream";
import { showErrorToast } from "@/lib/toast";
import type { Message, QuestionRequest, Session, ToolActivity, Usage } from "@/lib/types";

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
	const [questions, setQuestions] = useState<QuestionRequest[]>([]);

	function reset() {
		eventControllerRef.current?.abort();
		eventControllerRef.current = null;
		activeRunRef.current = null;
		requestRef.current = null;
		setIsStreaming(false);
		setIsStreamPaused(false);
		setStreamingReasoning("");
		setStreamingText("");
		setQuestions([]);
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
		if (activeRunRef.current?.runId && typeof data.run_id === "string" && data.run_id !== activeRunRef.current.runId) return;
		if (ev.type === "session.tool.input.delta") {
			const callID = typeof data.call_id === "string" ? data.call_id : "";
			const delta = typeof data.delta === "string" ? data.delta : "";
			if (!callID || !delta) return;
			setToolActivities((previous) => {
				const next = new Map(previous);
				const current = next.get(callID);
				const inputText = (current?.input_text ?? "") + delta;
				let input = current?.input;
				try {
					const parsed = JSON.parse(inputText);
					if (isRecord(parsed)) input = parsed;
				} catch {
					// Partial tool input is not valid JSON until the provider finishes it.
				}
				next.set(callID, {
					call_id: callID,
					tool: typeof data.tool === "string" ? data.tool : (current?.tool ?? "tool"),
					status: current?.status ?? "pending",
					...current,
					input,
					input_text: inputText,
				});
				return next;
			});
			return;
		}
		if (ev.type === "session.tool.progress") {
			const callID = typeof data.call_id === "string" ? data.call_id : "";
			if (!callID) return;
			setToolActivities((previous) => {
				const next = new Map(previous);
				const current = next.get(callID);
				const metadata = isRecord(data.metadata)
					? { ...(current?.metadata ?? {}), ...data.metadata }
					: current?.metadata;
				next.set(callID, {
					call_id: callID,
					tool: typeof data.tool === "string" ? data.tool : (current?.tool ?? "tool"),
					status: current?.status ?? "running",
					...current,
					output: (current?.output ?? "") + (typeof data.output_delta === "string" ? data.output_delta : ""),
					metadata,
				});
				return next;
			});
			return;
		}
		if (ev.type === "session.tool.updated") {
			const callID = typeof data.call_id === "string" ? data.call_id : "";
			const tool = typeof data.tool === "string" ? data.tool : "";
			const status = data.status;
			if (!callID || !tool || !["pending", "running", "completed", "error"].includes(String(status))) return;
			setToolActivities((previous) => {
				const next = new Map(previous);
				const current = next.get(callID);
				next.set(callID, {
					...current,
					call_id: callID,
					tool,
					status: status as ToolActivity["status"],
					input: isRecord(data.input) ? data.input : current?.input,
					output: typeof data.output === "string" ? data.output : current?.output,
					metadata: Object.hasOwn(data, "metadata") ? (isRecord(data.metadata) ? data.metadata : undefined) : current?.metadata,
					error: typeof data.error === "string" ? data.error : current?.error,
					started_at: typeof data.started_at === "string" ? data.started_at : current?.started_at,
					completed_at: typeof data.completed_at === "string" ? data.completed_at : current?.completed_at,
					duration_ms: typeof data.duration_ms === "number" ? data.duration_ms : current?.duration_ms,
				});
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
		if (ev.type === "session.question.asked") { setQuestions((current) => [...current, data as unknown as QuestionRequest]); return; }
		if (ev.type === "session.question.replied" || ev.type === "session.question.dismissed") { const id = typeof data.question_id === "string" ? data.question_id : ""; if (id) setQuestions((current) => current.filter((question) => question.id !== id)); return; }
		if (ev.type === "session.message.created") {
			const message = data.message as Message | undefined;
			if (!message) return;
			setSession((previous) => previous ? { ...previous, history: [...previous.history, message] } : previous);
			if (message.role === "assistant") {
				setStreamingText("");
				setStreamingReasoning("");
			}
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
    const response = await fetch(`/sessions/${id}/events?after=${lastEventSeqRef.current}`, { signal });
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
				showErrorToast(new Error(formatSessionError(err)), "Session run failed");
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

	async function attach(id: string) {
		if (activeRunRef.current?.sessionId === id) return;
		activeRunRef.current = { sessionId: id, completed: false };
		setIsStreaming(true);
		await captureCursor(id);
		const controller = new AbortController();
		eventControllerRef.current?.abort();
		eventControllerRef.current = controller;
		void subscribeAndFinish(id, controller);
	}

	async function loadQuestions(id: string) {
		const response = await wfetch(`/sessions/${id}/questions`);
		setQuestions(Array.isArray(response) ? response as QuestionRequest[] : []);
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

	return { isStreaming, isStreamPaused, streamingText, streamingReasoning, latestRunUsage, failedRun, toolActivities, questions, attach, loadQuestions, begin, captureCursor, start, fail, pause, resume, abort, finishSubmission };
}
