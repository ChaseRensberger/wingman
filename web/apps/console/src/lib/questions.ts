import type { Question } from "@wingman-actor/client";

type QuestionDraft = {
  index: number;
  prompts: {
    prompt: NonNullable<Question["questions"]>[number];
    selected: string[];
    custom: string;
  }[];
};

type QuestionDraftAction =
  | { type: "select"; label: string }
  | { type: "custom"; text: string }
  | { type: "back" }
  | { type: "next" };

export function createQuestionDraft(prompts: Question["questions"]): QuestionDraft {
  return {
    index: 0,
    prompts: (prompts ?? []).map((prompt) => ({ prompt, selected: [], custom: "" })),
  };
}

export function questionAnswers(draft: QuestionDraft): string[][] {
  return draft.prompts.map(({ prompt, selected, custom }) => {
    const text = custom.trim();
    return text ? (prompt.multiple ? [...new Set([...selected, text])] : [text]) : selected;
  });
}

export function reduceQuestionDraft(
  draft: QuestionDraft,
  action: QuestionDraftAction,
): QuestionDraft {
  if (action.type === "back") return { ...draft, index: Math.max(0, draft.index - 1) };
  if (action.type === "next") {
    if (!questionAnswers(draft)[draft.index]?.length) return draft;
    return { ...draft, index: Math.min(draft.prompts.length - 1, draft.index + 1) };
  }
  return {
    ...draft,
    prompts: draft.prompts.map((entry, index) => {
      if (index !== draft.index) return entry;
      if (action.type === "custom") {
        return {
          ...entry,
          custom: action.text,
          selected: !entry.prompt.multiple && action.text.trim() ? [] : entry.selected,
        };
      }
      return {
        ...entry,
        custom: entry.prompt.multiple ? entry.custom : "",
        selected: entry.prompt.multiple
          ? entry.selected.includes(action.label)
            ? entry.selected.filter((value) => value !== action.label)
            : [...entry.selected, action.label]
          : [action.label],
      };
    }),
  };
}

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
