import type { Question } from "@wingman-actor/client";

export function mergeQuestions(
  previous: ReadonlyMap<string, Question>,
  questions: readonly Question[],
) {
  const next = new Map(previous);
  for (const question of questions) {
    const current = next.get(question.id);
    // A stale list response must not reopen a resolved question.
    if (current && current.status !== "pending" && question.status === "pending") continue;
    next.set(question.id, question);
  }
  return next;
}
