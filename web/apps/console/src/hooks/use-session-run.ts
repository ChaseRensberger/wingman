import { useEffect, useEffectEvent, useRef, useState, type Dispatch, type SetStateAction } from "react";
import { type SessionEvent, type UnknownSessionEvent } from "@wingman-actor/client";

import { client } from "@/lib/client";
import { formatSessionError } from "@/lib/session-detail";
import { reduceToolActivity } from "@/lib/tool-activity-state";
import type { Message, PermissionRequest, Session, SessionRun, ToolActivity, Usage } from "@/lib/types";

export type FailedRun = { message: string; agentId: string; modelRef: string; error: string };
export type SessionRunRequest = Omit<FailedRun, "error">;

const terminalRunEvents = new Set(["session.run.completed", "session.run.failed", "session.run.aborted"]);
const initialRetryDelayMs = 250;
const maxRetryDelayMs = 5_000;

export type SessionStreamControl = "synchronized" | "resync_required";
export type SessionStreamResult = { resync: boolean; synchronized: boolean; terminalError?: string };

type SessionStreamRecoveryOptions = {
	isCurrent: () => boolean;
	isCompleted: () => boolean;
	subscribe: () => Promise<SessionStreamResult>;
	clearVolatileStreamState: () => void;
	reload: () => Promise<SessionRun | undefined>;
	finish: (error?: string) => Promise<void>;
	resync: () => Promise<void>;
	waitForRetry: (delay: number) => Promise<void>;
	reportFailure?: (message: string, error: unknown) => void;
};

export function latestActiveSessionRun(runs: readonly SessionRun[]): SessionRun | undefined {
	return runs.reduce<SessionRun | undefined>((latest, run) => {
		if (run.status !== "queued" && run.status !== "running") return latest;
		if (!latest || run.sequence > latest.sequence) return run;
		return latest;
	}, undefined);
}

export function isTerminalSessionRunEvent(type: string): boolean {
	return terminalRunEvents.has(type);
}

export function sessionRunEventError(data: { error_message?: string; error?: string; error_type?: string }): string {
	return typeof data.error_message === "string" ? data.error_message : typeof data.error === "string" ? data.error : typeof data.error_type === "string" ? data.error_type : "Run failed";
}

export function sessionStreamControl(type: string): SessionStreamControl | undefined {
	if (type === "session.events.synchronized") return "synchronized";
	if (type === "session.events.resync_required") return "resync_required";
}

export function sessionRunRetryDelay(attempt: number): number {
	return Math.min(initialRetryDelayMs * 2 ** Math.max(0, attempt), maxRetryDelayMs);
}

export function reconcileSessionEventSeq(current: number, authoritative: number, resync: boolean): number {
	return resync ? authoritative : Math.max(current, authoritative);
}

export function terminalSessionRunError(run: SessionRun): string | undefined {
	if (run.status === "completed" || run.status === "queued" || run.status === "running") return undefined;
	return run.error_message ?? run.error_type ?? `Run ${run.status}`;
}

export async function maintainSessionRunStream(options: SessionStreamRecoveryOptions) {
	let retryAttempt = 0;
	while (options.isCurrent()) {
		let result: SessionStreamResult | undefined;
		try {
			result = await options.subscribe();
		} catch (err) {
			if ((err as Error).name === "AbortError" || !options.isCurrent()) return;
			(options.reportFailure ?? console.error)("Event stream failed", err);
		}
		if (!options.isCurrent()) return;
		if (options.isCompleted()) {
			await options.finish(result?.terminalError);
			return;
		}
		options.clearVolatileStreamState();
		if (result?.resync) await options.resync();
		let run: SessionRun | undefined;
		try {
			run = await options.reload();
		} catch (err) {
			if ((err as Error).name === "AbortError" || !options.isCurrent()) return;
			(options.reportFailure ?? console.error)("Failed to reload session run", err);
		}
		if (!options.isCurrent()) return;
		if (run && run.status !== "queued" && run.status !== "running") {
			await options.finish(terminalSessionRunError(run));
			return;
		}
		if (result?.synchronized) retryAttempt = 0;
		await options.waitForRetry(sessionRunRetryDelay(retryAttempt++));
	}
}

type PermissionRequestAction = { type: "loaded"; requests: readonly PermissionRequest[] } | { type: "requested"; request: PermissionRequest } | { type: "resolved"; request: PermissionRequest };

function isTerminalPermissionRequest(request: PermissionRequest): boolean {
	return request.status !== "pending";
}

