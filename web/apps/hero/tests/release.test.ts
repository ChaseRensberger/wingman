import { afterAll, afterEach, describe, expect, spyOn, test } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { LatestRelease } from "../src/components/latest-release";
import { fetchLatestRelease, LATEST_RELEASE_URL } from "../src/lib/release";

const fetchMock = spyOn(globalThis, "fetch");

afterEach(() => fetchMock.mockReset());
afterAll(() => fetchMock.mockRestore());

describe("latest release", () => {
  test("renders a working generic link before release data is available", () => {
    const html = renderToStaticMarkup(createElement(LatestRelease));
    expect(html).toContain(`href="${LATEST_RELEASE_URL}"`);
    expect(html).toContain("Latest release ↗");
    expect(html).not.toContain("Latest release:");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  test("reads the latest published tag and links to that release", async () => {
    fetchMock.mockResolvedValue(Response.json({ tag_name: "v0.1.58" }));
    const signal = new AbortController().signal;

    expect(await fetchLatestRelease(signal)).toEqual({
      version: "v0.1.58",
      url: "https://github.com/chaserensberger/wingman/releases/tag/v0.1.58",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.github.com/repos/chaserensberger/wingman/releases/latest",
      { signal },
    );
  });

  test("uses new release data rather than a hardcoded version", async () => {
    fetchMock.mockResolvedValue(Response.json({ tag_name: "v0.2.0" }));
    expect((await fetchLatestRelease(new AbortController().signal))?.version).toBe("v0.2.0");
  });

  test("falls back when GitHub rejects the request", async () => {
    for (const status of [403, 404, 429, 500]) {
      fetchMock.mockResolvedValue(new Response(null, { status }));
      expect(await fetchLatestRelease(new AbortController().signal)).toBeNull();
    }
  });

  test("falls back on a network failure", async () => {
    fetchMock.mockRejectedValue(new TypeError("Failed to fetch"));
    expect(await fetchLatestRelease(new AbortController().signal)).toBeNull();
  });

  test("falls back on missing or invalid release data", async () => {
    for (const body of [null, {}, { tag_name: 123 }, { tag_name: " " }]) {
      fetchMock.mockResolvedValue(Response.json(body));
      expect(await fetchLatestRelease(new AbortController().signal)).toBeNull();
    }
    fetchMock.mockResolvedValue(new Response("not JSON"));
    expect(await fetchLatestRelease(new AbortController().signal)).toBeNull();
  });

  test("settles an aborted lookup without an error", async () => {
    fetchMock.mockRejectedValue(new DOMException("Aborted", "AbortError"));
    expect(await fetchLatestRelease(new AbortController().signal)).toBeNull();
  });
});
