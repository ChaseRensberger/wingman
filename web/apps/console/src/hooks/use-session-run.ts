import {
  useCallback,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { type Question, type SessionEvent, type UnknownSessionEvent } from "@wingman-actor/client";

import { client } from "@/lib/client";
import { onPageResume } from "@/lib/connection";
import { mergeQuestions } from "@/lib/questions";
import { formatSessionError } from "@/lib/session-detail";
import { isSessionNotice } from "@/lib/session-queue";
import { reduceToolActivity } from "@/lib/tool-activity-state";
import { reduceStreamingParts, type StreamingPart } from "@/lib/streaming-parts";
import type {
  Message,
  PermissionRequest,
  Session,
  SessionRun,
  ToolActivity,
  Usage,
} from "@/lib/types";

export type FailedRun = {
  message: string;
  agentId: string;
  modelRef: string;
  kind?: "message" | "action";
  action?: string;
  input?: unknown;
  error: string;
};
export type SessionRunRequest = Omit<FailedRun, "error">;

const terminalRunEvents = new Set([
  "session.run.completed",
  "session.run.failed",
  "session.run.aborted",
]);
const initialRetryDelayMs = 250;
const maxRetryDelayMs = 5_000;

export type SessionStreamControl = "synchronized" | "resync_required";
export type SessionStreamResult = {
  resync: boolean;
  synchronized: boolean;
  terminalError?: string;
};

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
  const active = runs.find(
    (run) =>
      run.status === "running" ||
      run.status === "awaiting_input" ||
      run.status === "awaiting_completion",
  );
  if (active) return active;
  return runs.reduce<SessionRun | undefined>((first, run) => {
    if (run.status !== "queued") return first;
    return !first || run.sequence < first.sequence ? run : first;
  }, undefined);
}

export async function followNextSessionRun(options: {
  isCurrent: () => boolean;
  list: () => Promise<SessionRun[]>;
  start: (run: SessionRun) => void;
  waitForRetry: (delay: number) => Promise<void>;
  reportFailure?: (message: string, error: unknown) => void;
}) {
  let attempt = 0;
  while (options.isCurrent()) {
    try {
      const next = latestActiveSessionRun(await options.list());
      if (!options.isCurrent()) return;
      if (next) options.start(next);
      return;
    } catch (err) {
      if (!options.isCurrent()) return;
      (options.reportFailure ?? console.error)("Failed to find next session run", err);
      await options.waitForRetry(sessionRunRetryDelay(attempt++));
    }
  }
}

export function isTerminalSessionRunEvent(type: string): boolean {
  return terminalRunEvents.has(type);
}

export function sessionRunEventError(data: {
  error_message?: string;
  error?: string;
  error_type?: string;
}): string {
  return typeof data.error_message === "string"
    ? data.error_message
    : typeof data.error === "string"
      ? data.error
      : typeof data.error_type === "string"
        ? data.error_type
        : "Run failed";
}

export function sessionStreamControl(type: string): SessionStreamControl | undefined {
  if (type === "session.events.synchronized") return "synchronized";
  if (type === "session.events.resync_required") return "resync_required";
}

export function sessionRunRetryDelay(attempt: number): number {
  return Math.min(initialRetryDelayMs * 2 ** Math.max(0, attempt), maxRetryDelayMs);
}

export function reconcileSessionEventSeq(
  current: number,
  authoritative: number,
  resync: boolean,
): number {
  return resync ? authoritative : Math.max(current, authoritative);
}

export function terminalSessionRunError(run: SessionRun): string | undefined {
  if (
    run.status === "completed" ||
    run.status === "queued" ||
    run.status === "running" ||
    run.status === "awaiting_input" ||
    run.status === "awaiting_completion"
  )
    return undefined;
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
    if (
      run &&
      run.status !== "queued" &&
      run.status !== "running" &&
      run.status !== "awaiting_input" &&
      run.status !== "awaiting_completion"
    ) {
      await options.finish(terminalSessionRunError(run));
      return;
    }
    if (result?.synchronized) retryAttempt = 0;
    await options.waitForRetry(sessionRunRetryDelay(retryAttempt++));
  }
}

type PermissionRequestAction =
  | { type: "loaded"; requests: readonly PermissionRequest[] }
  | { type: "requested"; request: PermissionRequest }
  | { type: "resolved"; request: PermissionRequest };

function isTerminalPermissionRequest(request: PermissionRequest): boolean {
  return request.status !== "pending";
}

