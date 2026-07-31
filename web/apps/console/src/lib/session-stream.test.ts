import { expect, test } from "bun:test";

import { readSSE } from "./session-stream";

test("readSSE preserves SSE event ids", async () => {
	const response = new Response('id:42\nevent: session.text.delta\ndata: {"delta":"hello"}\n\n');
	const events = await Array.fromAsync(readSSE(response));

	expect(events).toEqual([{ id: "42", event: "session.text.delta", data: { delta: "hello" } }]);
});
