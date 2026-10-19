import { expect, test } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { ReasoningPart } from "./reasoning-part";

test.each([false, true])(
  "reasoning stays collapsed with a text-only hover: streaming=%s",
  (isStreaming) => {
    const markup = renderToStaticMarkup(
      createElement(ReasoningPart, {
        reasoning: "**Checking the source**\n\nDetails that stay hidden until expanded",
        isStreaming,
      }),
    );
    expect(markup).toContain('aria-expanded="false"');
    expect(markup).toContain("Checking the source");
    expect(markup).not.toContain("Details that stay hidden until expanded");
    expect(markup).not.toContain("hover:bg-");
    expect(markup).toContain("group-hover/thought:text-foreground");
    expect(markup).toContain("text-xs text-primary");
    expect(markup).not.toContain("text-amber-");
    expect(markup.match(/<svg\b/g)?.length ?? 0).toBe(isStreaming ? 1 : 0);
    if (isStreaming) expect(markup).toContain('data-hexwave=""');
    else expect(markup).toContain("Thought:");
  },
);
