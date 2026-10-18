import { expect, test } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import type { Question } from "@wingman-actor/client";

import { QuestionPanel } from "./question-panel";

const question: Question = {
  id: "q1",
  session_id: "s1",
  client_id: "c1",
  status: "pending",
  created_at: "2026-10-18T00:00:00Z",
  updated_at: "2026-10-18T00:00:00Z",
  questions: [
    {
      header: "Color",
      question: "Which color?",
      options: [{ label: "Blue", description: "Blue color" }],
    },
    { header: "Features", question: "Which features?", options: [], multiple: true },
  ],
};

test("the question dock shows one prompt with progress, choice cards, and navigation", () => {
  const markup = renderToStaticMarkup(
    createElement(QuestionPanel, { questions: [question], onReply: async () => {} }),
  );
  expect(markup).toContain("Question 1 of 2");
  expect(markup).toContain("Which color?");
  expect(markup).not.toContain(">Color<");
  expect(markup).not.toContain("Select one option, or write your own answer.");
  expect(markup).toContain('type="radio"');
  expect(markup).toContain("Blue color");
  expect(markup).toContain('placeholder="Type your own answer"');
  expect(markup).toContain('aria-label="Type your own answer"');
  expect(markup).not.toContain(">Type your own answer<");
  expect(markup).toContain("<textarea");
  expect(markup).toContain("Dismiss");
  expect(markup).toMatch(/<button[^>]*disabled[^>]*>Next<\/button>/);
  expect(markup).not.toContain("Which features?");
  expect(markup).not.toContain(">Back<");
  expect(markup).not.toContain("Stop run");
});

test("other pending requests wait their turn without hiding run cancellation", () => {
  const markup = renderToStaticMarkup(
    createElement(QuestionPanel, {
      questions: [
        question,
        {
          ...question,
          id: "q2",
          questions: [{ header: "Other", question: "Another request?", options: [] }],
        },
      ],
      onReply: async () => {},
      onAbort: () => {},
    }),
  );
  expect(markup).toContain("1 more request waiting");
  expect(markup).toContain("Stop run");
  expect(markup).not.toContain("Another request?");
});

test("a single text-only prompt offers submit instead of next", () => {
  const markup = renderToStaticMarkup(
    createElement(QuestionPanel, {
      questions: [
        { ...question, questions: [{ header: "Notes", question: "What else?", options: null }] },
      ],
      onReply: async () => {},
    }),
  );
  expect(markup).toContain("Question 1 of 1");
  expect(markup).toContain("Type your own answer");
  expect(markup).toMatch(/<button[^>]*disabled[^>]*>Submit answers<\/button>/);
  expect(markup).not.toContain(">Next<");
});

test("the question dock renders nothing when no requests are pending", () => {
  expect(
    renderToStaticMarkup(createElement(QuestionPanel, { questions: [], onReply: async () => {} })),
  ).toBe("");
});
