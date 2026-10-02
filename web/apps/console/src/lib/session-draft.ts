export type PendingSubmission = {
  requestId: string;
  sessionId: string;
  agentId: string;
  modelRef: string;
  message: string;
};

export type SessionDraft = { text: string; pending?: PendingSubmission };

export function draftKey(sessionId: string, workspace?: string) {
  return `wingman-draft:${sessionId === "new" ? `new:${workspace ?? ""}` : sessionId}`;
}

export function readDraft(key: string): SessionDraft {
  try {
    const value = JSON.parse(localStorage.getItem(key) ?? "null");
    if (value && typeof value.text === "string") {
      const pending = value.pending;
      return {
        text: value.text,
        pending:
          pending &&
          [
            pending.requestId,
            pending.sessionId,
            pending.agentId,
            pending.modelRef,
            pending.message,
          ].every((item) => typeof item === "string")
            ? pending
            : undefined,
      };
    }
  } catch {
    /* Storage can be unavailable or cleared by the browser. */
  }
  return { text: "" };
}

export function saveDraft(key: string, draft: SessionDraft) {
  try {
    if (!draft.text && !draft.pending) localStorage.removeItem(key);
    else localStorage.setItem(key, JSON.stringify(draft));
  } catch {
    /* Sending and editing still work when storage is unavailable. */
  }
}

export function matchingSubmission(
  pending: PendingSubmission | null | undefined,
  sessionId: string,
  agentId: string,
  modelRef: string,
  message: string,
) {
  return pending?.sessionId === sessionId &&
    pending.agentId === agentId &&
    pending.modelRef === modelRef &&
    pending.message === message
    ? pending.requestId
    : undefined;
}
