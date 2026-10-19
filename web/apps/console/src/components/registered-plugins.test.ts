import { expect, test } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { RegisteredPlugins } from "./registered-plugins";

test("registered plugins render without a session and include disabled registrations", () => {
  const queryClient = new QueryClient();
  queryClient.setQueryData(["plugins"], {
    plugins: [
      { id: "compaction", enabled: true },
      { id: "wingcode", enabled: true },
      { id: "optional", enabled: false },
    ],
  });
  const markup = renderToStaticMarkup(
    createElement(QueryClientProvider, { client: queryClient }, createElement(RegisteredPlugins)),
  );
  expect(markup).toContain("compaction");
  expect(markup).toContain("wingcode");
  expect(markup).toContain("optional");
  expect(markup.match(/>Enabled</g)).toHaveLength(2);
  expect(markup).toContain(">Disabled<");
  expect(markup).not.toContain("Create the session");
  expect(markup).not.toContain("Loading plugins");
  queryClient.clear();
});
