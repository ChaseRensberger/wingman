import { expect, test } from "bun:test";
import type { Question } from "@wingman-actor/client";
import { mergeQuestions } from "./questions";

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
