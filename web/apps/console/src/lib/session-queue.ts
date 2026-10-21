import type { SessionEvent } from "@wingman-actor/client";
import type { Agent, Message, SessionRun } from "./types";

export type SessionNotice = Extract<
  SessionEvent,
  { type: "session.agent.switched" | "session.model.switched" }
>;

export function isSessionNotice(event: SessionEvent): event is SessionNotice {
  return event.type === "session.agent.switched" || event.type === "session.model.switched";
}

export function noticeLabel(notice: SessionNotice): string {
  if (notice.type === "session.agent.switched") {
    const { previous_name, previous, name, agent } = notice.data;
    return `Switched agent from ${previous_name || previous} to ${name || agent}`;
  }
  return `Switched model from ${notice.data.previous || "default"} to ${notice.data.model_ref || "default"}`;
}

function pendingChanges(
  previous: Agent | undefined,
  next: Pick<Agent, "id" | "name" | "model_ref">,
): string[] {
  if (!previous) return [];
  const changes: string[] = [];
  if (previous.id !== next.id)
    changes.push(`Agent: ${previous.name || previous.id} → ${next.name || next.id}`);
  if ((previous.model_ref ?? "") !== (next.model_ref ?? ""))
    changes.push(`Model: ${previous.model_ref || "default"} → ${next.model_ref || "default"}`);
  return changes;
}

export function sessionQueue(runs: readonly SessionRun[]) {
  const ordered = [...runs].sort((a, b) => a.sequence - b.sequence);
  let previous: Agent | undefined;
  const queued: Array<{ run: SessionRun; changes: string[] }> = [];
  for (const run of ordered) {
    if (run.status === "queued") queued.push({ run, changes: pendingChanges(previous, run.agent) });
    if (run.status === "queued" || run.started_at) previous = run.agent;
  }
  return queued;
}

export function withStartedRunMessages(
  messages: Message[],
  runs: readonly SessionRun[],
): Message[] {
  const active = runs.find(
    (run) =>
      run.status === "running" ||
      run.status === "awaiting_input" ||
      run.status === "awaiting_completion",
  );
  if (
    !active ||
    active.kind === "action" ||
    messages.some((message) => message.role === "user" && message.metadata?.run_id === active.id)
  )
    return messages;

  // The initial user prompt is saved without a message event. Keep it visible
  // between queue removal and the next authoritative history reload.
  const prompt: Message = {
    id: `run:${active.id}:user`,
    role: "user",
    metadata: { run_id: active.id },
    content: [{ type: "text", text: active.message }],
  };
  const index = messages.findIndex((message) => message.metadata?.run_id === active.id);
  const insertion = index < 0 ? messages.length : index;
  return [...messages.slice(0, insertion), prompt, ...messages.slice(insertion)];
}