function newerPermissionRequest(
  previous: PermissionRequest,
  next: PermissionRequest,
): PermissionRequest {
  const comparison = next.updated_at.localeCompare(previous.updated_at);
  if (comparison > 0) return next;
  if (comparison < 0) return previous;
  return isTerminalPermissionRequest(next) && !isTerminalPermissionRequest(previous)
    ? next
    : previous;
}

export function reducePermissionRequestRecords(
  previous: ReadonlyMap<string, PermissionRequest>,
  action: PermissionRequestAction,
): Map<string, PermissionRequest> {
  const records = new Map(previous);
  const requests = action.type === "loaded" ? action.requests : [action.request];
  for (const request of requests) {
    const current = records.get(request.id);
    records.set(request.id, current ? newerPermissionRequest(current, request) : request);
  }
  return records;
}

export function pendingPermissionRequests(
  records: ReadonlyMap<string, PermissionRequest>,
): PermissionRequest[] {
  return [...records.values()]
    .filter((request) => request.status === "pending")
    .toSorted((a, b) => a.created_at.localeCompare(b.created_at) || a.id.localeCompare(b.id));
}

export function addPermissionReplyInFlight(
  previous: ReadonlySet<string>,
  requestID: string,
): Set<string> {
  if (previous.has(requestID)) return new Set(previous);
  return new Set(previous).add(requestID);
}

function isPermissionRequest(data: unknown): data is PermissionRequest {
  if (!data || typeof data !== "object") return false;
  const request = data as Record<string, unknown>;
  return (
    typeof request.id === "string" &&
    typeof request.session_id === "string" &&
    typeof request.action === "string" &&
    Array.isArray(request.resources) &&
    request.resources.every((resource) => typeof resource === "string") &&
    (request.status === "pending" ||
      request.status === "approved" ||
      request.status === "rejected" ||
      request.status === "timed_out" ||
      request.status === "interrupted") &&
    typeof request.created_at === "string" &&
    typeof request.updated_at === "string"
  );
}

type Options = {
  sessionId: string;
  loadSession: (id?: string) => Promise<void>;
  setSession: Dispatch<SetStateAction<Session | null>>;
};

async function latestSessionEventSeq(sessionId: string): Promise<number> {
  let after = 0;
  for (;;) {
    const page = (await client.sessions.listEvents(sessionId, { after, limit: 500 })) as {
      data?: UnknownSessionEvent[] | null;
      has_more?: boolean;
    };
    const events = page.data ?? [];
    if (events.length === 0) return after;
    after = events.at(-1)?.cursor?.seq ?? after;
    if (!page.has_more) return after;
  }
}

