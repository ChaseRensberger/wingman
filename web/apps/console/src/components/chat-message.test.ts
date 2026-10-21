import { expect, test } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { ChatMessage } from "./chat-message";
import type { Message, Usage } from "@/lib/types";

function compaction(
  usage?: Usage,
  summary = "The complete checkpoint summary",
  state?: Message["state"],
) {
  return renderToStaticMarkup(
    createElement(ChatMessage, {
      message: {
        role: "user",
        state,
        content: [
          {
            type: "compaction_marker",
            summary,
            reason: "auto",
            tokens_before: 90000,
            usage,
          },
        ],
      },
    }),
  );
}

test("compaction reports summarization usage without double-counting cache or reasoning", () => {
  const markup = compaction({
    input_tokens: 60300,
    output_tokens: 1900,
    total_tokens: 62200,
    cached_input_tokens: 50000,
    reasoning_tokens: 900,
  });
  expect(markup).toContain("Compaction · 60.3K in · 1.9K out");
  expect(markup).not.toContain("90.0K");
  expect(markup).not.toContain("Session compacted");
});

test.each([undefined, { input_tokens: 0, output_tokens: 0, total_tokens: 0 }])(
  "compaction does not invent unavailable usage: %j",
  (usage) => {
    const markup = compaction(usage);
    expect(markup).toContain(">Compaction</span>");
    expect(markup).not.toContain(" in · ");
  },
);

test("the compaction separator is the only toggle and the full summary stays visible when expanded", () => {
  const markup = compaction();
  expect(markup).toContain('data-component="session-compaction-message"');
  expect(markup).toContain("<details><summary");
  expect(markup).not.toContain("<details open");
  expect(markup).toMatch(/<summary[^>]*>.*>Compaction<\/span>.*<\/summary>/);
  expect(markup).toContain('class="pt-2 text-sm leading-6 text-foreground"');
  expect(markup).toContain("The complete checkpoint summary");
  expect(markup).toContain("group-hover/compaction:text-foreground");
  expect(markup).not.toContain("hover:bg-");
  expect(markup).not.toContain("<svg");
  expect(markup).not.toContain("max-w-3xl");
  expect(markup).not.toMatch(/class="[^"]*\sborder-l(?:\s|-)/);
  expect(markup).not.toContain("Automatic checkpoint");
});

test("a checkpoint without a summary has no empty disclosure control", () => {
  const markup = compaction(undefined, "");
  expect(markup).toContain(">Compaction</span>");
  expect(markup).not.toContain("<details");
  expect(markup).not.toContain("<summary");
});

test("running compaction shows the custom spinner to the left of its label", () => {
  const markup = compaction(undefined, "", "in_progress");
  expect(markup).toContain('data-hexwave=""');
  expect(markup).toContain('aria-label="Compaction running"');
  expect(markup.indexOf("<svg")).toBeLessThan(markup.indexOf(">Compaction</span>"));
  expect(markup).not.toContain("<details");
  expect(markup).not.toContain(" in · ");
});

test.each(["completed", "failed"] as const)("%s compaction never keeps spinning", (state) => {
  const markup = compaction(undefined, "", state);
  expect(markup).not.toContain('data-hexwave=""');
  expect(markup).toContain(state === "failed" ? "Compaction failed" : ">Compaction</span>");
});