function newerPermissionRequest(previous: PermissionRequest, next: PermissionRequest): PermissionRequest {
	const comparison = next.updated_at.localeCompare(previous.updated_at);
	if (comparison > 0) return next;
	if (comparison < 0) return previous;
	return isTerminalPermissionRequest(next) && !isTerminalPermissionRequest(previous) ? next : previous;
}

export function reducePermissionRequestRecords(previous: ReadonlyMap<string, PermissionRequest>, action: PermissionRequestAction): Map<string, PermissionRequest> {
	const records = new Map(previous);
	const requests = action.type === "loaded" ? action.requests : [action.request];
	for (const request of requests) {
		const current = records.get(request.id);
		records.set(request.id, current ? newerPermissionRequest(current, request) : request);
	}
	return records;
}

export function pendingPermissionRequests(records: ReadonlyMap<string, PermissionRequest>): PermissionRequest[] {
	return [...records.values()]
		.filter((request) => request.status === "pending")
		.toSorted((a, b) => a.created_at.localeCompare(b.created_at) || a.id.localeCompare(b.id));
}

export function addPermissionReplyInFlight(previous: ReadonlySet<string>, requestID: string): Set<string> {
	if (previous.has(requestID)) return new Set(previous);
	return new Set(previous).add(requestID);
}

function isPermissionRequest(data: unknown): data is PermissionRequest {
	if (!data || typeof data !== "object") return false;
	const request = data as Record<string, unknown>;
	return typeof request.id === "string"
		&& typeof request.session_id === "string"
		&& typeof request.action === "string"
		&& Array.isArray(request.resources)
		&& request.resources.every((resource) => typeof resource === "string")
		&& (request.status === "pending" || request.status === "approved" || request.status === "rejected" || request.status === "timed_out" || request.status === "interrupted")
		&& typeof request.created_at === "string"
		&& typeof request.updated_at === "string";
}

type Options = {
	sessionId: string;
	loadSession: (id?: string) => Promise<void>;
	setSession: Dispatch<SetStateAction<Session | null>>;
};