export function useSessionRun({ sessionId, loadSession, setSession }: Options) {
  const load = loadSession;
  const submissionControllerRef = useRef<AbortController | null>(null);
  const submissionSessionRef = useRef(sessionId);
  const eventControllerRef = useRef<AbortController | null>(null);
  const followControllerRef = useRef<AbortController | null>(null);
  const lastEventSeqRef = useRef(0);
  const activeRunRef = useRef<{ sessionId: string; runId?: string; completed: boolean } | null>(
    null,
  );
  const requestRef = useRef<SessionRunRequest | null>(null);
  const submissionRequestRef = useRef<SessionRunRequest | null>(null);
  const currentSessionRef = useRef(sessionId);
  currentSessionRef.current = sessionId;
  const runsRevisionRef = useRef(0);
  const [runs, setRuns] = useState<SessionRun[]>([]);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [resumeRevision, setResumeRevision] = useState(0);
  const [isStreaming, setIsStreaming] = useState(false);
  const [streamingParts, setStreamingParts] = useState<StreamingPart[]>([]);
  const [isReasoningStreaming, setIsReasoningStreaming] = useState(false);
  const [latestRunUsage, setLatestRunUsage] = useState<Usage>();
  const [failedRun, setFailedRun] = useState<FailedRun | null>(null);
  const [toolActivities, setToolActivities] = useState<Map<string, ToolActivity>>(() => new Map());
  const [questionRecords, setQuestionRecords] = useState<Map<string, Question>>(() => new Map());
  const [permissionRequestRecords, setPermissionRequestRecords] = useState<
    Map<string, PermissionRequest>
  >(() => new Map());
  const [permissionRepliesInFlight, setPermissionRepliesInFlight] = useState<Set<string>>(
    () => new Set(),
  );
  const permissionRepliesInFlightRef = useRef(new Set<string>());
  const startRecoveredRun = useEffectEvent((id: string, run: SessionRun) => start(id, run));
  const resumeSession = useEffectEvent(() => {
    if (sessionId === "new") return;
    setResumeRevision((revision) => revision + 1);
    const active = activeRunRef.current;
    if (active?.runId) {
      const controller = new AbortController();
      eventControllerRef.current?.abort();
      eventControllerRef.current = controller;
      void subscribeAndFinish(sessionId, controller);
    } else
      void reloadPermissionRequests(sessionId).catch((err) => {
        console.error("Failed to reload permission requests", err);
      });
  });
  useEffect(() => onPageResume(resumeSession), []);

  function reset() {
    eventControllerRef.current?.abort();
    eventControllerRef.current = null;
    activeRunRef.current = null;
    requestRef.current = null;
    setIsStreaming(false);
    setIsReasoningStreaming(false);
    setStreamingParts([]);
  }

  function clearVolatileStreamState() {
    setIsReasoningStreaming(false);
    setStreamingParts([]);
    setToolActivities(new Map());
  }

  const reloadRuns = useCallback(async (id: string, signal?: AbortSignal) => {
    const revision = ++runsRevisionRef.current;
    const records = (await client.sessions.runs.list(id)) as SessionRun[];
    if (
      !signal?.aborted &&
      revision === runsRevisionRef.current &&
      (currentSessionRef.current === id || currentSessionRef.current === "new")
    )
      setRuns(records);
    return records;
  }, []);

  useEffect(() => {
    // Creating a draft session changes the route before its admission request finishes.
    if (!submissionControllerRef.current || submissionSessionRef.current !== sessionId) {
      submissionControllerRef.current?.abort();
      submissionControllerRef.current = null;
      submissionRequestRef.current = null;
      setIsSubmitting(false);
      lastEventSeqRef.current = 0;
      runsRevisionRef.current++;
      setRuns([]);
      setFailedRun(null);
      setToolActivities(new Map());
      setPermissionRequestRecords(new Map());
      setQuestionRecords(new Map());
      permissionRepliesInFlightRef.current = new Set();
      setPermissionRepliesInFlight(new Set());
      reset();
    }
    return () => {
      eventControllerRef.current?.abort();
      followControllerRef.current?.abort();
    };
  }, [sessionId]);

  const reloadPermissionRequests = useCallback(
    async (id: string, signal?: AbortSignal) => {
      const [requests, questions] = await Promise.all([
        client.sessions.permissionRequests.list(id) as Promise<PermissionRequest[]>,
        client.sessions.questions.list(id),
      ]);
      if (signal?.aborted || id !== sessionId) return;
      setQuestionRecords((previous) => mergeQuestions(previous, questions ?? []));
      setPermissionRequestRecords((previous) =>
        reducePermissionRequestRecords(previous, { type: "loaded", requests }),
      );
    },
    [sessionId],
  );

  useEffect(() => {
    if (sessionId === "new") return;
    const controller = new AbortController();
    void reloadPermissionRequests(sessionId, controller.signal).catch((err) => {
      if ((err as Error).name !== "AbortError")
        console.error("Failed to load permission requests", err);
    });
    return () => controller.abort();
  }, [sessionId, reloadPermissionRequests]);

  useEffect(() => {
    if (sessionId === "new") return;
    const controller = new AbortController();
    const { signal } = controller;

    async function recover() {
      const run = latestActiveSessionRun(await reloadRuns(sessionId, signal));
      if (signal.aborted || !run || activeRunRef.current || submissionControllerRef.current) return;
      setIsStreaming(true);
      startRecoveredRun(sessionId, run);
    }

    async function watch() {
      let after = 0;
      let retry = 0;
      try {
        after = await latestSessionEventSeq(sessionId);
        if (signal.aborted) return;
        // Capture the cursor first so changes during the reload are replayed.
        if (resumeRevision > 0 && !submissionControllerRef.current) await load(sessionId);
        await reloadPermissionRequests(sessionId, signal);
        if (signal.aborted) return;
        await recover();
      } catch (err) {
        if (!signal.aborted) console.error("Failed to recover session run", err);
      }
      while (!signal.aborted) {
        try {
          for await (const parsed of client.sessions.streamEvents(sessionId, {
            after,
            lastEventID: after || undefined,
            signal,
          })) {
            if (signal.aborted) return;
            if (parsed.event.type === "session.events.resync_required") {
              after = await latestSessionEventSeq(sessionId);
              await load(sessionId);
              await reloadPermissionRequests(sessionId, signal);
              await recover();
              break;
            }
            if (typeof parsed.event.cursor?.seq === "number")
              after = Math.max(after, parsed.event.cursor.seq);
            retry = 0;
            if (parsed.known && parsed.event.type.startsWith("session.question.")) {
              setQuestionRecords((previous) =>
                mergeQuestions(previous, [parsed.event.data as Question]),
              );
            }
            if (parsed.known && parsed.event.type.startsWith("session.run.")) {
              await recover();
            }
            if (parsed.known && isSessionNotice(parsed.event)) {
              recordNotice(parsed.event);
            }
            if (!parsed.known || activeRunRef.current || submissionControllerRef.current) continue;
            if (
              parsed.event.type === "session.run.queued" ||
              parsed.event.type === "session.run.started" ||
              parsed.event.type === "session.run.input"
            ) {
              await load(sessionId);
              await recover();
            } else if (parsed.event.type === "session.message.created") {
              await load(sessionId);
            }
          }
          if (signal.aborted) return;
          await load(sessionId);
          await recover();
        } catch (err) {
          if (signal.aborted) return;
          console.error("Session event stream failed", err);
        }
        await waitForRetry(sessionRunRetryDelay(retry++), signal);
      }
    }

    void watch();
    return () => controller.abort();
  }, [sessionId, load, resumeRevision, reloadPermissionRequests, reloadRuns]);

  const recordNotice = useEffectEvent((event: SessionEvent) => {
    if (!isSessionNotice(event)) return;
    setSession((previous) => {
      if (!previous || previous.id !== event.cursor?.session_id) return previous;
      const notices = previous.notices ?? [];
      if (notices.some((notice) => notice.id === event.id)) return previous;
      return {
        ...previous,
        notices: [...notices, event].sort((a, b) => (a.cursor?.seq ?? 0) - (b.cursor?.seq ?? 0)),
      };
    });
  });

  function applySessionEvent(ev: SessionEvent): string | undefined {
    if (typeof ev.cursor?.seq === "number" && ev.cursor.seq > lastEventSeqRef.current)
      lastEventSeqRef.current = ev.cursor.seq;
    if (isSessionNotice(ev)) {
      recordNotice(ev);
      return;
    }
    if (ev.type.startsWith("session.question.")) {
      setQuestionRecords((previous) => mergeQuestions(previous, [ev.data as Question]));
      return;
    }
    if (
      (ev.type === "session.permission.requested" || ev.type === "session.permission.resolved") &&
      isPermissionRequest(ev.data)
    ) {
      const request = ev.data;
      setPermissionRequestRecords((previous) =>
        reducePermissionRequestRecords(
          previous,
          ev.type === "session.permission.requested"
            ? { type: "requested", request }
            : { type: "resolved", request },
        ),
      );
      return;
    }
    if (
      "run_id" in ev.data &&
      activeRunRef.current?.runId &&
      ev.data.run_id !== activeRunRef.current.runId
    )
      return;
    setStreamingParts((previous) => reduceStreamingParts(previous, ev));
    if (ev.type === "session.tool.input.delta") {
      const callID = ev.data.call_id ?? "";
      const delta = ev.data.delta;
      if (!callID || !delta) return;
      setIsReasoningStreaming(false);
      setToolActivities((previous) =>
        reduceToolActivity(previous, { type: "input", ...ev.data, delta }),
      );
      return;
    }
    if (ev.type === "session.tool.progress") {
      setIsReasoningStreaming(false);
      setToolActivities((previous) =>
        reduceToolActivity(previous, { type: "progress", ...ev.data }),
      );
      return;
    }
    if (ev.type === "session.tool.updated") {
      setIsReasoningStreaming(false);
      setToolActivities((previous) =>
        reduceToolActivity(previous, { type: "updated", ...ev.data }),
      );
      return;
    }
    if (ev.type === "session.text.delta") {
      const delta = ev.data.delta;
      if (delta) {
        setIsReasoningStreaming(false);
      }
      return;
    }
    if (ev.type === "session.reasoning.delta") {
      const delta = ev.data.delta;
      if (delta) {
        setIsReasoningStreaming(true);
      }
      return;
    }
    if (ev.type === "session.reasoning.completed") {
      setIsReasoningStreaming(false);
      return;
    }
    if (ev.type === "session.message.created") {
      const message = {
        ...ev.data.message,
        metadata: { ...ev.data.message.metadata, run_id: ev.data.run_id },
      } as Message;
      setSession((previous) => {
        if (!previous) return previous;
        const index = message.id
          ? previous.history.findIndex((candidate) => candidate.id === message.id)
          : -1;
        if (index < 0) return { ...previous, history: [...previous.history, message] };
        if ((previous.history[index].revision ?? 0) > (message.revision ?? 0)) return previous;
        const history = [...previous.history];
        history[index] = message;
        return { ...previous, history };
      });
      if (message.role === "assistant") {
        setIsReasoningStreaming(false);
      }
      return;
    }
    if (
      ev.type === "session.run.completed" ||
      ev.type === "session.run.failed" ||
      ev.type === "session.run.aborted"
    ) {
      if (activeRunRef.current?.runId && ev.data.run_id !== activeRunRef.current.runId) return;
      if (ev.type === "session.run.completed") {
        const usage = ev.data.usage;
        if (usage) setLatestRunUsage(usage);
      }
      if (activeRunRef.current) activeRunRef.current = { ...activeRunRef.current, completed: true };
      setIsStreaming(false);
      setIsReasoningStreaming(false);
      return ev.type === "session.run.completed" ? undefined : sessionRunEventError(ev.data);
    }
  }

  async function subscribe(id: string, signal: AbortSignal): Promise<SessionStreamResult> {
    const after = lastEventSeqRef.current;
    let synchronized = false;
    for await (const parsed of client.sessions.streamEvents(id, {
      after,
      lastEventID: after || undefined,
      signal,
    })) {
      if (signal.aborted || activeRunRef.current?.sessionId !== id)
        return { resync: false, synchronized };
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
    return (
      !controller.signal.aborted &&
      activeRunRef.current?.sessionId === id &&
      eventControllerRef.current === controller
    );
  }

  async function reloadRun(
    id: string,
    runID: string,
    controller: AbortController,
  ): Promise<SessionRun | undefined> {
    await Promise.all([load(id), reloadPermissionRequests(id, controller.signal)]);
    if (!isCurrentSubscription(id, controller)) return;
    const run = (await client.sessions.runs.get(id, runID)) as SessionRun;
    return isCurrentSubscription(id, controller) ? run : undefined;
  }

  async function finishRun(id: string, controller: AbortController, error?: string) {
    if (!isCurrentSubscription(id, controller)) return;
    const request = requestRef.current;
    if (request && error) setFailedRun({ ...request, error });
    reset();
    await Promise.all([load(id), reloadPermissionRequests(id)]);
    if (sessionId !== id || submissionControllerRef.current || activeRunRef.current) return;
    const followController = new AbortController();
    followControllerRef.current?.abort();
    followControllerRef.current = followController;
    const isCurrent = () =>
      !followController.signal.aborted &&
      followControllerRef.current === followController &&
      !activeRunRef.current &&
      !submissionControllerRef.current;
    try {
      await followNextSessionRun({
        isCurrent,
        list: () => reloadRuns(id),
        start: (next) => start(id, next),
        waitForRetry: (delay) => waitForRetry(delay, followController.signal),
      });
    } finally {
      if (followControllerRef.current === followController) followControllerRef.current = null;
    }
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
        lastEventSeqRef.current = reconcileSessionEventSeq(
          lastEventSeqRef.current,
          await latestSessionEventSeq(id),
          true,
        );
      },
      waitForRetry: (delay) => waitForRetry(delay, controller.signal),
    });
  }

  function begin(request: SessionRunRequest) {
    if (submissionControllerRef.current) return null;
    const controller = new AbortController();
    submissionControllerRef.current = controller;
    submissionSessionRef.current = sessionId;
    submissionRequestRef.current = request;
    setIsSubmitting(true);
    setFailedRun(null);
    if (!activeRunRef.current) setIsStreaming(true);
    return controller;
  }

  async function captureCursor(id: string) {
    if (activeRunRef.current) return;
    const cursor = await latestSessionEventSeq(id);
    if (activeRunRef.current) return;
    lastEventSeqRef.current = reconcileSessionEventSeq(lastEventSeqRef.current, cursor, false);
  }

  function start(id: string, run: SessionRun) {
    followControllerRef.current?.abort();
    activeRunRef.current = { sessionId: id, runId: run.id, completed: false };
    requestRef.current = requestForRun(run);
    clearVolatileStreamState();
    setLatestRunUsage(undefined);
    setIsStreaming(true);
    const controller = new AbortController();
    eventControllerRef.current?.abort();
    eventControllerRef.current = controller;
    void reloadPermissionRequests(id, controller.signal).catch((err) => {
      if ((err as Error).name !== "AbortError")
        console.error("Failed to load permission requests", err);
    });
    void subscribeAndFinish(id, controller);
  }

  function requestForRun(run: SessionRun): SessionRunRequest {
    return {
      message: run.message,
      agentId: run.agent.id,
      modelRef: run.agent.model_ref ?? "",
      kind: run.kind,
      action: run.action,
      input: run.input,
    };
  }

  async function replyPermission(requestID: string, response: "once" | "always" | "reject") {
    if (permissionRepliesInFlightRef.current.has(requestID)) return;
    permissionRepliesInFlightRef.current = addPermissionReplyInFlight(
      permissionRepliesInFlightRef.current,
      requestID,
    );
    setPermissionRepliesInFlight(permissionRepliesInFlightRef.current);
    try {
      const targetSessionID = activeRunRef.current?.sessionId ?? sessionId;
      const request = (await client.sessions.permissionRequests.reply(targetSessionID, requestID, {
        response,
      })) as PermissionRequest;
      setPermissionRequestRecords((previous) =>
        reducePermissionRequestRecords(previous, { type: "resolved", request }),
      );
    } finally {
      const next = new Set(permissionRepliesInFlightRef.current);
      next.delete(requestID);
      permissionRepliesInFlightRef.current = next;
      setPermissionRepliesInFlight(next);
    }
  }

  async function fail(err: unknown, id: string) {
    const request = submissionRequestRef.current;
    if (request) setFailedRun({ ...request, error: formatSessionError(err) });
    if (!activeRunRef.current) setIsStreaming(false);
    await load(id);
  }

  async function accept(id: string, runID: string) {
    try {
      const records = await reloadRuns(id);
      if (currentSessionRef.current !== id && currentSessionRef.current !== "new") return;
      const admitted = records.find((run) => run.id === runID);
      const error = admitted && terminalSessionRunError(admitted);
      if (admitted && error) setFailedRun({ ...requestForRun(admitted), error });
      const next = latestActiveSessionRun(records);
      if (!activeRunRef.current && next) start(id, next);
      if (!activeRunRef.current && !next) setIsStreaming(false);
      await load(id);
    } catch (err) {
      console.error("Failed to refresh accepted session run", err);
    }
  }

  async function abort(id: string) {
    const runID = activeRunRef.current?.runId;
    if (!runID) return;
    try {
      await client.sessions.runs.abort(id, runID);
    } catch (err) {
      console.error("Abort failed", err);
      throw err;
    }
    if (activeRunRef.current?.runId === runID) reset();
    await load(id);
    setResumeRevision((revision) => revision + 1);
  }

  function finishSubmission(controller?: AbortController) {
    if (controller && submissionControllerRef.current !== controller) return;
    submissionControllerRef.current = null;
    submissionRequestRef.current = null;
    setIsSubmitting(false);
    setResumeRevision((revision) => revision + 1);
  }

  const permissionRequests = pendingPermissionRequests(permissionRequestRecords);
  return {
    runs,
    isSubmitting,
    isStreaming: isStreaming || permissionRequests.length > 0,
    streamingParts,
    streamingText: streamingParts
      .flatMap((part) => (part.type === "text" ? [part.text] : []))
      .join(""),
    isReasoningStreaming: isStreaming && isReasoningStreaming,
    latestRunUsage,
    failedRun,
    toolActivities,
    questions: [...questionRecords.values()].filter((q) => q.status === "pending"),
    replyQuestion: async (
      questionID: string,
      status: "answered" | "dismissed",
      answers?: string[][],
    ) => {
      const resolved = await client.sessions.questions.reply(sessionId, questionID, {
        status,
        answers,
      });
      setQuestionRecords((previous) => mergeQuestions(previous, [resolved]));
    },
    permissionRequests,
    permissionRepliesInFlight,
    replyPermission,
    begin,
    attachSession: (id: string) => {
      submissionSessionRef.current = id;
    },
    captureCursor,
    accept,
    fail,
    abort,
    finishSubmission,
  };
}
