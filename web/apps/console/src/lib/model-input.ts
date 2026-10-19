import type { ModelCall } from "./types";

export type InstructionDiscovery = {
  id: string;
  path: string;
  requested?: string;
  resolved?: string;
  order: number;
};

export function indexInstructionDiscoveries(calls: ModelCall[]) {
  const index = new Map<string, Map<string, InstructionDiscovery>>();
  for (const [order, call] of calls.entries()) {
    for (const source of call.trace?.instructions ?? []) {
      const key = source.tool_use_id
        ? `tool:${source.tool_use_id}`
        : source.call_id
          ? `call:${source.call_id}`
          : undefined;
      if (!key) continue;
      let paths = index.get(key);
      if (!paths) {
        paths = new Map();
        index.set(key, paths);
      }
      if (!paths.has(source.path))
        paths.set(source.path, {
          id: call.id,
          path: source.path,
          requested: source.requested_path,
          resolved: source.resolved_path,
          order,
        });
    }
  }
  return index;
}

export function modelCallChanges(call: ModelCall, previous?: ModelCall): string[] {
  if (!previous) return [];
  const changes: string[] = [];
  const model = (value: ModelCall) =>
    value.model_ref ||
    (value.provider && value.model_id ? `${value.provider}/${value.model_id}` : undefined);
  if (model(call) && model(previous) && model(call) !== model(previous))
    changes.push("Model changed");
  if (call.trace && previous.trace) {
    const instructions = (value: ModelCall) =>
      JSON.stringify(value.trace?.instructions?.map(({ path, sha256 }) => [path, sha256]) ?? []);
    if (instructions(call) !== instructions(previous)) changes.push("Instructions updated");
    if (call.trace.system.sha256 !== previous.trace.system.sha256)
      changes.push("System prompt changed");
    const tools = (value: ModelCall) =>
      JSON.stringify(value.trace?.tools?.map(({ name, schema_hash }) => [name, schema_hash]) ?? []);
    if (tools(call) !== tools(previous)) changes.push("Available tools changed");
  }
  return changes;
}