async function latestSessionEventSeq(sessionId: string): Promise<number> {
	let after = 0;
	for (;;) {
		const page = await client.sessions.listEvents(sessionId, { after, limit: 500 }) as { data?: UnknownSessionEvent[] | null; has_more?: boolean };
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
	const [streamingText, setStreamingText] = useState("");
	const [streamingReasoning, setStreamingReasoning] = useState("");
	const [latestRunUsage, setLatestRunUsage] = useState<Usage>();
	const [failedRun, setFailedRun] = useState<FailedRun | null>(null);
	const [toolActivities, setToolActivities] = useState<Map<string, ToolActivity>>(() => new Map());
	const [permissionRequestRecords, setPermissionRequestRecords] = useState<Map<string, PermissionRequest>>(() => new Map());
	const [permissionRepliesInFlight, setPermissionRepliesInFlight] = useState<Set<string>>(() => new Set());
	const permissionRepliesInFlightRef = useRef(new Set<string>());

	function reset() {
		eventControllerRef.current?.abort();
		eventControllerRef.current = null;
		activeRunRef.current = null;
		requestRef.current = null;
		setIsStreaming(false);
		setStreamingReasoning("");
		setStreamingText("");
	}

	function clearVolatileStreamState() {
		setStreamingReasoning("");
		setStreamingText("");
		setToolActivities(new Map());
	}

	useEffect(() => {
		// Creating a draft session changes the route before its admission request finishes.
		if (submissionControllerRef.current) return;
		lastEventSeqRef.current = 0;
		setFailedRun(null);
		setToolActivities(new Map());
		setPermissionRequestRecords(new Map());
		permissionRepliesInFlightRef.current = new Set();
		setPermissionRepliesInFlight(new Set());
		reset();
		return () => eventControllerRef.current?.abort();
	}, [sessionId]);

	async function reloadPermissionRequests(id: string, signal?: AbortSignal) {
		const requests = await client.sessions.permissionRequests.list(id) as PermissionRequest[];
		if (signal?.aborted) return;
		setPermissionRequestRecords((previous) => reducePermissionRequestRecords(previous, { type: "loaded", requests }));
	}

	useEffect(() => {
		if (sessionId === "new") return;
		const controller = new AbortController();
		void reloadPermissionRequests(sessionId, controller.signal).catch((err) => {
			if ((err as Error).name !== "AbortError") console.error("Failed to load permission requests", err);
		});
		return () => controller.abort();
	}, [sessionId]);

	useEffect(() => {
		if (sessionId === "new") return;
		const controller = new AbortController();
		let cancelled = false;

		async function recover() {
			try {
				const runs = await client.sessions.runs.list(sessionId) as SessionRun[];
				const run = latestActiveSessionRun(runs);
				if (cancelled || !run || activeRunRef.current || submissionControllerRef.current) return;
				setIsStreaming(true);
				start(sessionId, run.id);
			} catch (err) {
				if ((err as Error).name !== "AbortError" && !cancelled) console.error("Failed to recover session run", err);
			}
		}

		void recover();
		return () => {
			cancelled = true;
			controller.abort();
		};
	}, [sessionId]);

	function applySessionEvent(ev: SessionEvent): string | undefined {
		if (typeof ev.cursor?.seq === "number" && ev.cursor.seq > lastEventSeqRef.current) lastEventSeqRef.current = ev.cursor.seq;
		if ((ev.type === "session.permission.requested" || ev.type === "session.permission.resolved") && isPermissionRequest(ev.data)) {
			const request = ev.data;
			setPermissionRequestRecords((previous) => reducePermissionRequestRecords(previous, ev.type === "session.permission.requested" ? { type: "requested", request } : { type: "resolved", request }));
			return;
		}
		if ("run_id" in ev.data && activeRunRef.current?.runId && ev.data.run_id !== activeRunRef.current.runId) return;
		if (ev.type === "session.tool.input.delta") {
			const callID = ev.data.call_id ?? "";
			const delta = ev.data.delta;
			if (!callID || !delta) return;
			setToolActivities((previous) => reduceToolActivity(previous, { type: "input", ...ev.data, delta }));
			return;
		}
		if (ev.type === "session.tool.progress") {
			setToolActivities((previous) => reduceToolActivity(previous, { type: "progress", ...ev.data }));
			return;
		}
		if (ev.type === "session.tool.updated") {
			setToolActivities((previous) => reduceToolActivity(previous, { type: "updated", ...ev.data }));
			return;
		}
		if (ev.type === "session.text.delta") {
			const delta = ev.data.delta;
			if (delta) setStreamingText((previous) => previous + delta);
			return;
		}
		if (ev.type === "session.reasoning.delta") {
			const delta = ev.data.delta;
			if (delta) setStreamingReasoning((previous) => previous + delta);
			return;
		}
		if (ev.type === "session.reasoning.completed") {
			const text = ev.data.text;
			if (text) setStreamingReasoning(text);
			return;
		}
		if (ev.type === "session.message.created") {
			const message = ev.data.message as Message;
			setSession((previous) => {
				if (!previous) return previous;
				const index = message.id ? previous.history.findIndex((candidate) => candidate.id === message.id) : -1;
				if (index < 0) return { ...previous, history: [...previous.history, message] };
				if ((previous.history[index].revision ?? 0) > (message.revision ?? 0)) return previous;
				const history = [...previous.history];
				history[index] = message;
				return { ...previous, history };
			});
			if (message.role === "assistant") {
				setStreamingText("");
				setStreamingReasoning("");
			}
			return;
		}
		if (ev.type === "session.run.completed" || ev.type === "session.run.failed" || ev.type === "session.run.aborted") {
			if (activeRunRef.current?.runId && ev.data.run_id !== activeRunRef.current.runId) return;
			if (ev.type === "session.run.completed") {
				const usage = ev.data.usage;
				if (usage) setLatestRunUsage(usage);
			}
			if (activeRunRef.current) activeRunRef.current = { ...activeRunRef.current, completed: true };
			setIsStreaming(false);
			return ev.type === "session.run.completed" ? undefined : sessionRunEventError(ev.data);
		}
	}

	async function subscribe(id: string, signal: AbortSignal): Promise<SessionStreamResult> {
		const after = lastEventSeqRef.current;
		let synchronized = false;
		for await (const parsed of client.sessions.streamEvents(id, { after, lastEventID: after || undefined, signal })) {
			if (signal.aborted || activeRunRef.current?.sessionId !== id) return { resync: false, synchronized };
			const control = sessionStreamControl(parsed.event.type);
			if (control === "synchronized") {
				synchronized = true;
				continue;
			}
			if (control === "resync_required") return { resync: true, synchronized };
			if (!parsed.known) continue;
			const terminalError = applySessionEvent(parsed.event);
			if (activeRunRef.current?.completed) return { resync: false, synchronized, terminalError };
		}
		return { resync: false, synchronized };
	}

	function isCurrentSubscription(id: string, controller: AbortController) {
		return !controller.signal.aborted && activeRunRef.current?.sessionId === id && eventControllerRef.current === controller;
	}

	async function reloadRun(id: string, runID: string, controller: AbortController): Promise<SessionRun | undefined> {
		await Promise.all([load(id), reloadPermissionRequests(id, controller.signal)]);
		if (!isCurrentSubscription(id, controller)) return;
		const run = await client.sessions.runs.get(id, runID) as SessionRun;
		return isCurrentSubscription(id, controller) ? run : undefined;
	}

	async function finishRun(id: string, controller: AbortController, error?: string) {
		if (!isCurrentSubscription(id, controller)) return;
		const request = requestRef.current;
		if (request && error) setFailedRun({ ...request, error });
		reset();
		await load(id);
	}

	function waitForRetry(delay: number, signal: AbortSignal): Promise<void> {
		return new Promise((resolve) => {
			const finish = () => {
				window.clearTimeout(timer);
				signal.removeEventListener("abort", finish);
				resolve();
			};
			const timer = window.setTimeout(finish, delay);
			signal.addEventListener("abort", finish, { once: true });
		});
	}

	async function subscribeAndFinish(id: string, controller: AbortController) {
		await maintainSessionRunStream({
			isCurrent: () => isCurrentSubscription(id, controller),
			isCompleted: () => activeRunRef.current?.completed ?? false,
			subscribe: () => subscribe(id, controller.signal),
			clearVolatileStreamState,
			reload: () => {
				const runID = activeRunRef.current?.runId;
				return runID ? reloadRun(id, runID, controller) : Promise.resolve(undefined);
			},
			finish: (error) => finishRun(id, controller, error),
			resync: async () => {
				lastEventSeqRef.current = reconcileSessionEventSeq(lastEventSeqRef.current, await latestSessionEventSeq(id), true);
			},
			waitForRetry: (delay) => waitForRetry(delay, controller.signal),
		});
	}

	function begin(request: SessionRunRequest) {
		const controller = new AbortController();
		submissionControllerRef.current = controller;
		requestRef.current = request;
		setFailedRun(null);
		setIsStreaming(true);
		setStreamingReasoning("");
		setLatestRunUsage(undefined);
		setStreamingText("");
		return controller;
	}

	async function captureCursor(id: string) {
		lastEventSeqRef.current = reconcileSessionEventSeq(lastEventSeqRef.current, await latestSessionEventSeq(id), false);
	}

	function start(id: string, runID: string) {
		activeRunRef.current = { sessionId: id, runId: runID, completed: false };
		const controller = new AbortController();
		eventControllerRef.current?.abort();
		eventControllerRef.current = controller;
		void reloadPermissionRequests(id, controller.signal).catch((err) => {
			if ((err as Error).name !== "AbortError") console.error("Failed to load permission requests", err);
		});
		void subscribeAndFinish(id, controller);
	}

	async function replyPermission(requestID: string, response: "once" | "always" | "reject") {
		if (permissionRepliesInFlightRef.current.has(requestID)) return;
		permissionRepliesInFlightRef.current = addPermissionReplyInFlight(permissionRepliesInFlightRef.current, requestID);
		setPermissionRepliesInFlight(permissionRepliesInFlightRef.current);
		try {
			const targetSessionID = activeRunRef.current?.sessionId ?? sessionId;
			const request = await client.sessions.permissionRequests.reply(targetSessionID, requestID, { response }) as PermissionRequest;
			setPermissionRequestRecords((previous) => reducePermissionRequestRecords(previous, { type: "resolved", request }));
		} finally {
			const next = new Set(permissionRepliesInFlightRef.current);
			next.delete(requestID);
			permissionRepliesInFlightRef.current = next;
			setPermissionRepliesInFlight(next);
		}
	}

	async function fail(err: unknown, id: string) {
		const request = requestRef.current;
		if (request) setFailedRun({ ...request, error: formatSessionError(err) });
		reset();
		await load(id);
	}

	async function abort(id: string) {
		try {
			const runID = activeRunRef.current?.runId;
			if (runID) {
				await client.sessions.runs.abort(id, runID);
			} else {
				await client.sessions.abort(id);
			}
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

	const permissionRequests = pendingPermissionRequests(permissionRequestRecords);
	return { isStreaming: isStreaming || permissionRequests.length > 0, streamingText, streamingReasoning, latestRunUsage, failedRun, toolActivities, permissionRequests, permissionRepliesInFlight, replyPermission, begin, captureCursor, start, fail, abort, finishSubmission };
}
