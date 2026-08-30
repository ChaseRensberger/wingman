import type { Macro } from "./types";

export type MacroInvocation = { macroID: string; arguments: string };

export function macroInvocation(text: string, macros: readonly Macro[]): MacroInvocation | undefined {
  const match = text.trim().match(/^\/(\S+)(?:\s+([\s\S]*))?$/);
  if (!match) return;
  const macroID = match[1] ?? "";
  if (!macros.some((macro) => macro.id === macroID)) return;
  return { macroID, arguments: (match[2] ?? "").trim() };
}
