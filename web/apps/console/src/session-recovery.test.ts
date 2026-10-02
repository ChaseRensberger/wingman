import { expect, test } from "bun:test";
import ts from "typescript";
import { readFileSync } from "node:fs";

import * as connection from "./lib/connection";
import * as sessionRuns from "./hooks/use-session-run";
import { matchingSubmission, type SessionDraft } from "./lib/session-draft";

// Exercise the real private handlers with controlled hooks and network timing.
function handler(path: string, name: string, dependencies: Record<string, unknown>) {
  const file = ts.createSourceFile(
    path,
    readFileSync(new URL(path, import.meta.url), "utf8"),
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  let declaration: ts.FunctionDeclaration | undefined;
  function visit(node: ts.Node) {
    if (ts.isFunctionDeclaration(node) && node.name?.text === name) declaration = node;
    ts.forEachChild(node, visit);
  }
  visit(file);
  if (!declaration) throw new Error(`Missing ${name} in ${path}`);
  const { outputText } = ts.transpileModule(declaration.getText(file).replace(/^export /, ""), {
    compilerOptions: { target: ts.ScriptTarget.ESNext, jsx: ts.JsxEmit.React },
  });
  return new Function(...Object.keys(dependencies), `${outputText}; return ${name};`)(
    ...Object.values(dependencies),
  );
}

test.each(["unchanged", "new draft", "edited pending", "new request"])(
  "recovered admission keeps its identity and preserves newer drafts: %s",
  async (change) => {
    const pending = {
      requestId: "req_original",
      sessionId: "ses_1",
      agentId: "agent_1",
      modelRef: "provider/model",
      message: "perform task",
    };
    const pendingSubmissionRef = { current: pending as typeof pending | null };
    let savedDraft: SessionDraft = { text: pending.message, pending };
    let messageText = pending.message;
    const cursor = Promise.withResolvers<void>();
    const admissionReady = Promise.withResolvers<void>();
    const admission = Promise.withResolvers<void>();
    const admitted: string[] = [];
    const send = handler("./routes/sessions/$sessionId.tsx", "handleSend", {
      messageText: pending.message,
      selectedAgent: pending.agentId,
      selectedProvider: "provider",
      selectedModel: "model",
      selectedVariant: null,
      availableAgents: [{ id: pending.agentId }],
      actions: [],
      macros: [],
      session: { id: pending.sessionId, history: [] },
      sessionId: pending.sessionId,
      storageKey: "draft",
      isDraft: false,
      pendingSubmissionRef,
      activeSessionIdRef: { current: pending.sessionId },
      actionInvocation: () => null,
      macroInvocation: () => null,
      shouldAutoGenerateTitle: () => false,
      titleSourceMessage: () => "",
      persistLastAgentId: () => {},
      persistLastModelRef: () => {},
      readDraft: () => savedDraft,
      saveDraft: (_key: string, draft: SessionDraft) => {
        savedDraft = draft;
      },
      setMessageText: (update: string | ((text: string) => string)) => {
        messageText = typeof update === "function" ? update(messageText) : update;
      },
      transcriptScroll: { reset: () => {} },
      setSession: () => {},
      buildUserMessage: () => ({}),
      buildModelRef: () => pending.modelRef,
      run: {
        begin: () => new AbortController(),
        captureCursor: () => cursor.promise,
        finishSubmission: () => {},
        fail: (error: unknown) => {
          throw error;
        },
      },
      matchingSubmission,
      newRequestID: () => "req_duplicate",
      draftKey: () => "draft",
      loadSession: async () => {},
      client: {
        sessions: {
          admit: async (_id: string, body: { request_id: string }) => {
            admitted.push(body.request_id);
            admissionReady.resolve();
            await admission.promise;
            return { run_id: "run_original", status: "completed", session_version: 2 };
          },
        },
      },
    });
    const sending = send();
    pendingSubmissionRef.current = null;
    cursor.resolve();
    await admissionReady.promise;
    if (change === "new draft") savedDraft = { text: "draft from another tab" };
    if (change === "edited pending") {
      messageText = "newer text";
      savedDraft = { ...savedDraft, text: messageText };
    }
    if (change === "new request") {
      pendingSubmissionRef.current = { ...pending, requestId: "req_newer" };
      messageText = pending.message;
      savedDraft = { text: messageText, pending: pendingSubmissionRef.current };
    }
    const expectedText = change === "unchanged" ? "" : savedDraft.text;
    const expectedPending = change === "new request" ? savedDraft.pending : undefined;
    const expectedComposerText = messageText;
    admission.resolve();
    await sending;
    expect(admitted).toEqual(["req_original"]);
    expect(savedDraft.text).toBe(expectedText);
    expect(savedDraft.pending).toEqual(expectedPending);
    expect(pendingSubmissionRef.current).toEqual(expectedPending ?? null);
    expect(messageText).toBe(expectedComposerText);
  },
);

test("returning to a healthy daemon preserves the route revision", async () => {
  let state = { phase: "connecting", revision: 0, hasConnected: false };
  const document = Object.assign(new EventTarget(), { visibilityState: "visible" });
  const timers = new Map<number, () => Promise<void>>();
  let timerID = 0;
  const window = Object.assign(new EventTarget(), {
    setTimeout: (callback: () => Promise<void>) => {
      timers.set(++timerID, callback);
      return timerID;
    },
    clearTimeout: (id: number) => timers.delete(id),
  });
  let mount: (() => () => void) | undefined;
  const provider = handler("./components/daemon-connection.tsx", "DaemonConnectionProvider", {
    ...connection,
    document,
    window,
    navigator: { onLine: true },
    useRef: (current: unknown) => ({ current }),
    useState: () => [
      state,
      (update: (value: typeof state) => typeof state) => {
        state = update(state);
      },
    ],
    useEffect: (effect: () => () => void) => {
      mount = effect;
    },
    client: { health: { ready: async () => ({ ready: true, instance_id: "instance_1" }) } },
    queryClient: { invalidateQueries: async () => {} },
    toastManager: { add: () => {} },
    daemonConnectionFailureEvent: "connection-failed",
    daemonRestartRequestedEvent: "restart-requested",
    DaemonConnectionContext: {},
    React: { createElement: () => null },
  });
  provider({ children: null });
  const cleanup = mount!();
  async function probe() {
    const next = timers.values().next().value;
    if (!next) throw new Error("Readiness probe was not scheduled");
    await next();
  }
  try {
    await probe();
    expect(state.phase).toBe("live");
    document.visibilityState = "hidden";
    document.dispatchEvent(new Event("visibilitychange"));
    document.visibilityState = "visible";
    document.dispatchEvent(new Event("visibilitychange"));
    await probe();
    expect(state).toMatchObject({ phase: "live", revision: 0, hasConnected: true });
    window.dispatchEvent(new Event("connection-failed"));
    await probe();
    expect(state.revision).toBe(1);
  } finally {
    cleanup();
  }
});

test("session resume listeners handle visibility and cached-page restoration and clean up", () => {
  const document = Object.assign(new EventTarget(), { visibilityState: "hidden" });
  const window = new EventTarget();
  let resumed = 0;
  const subscribe = handler("./lib/connection.ts", "onPageResume", { document, window });
  const cleanup = subscribe(() => resumed++);
  document.dispatchEvent(new Event("visibilitychange"));
  window.dispatchEvent(Object.assign(new Event("pageshow"), { persisted: false }));
  expect(resumed).toBe(0);
  document.visibilityState = "visible";
  document.dispatchEvent(new Event("visibilitychange"));
  window.dispatchEvent(Object.assign(new Event("pageshow"), { persisted: true }));
  expect(resumed).toBe(2);
  cleanup();
  document.dispatchEvent(new Event("visibilitychange"));
  window.dispatchEvent(Object.assign(new Event("pageshow"), { persisted: true }));
  expect(resumed).toBe(2);
});

test("session resume preserves a pending send and replaces a suspended active stream", async () => {
  const effects: Array<() => unknown> = [];
  const states: unknown[] = [];
  const streams: Array<{ isCurrent: () => boolean }> = [];
  let resume: (() => void) | undefined;
  let loads = 0;
  let permissionLoads = 0;
  const hook = handler("./hooks/use-session-run.ts", "useSessionRun", {
    ...sessionRuns,
    useRef: (current: unknown) => ({ current }),
    useState: (initial: unknown) => {
      const index = states.length;
      const value = typeof initial === "function" ? initial() : initial;
      states.push(value);
      return [
        value,
        (update: unknown) => {
          states[index] = typeof update === "function" ? update(states[index]) : update;
        },
      ];
    },
    useEffectEvent: (callback: unknown) => callback,
    useEffect: (effect: () => unknown) => effects.push(effect),
    onPageResume: (callback: () => void) => {
      resume = callback;
      return () => {};
    },
    maintainSessionRunStream: async (options: { isCurrent: () => boolean }) => {
      streams.push(options);
    },
    client: {
      sessions: {
        permissionRequests: {
          list: async () => {
            permissionLoads++;
            return [];
          },
        },
      },
    },
  });
  const run = hook({
    sessionId: "ses_1",
    loadSession: async () => {
      loads++;
    },
    setSession: () => {},
  });
  effects[0]();
  const submission = run.begin({ message: "task", agentId: "agent_1", modelRef: "provider/model" });
  resume!();
  expect(submission.signal.aborted).toBe(false);
  expect(loads).toBe(0);
  expect(states[0]).toBe(1);
  run.finishSubmission();
  run.start("ses_1", "run_1");
  expect(streams[0].isCurrent()).toBe(true);
  resume!();
  expect(streams).toHaveLength(2);
  expect(streams[0].isCurrent()).toBe(false);
  expect(streams[1].isCurrent()).toBe(true);
  expect(states[0]).toBe(2);
  expect(loads).toBe(0);
  expect(permissionLoads).toBeGreaterThan(0);
});

test.each(["before cursor", "after snapshot", "pending send"])(
  "session resume covers changes around history reload: %s",
  async (timing) => {
    const effects: Array<() => () => void> = [];
    const states: unknown[] = [];
    const refs: Array<{ current: unknown }> = [];
    let stateIndex = 0;
    let refIndex = 0;
    let resume: () => void = () => {};
    let serverHistory: string[] = [];
    let displayedHistory: string[] = [];
    let loads = 0;
    let streamAfter: number | undefined;
    const observed = Promise.withResolvers<void>();
    const completeRun = () => {
      serverHistory = ["new message"];
    };
    const hook = handler("./hooks/use-session-run.ts", "useSessionRun", {
      ...sessionRuns,
      useRef: (current: unknown) => refs[refIndex++] ?? (refs[refIndex - 1] = { current }),
      useState: (initial: unknown) => {
        const index = stateIndex++;
        if (!(index in states)) states[index] = typeof initial === "function" ? initial() : initial;
        return [
          states[index],
          (update: unknown) => {
            states[index] = typeof update === "function" ? update(states[index]) : update;
          },
        ];
      },
      useEffectEvent: (callback: unknown) => callback,
      useEffect: (effect: () => () => void) => effects.push(effect),
      onPageResume: (callback: () => void) => {
        resume = callback;
        return () => {};
      },
      latestSessionEventSeq: async () => {
        if (timing === "before cursor") completeRun();
        return serverHistory.length;
      },
      client: {
        sessions: {
          permissionRequests: { list: async () => [] },
          runs: { list: async () => [{ id: "run_1", status: "completed", sequence: 1 }] },
          streamEvents: async function* (
            _id: string,
            options: { after: number; signal: AbortSignal },
          ) {
            streamAfter = options.after;
            if (options.after < serverHistory.length) {
              yield { known: true, event: { type: "session.message.created", cursor: { seq: 1 } } };
            }
            const closed = new Promise<void>((resolve) =>
              options.signal.addEventListener("abort", () => resolve(), { once: true }),
            );
            observed.resolve();
            await closed;
          },
        },
      },
    });
    function render() {
      stateIndex = 0;
      refIndex = 0;
      effects.length = 0;
      return hook({
        sessionId: "ses_1",
        loadSession: async () => {
          displayedHistory = [...serverHistory];
          loads++;
          if (timing === "after snapshot") completeRun();
        },
        setSession: () => {},
      });
    }
    const run = render();
    effects[0]();
    const submission =
      timing === "pending send"
        ? run.begin({ message: "task", agentId: "agent_1", modelRef: "provider/model" })
        : undefined;
    resume();
    render();
    const cleanup = effects[3]();
    try {
      await observed.promise;
      expect(streamAfter).toBe(timing === "before cursor" ? 1 : 0);
      expect(displayedHistory).toEqual(serverHistory);
      expect(loads).toBe(timing === "pending send" ? 0 : timing === "after snapshot" ? 2 : 1);
      if (submission) expect(submission.signal.aborted).toBe(false);
    } finally {
      cleanup();
    }
  },
);
