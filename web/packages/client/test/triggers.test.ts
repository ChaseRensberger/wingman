import { expect, test } from "bun:test";
import { createWingmanClient } from "../src/index";

test("trigger requests keep client identity, versions, and manual retry identity", async () => {
  const requests: Request[] = [];
  const client = createWingmanClient({
    baseUrl: "https://wingman.test",
    password: "secret",
    clientName: "cli_console",
    fetch: async (input, init) => {
      requests.push(new Request(input, init));
      return Response.json({});
    },
  });
  await client.triggers.fire("trg_test", "same-click");
  await client.triggers.fire("trg_test", "same-click");
  for (const request of requests) {
    expect(request.url).toBe("https://wingman.test/triggers/trg_test/fire");
    expect(request.method).toBe("POST");
    expect(request.headers.get("X-Wingman-Client")).toBe("cli_console");
    expect(request.headers.get("Authorization")).toBe("Basic d2luZ21hbjpzZWNyZXQ=");
    expect(await request.json()).toEqual({ request_id: "same-click" });
  }
  await client.triggers.setState("trg_test", { enabled: false, expected_version: 3 });
  expect(requests.at(-1)?.method).toBe("PUT");
  expect(await requests.at(-1)?.json()).toEqual({ enabled: false, expected_version: 3 });
  await client.triggers.delete("trg_test", 4);
  expect(requests.at(-1)?.url).toBe("https://wingman.test/triggers/trg_test?expected_version=4");
  await client.triggers.occurrences();
  expect(requests.at(-1)?.url).toBe("https://wingman.test/triggers/occurrences");
  await client.triggers.occurrences("trg_test");
  expect(requests.at(-1)?.url).toBe("https://wingman.test/triggers/trg_test/occurrences");
});
