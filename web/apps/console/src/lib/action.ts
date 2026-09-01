import type { PluginAction } from "./types";

export type ActionInvocation = { action: string; arguments: string };

export function actionInvocation(
  text: string,
  actions: readonly PluginAction[],
): ActionInvocation | undefined {
  const match = text.trim().match(/^\/(\S+)(?:\s+([\s\S]*))?$/);
  if (!match) return;
  const command = match[1] ?? "";
  const action = actions.find((candidate) => candidate.command === command);
  if (!action) return;
  return { action: action.id, arguments: (match[2] ?? "").trim() };
}
