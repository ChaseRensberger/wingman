import type { ToolActivity } from "@/lib/types";

type ToolIdentity = {
  tool_use_id?: unknown;
  run_id?: unknown;
  call_id?: unknown;
};

export type ToolActivityEvent = ToolIdentity & {
  type: "input" | "progress" | "updated";
  tool?: unknown;
  status?: unknown;
  delta?: unknown;
  input?: unknown;
  output?: unknown;
  output_delta?: unknown;
  metadata?: unknown;
  error?: unknown;
  started_at?: unknown;
  completed_at?: unknown;
  duration_ms?: unknown;
};

function stringField(value: unknown): string | undefined {
  return typeof value === "string" && value ? value : undefined;
}

function recordField(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

export function transientToolActivityKey(runID: unknown, callID: unknown): string | undefined {
  const run = stringField(runID);
  const call = stringField(callID);
  return run && call ? `${run}:${call}` : call;
}

export function toolActivityKey(identity: ToolIdentity): string | undefined {
  return (
    stringField(identity.tool_use_id) ?? transientToolActivityKey(identity.run_id, identity.call_id)
  );
}

export function normalizeToolStatus(status: unknown): ToolActivity["status"] | undefined {
  switch (status) {
    case "proposed":
    case "authorized":
    case "pending":
      return "pending";
    case "started":
    case "running":
      return "running";
    case "completed":
      return "completed";
    case "failed":
    case "interrupted":
    case "declined":
    case "error":
      return "error";
    default:
      return undefined;
  }
}

export function reduceToolActivity(
  previous: Map<string, ToolActivity>,
  event: ToolActivityEvent,
): Map<string, ToolActivity> {
  const key = toolActivityKey(event);
  if (!key) return previous;
  const fallbackKey = stringField(event.tool_use_id)
    ? transientToolActivityKey(event.run_id, event.call_id)
    : undefined;
  const fallback = fallbackKey ? previous.get(fallbackKey) : undefined;
  const current = previous.get(key) ?? fallback;
  const next = new Map(previous);
  if (fallbackKey && fallbackKey !== key) next.delete(fallbackKey);

  const toolUseID = stringField(event.tool_use_id);
  const callID = stringField(event.call_id) ?? current?.call_id;
  if (!callID) return previous;
  const common = {
    ...current,
    call_id: callID,
    tool_use_id: toolUseID ?? current?.tool_use_id,
    run_id: stringField(event.run_id) ?? current?.run_id,
    tool: stringField(event.tool) ?? current?.tool ?? "tool",
  };

  if (event.type === "input") {
    const inputText = (current?.input_text ?? "") + (stringField(event.delta) ?? "");
    let input = current?.input;
    try {
      const parsed = JSON.parse(inputText);
      if (recordField(parsed)) input = parsed;
    } catch {
      // Provider input arrives as an incomplete JSON stream.
    }
    next.set(key, {
      ...common,
      status: current?.status ?? "pending",
      input,
      input_text: inputText,
    });
    return next;
  }

  if (event.type === "progress") {
    const eventMetadata = recordField(event.metadata);
    const metadata = eventMetadata
      ? { ...(current?.metadata ?? {}), ...eventMetadata }
      : current?.metadata;
    next.set(key, {
      ...common,
      status: current?.status ?? "running",
      output: (current?.output ?? "") + (stringField(event.output_delta) ?? ""),
      metadata,
    });
    return next;
  }

  const status = normalizeToolStatus(event.status);
  if (!status) return previous;
  next.set(key, {
    ...common,
    status,
    input: recordField(event.input) ?? current?.input,
    output: stringField(event.output) ?? current?.output,
    metadata: Object.hasOwn(event, "metadata") ? recordField(event.metadata) : current?.metadata,
    error: stringField(event.error) ?? current?.error,
    started_at: stringField(event.started_at) ?? current?.started_at,
    completed_at: stringField(event.completed_at) ?? current?.completed_at,
    duration_ms: typeof event.duration_ms === "number" ? event.duration_ms : current?.duration_ms,
  });
  return next;
}

export function lookupToolActivity(
  activities: Map<string, ToolActivity>,
  identity: ToolIdentity,
): ToolActivity | undefined {
  const toolUseID = stringField(identity.tool_use_id);
  if (toolUseID) return activities.get(toolUseID);
  const key = toolActivityKey(identity);
  if (key && activities.has(key)) return activities.get(key);
  const callID = stringField(identity.call_id);
  return callID
    ? [...activities.values()].find((activity) => activity.call_id === callID)
    : undefined;
}

export function sameToolActivity(identity: ToolIdentity, activity: ToolActivity): boolean {
  const toolUseID = stringField(identity.tool_use_id);
  if (toolUseID || activity.tool_use_id) return toolUseID === activity.tool_use_id;
  return stringField(identity.call_id) === activity.call_id;
}
