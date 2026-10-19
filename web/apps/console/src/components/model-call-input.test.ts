import { expect, test } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { ModelInputContent } from "./model-call-input";

test("recorded input preserves source revisions and displays prompt text without executing markup", () => {
  const html = renderToStaticMarkup(
    createElement(ModelInputContent, {
      input: {
        system: "Historical instructions <script>alert('x')</script>",
        messages: [
          {
            id: "msg_original",
            revision: 3,
            role: "user",
            content: [{ type: "text", text: "Exact transformed input" }],
          },
        ],
        tools: [{ name: "read", description: "Read a file", input_schema: { type: "object" } }],
        instructions: [
          {
            kind: "directory",
            path: "/reference/AGENTS.md",
            sha256: "hash",
            content: "Recorded guidance",
            requested_path: "linked.go",
            resolved_path: "/reference/source.go",
            tool_use_id: "tlu_read",
          },
        ],
      },
    }),
  );
  expect(html).toContain("Historical instructions &lt;script&gt;");
  expect(html).not.toContain("<script>");
  expect(html).toContain("Source message msg_original · revision 3");
  expect(html).toContain("Exact transformed input");
  expect(html).toContain("These are not tool executions.");
  expect(html).toContain("Recorded guidance");
  expect(html).toContain("linked.go → /reference/source.go · tool use tlu_read");
});
