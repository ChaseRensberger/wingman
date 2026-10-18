import { expect, test } from "bun:test";
import type { Question } from "@wingman-actor/client";
import {
  createQuestionDraft,
  mergeQuestions,
  questionAnswers,
  reduceQuestionDraft,
} from "./questions";

const prompts: Question["questions"] = [
  {
    question: "Which color?",
    header: "Color",
    options: [
      { label: "Blue", description: "Blue color" },
      { label: "Green", description: "Green color" },
    ],
  },
  {
    question: "Which features?",
    header: "Features",
    multiple: true,
    options: [
      { label: "Search", description: "Find things" },
      { label: "Export", description: "Save things" },
    ],
  },
];

test("question navigation requires an answer and retains edits in prompt order", () => {
  let draft = createQuestionDraft(prompts);
  expect(questionAnswers(draft)).toEqual([[], []]);
  expect(reduceQuestionDraft(draft, { type: "next" })).toBe(draft);
  expect(reduceQuestionDraft(draft, { type: "back" }).index).toBe(0);
  draft = reduceQuestionDraft(draft, { type: "select", label: "Blue" });
  draft = reduceQuestionDraft(draft, { type: "next" });
  expect(draft.index).toBe(1);
  draft = reduceQuestionDraft(draft, { type: "select", label: "Search" });
  draft = reduceQuestionDraft(draft, { type: "custom", text: "  Offline mode\nwith sync  " });
  expect(reduceQuestionDraft(draft, { type: "next" }).index).toBe(1);
  draft = reduceQuestionDraft(draft, { type: "back" });
  expect(draft.index).toBe(0);
  draft = reduceQuestionDraft(draft, { type: "select", label: "Green" });
  draft = reduceQuestionDraft(draft, { type: "next" });
  expect(questionAnswers(draft)).toEqual([["Green"], ["Search", "Offline mode\nwith sync"]]);
  expect(draft.prompts[1].custom).toBe("  Offline mode\nwith sync  ");
});

test("single-choice custom text and suggested options replace each other", () => {
  let draft = createQuestionDraft(prompts);
  draft = reduceQuestionDraft(draft, { type: "select", label: "Blue" });
  draft = reduceQuestionDraft(draft, { type: "custom", text: "  Purple  " });
  expect(draft.prompts[0].selected).toEqual([]);
  expect(questionAnswers(draft)[0]).toEqual(["Purple"]);
  draft = reduceQuestionDraft(draft, { type: "select", label: "Green" });
  expect(draft.prompts[0].custom).toBe("");
  expect(questionAnswers(draft)[0]).toEqual(["Green"]);
  draft = reduceQuestionDraft(draft, { type: "custom", text: "   " });
  expect(questionAnswers(draft)[0]).toEqual(["Green"]);
  draft = reduceQuestionDraft(draft, { type: "custom", text: "Purple" });
  draft = reduceQuestionDraft(draft, { type: "custom", text: "   " });
  expect(questionAnswers(draft)[0]).toEqual([]);
  expect(reduceQuestionDraft(draft, { type: "next" }).index).toBe(0);
});

test("multiple-choice answers toggle independently and deduplicate custom text", () => {
  let draft = createQuestionDraft(prompts);
  draft = reduceQuestionDraft(draft, { type: "select", label: "Blue" });
  draft = reduceQuestionDraft(draft, { type: "next" });
  draft = reduceQuestionDraft(draft, { type: "select", label: "Search" });
  draft = reduceQuestionDraft(draft, { type: "select", label: "Export" });
  draft = reduceQuestionDraft(draft, { type: "custom", text: " Search " });
  expect(questionAnswers(draft)[1]).toEqual(["Search", "Export"]);
  draft = reduceQuestionDraft(draft, { type: "select", label: "Search" });
  expect(questionAnswers(draft)[1]).toEqual(["Export", "Search"]);
  draft = reduceQuestionDraft(draft, { type: "custom", text: "" });
  expect(questionAnswers(draft)[1]).toEqual(["Export"]);
});

test("a text-only prompt can continue and a new request starts without old answers", () => {
  let draft = createQuestionDraft([{ question: "What else?", header: "Notes", options: null }]);
  draft = reduceQuestionDraft(draft, { type: "custom", text: " Include tests " });
  expect(questionAnswers(draft)).toEqual([["Include tests"]]);
  expect(reduceQuestionDraft(draft, { type: "next" }).index).toBe(0);
  expect(questionAnswers(createQuestionDraft(prompts))).toEqual([[], []]);
});

test("reconnecting with an older snapshot does not reopen a resolved question", () => {
  const pending: Question = {
    id: "q1",
    session_id: "s1",
    client_id: "c1",
    status: "pending",
    created_at: "2026-10-18T00:00:00Z",
    updated_at: "2026-10-18T00:00:00Z",
    questions: [],
  };
  const answered = { ...pending, status: "answered", answers: [["Blue"]] };
  let records = mergeQuestions(new Map(), [answered]);
  records = mergeQuestions(records, [pending]);
  expect(records.get("q1")).toEqual(answered);
});
