import { describe, expect, test } from "bun:test";

const allowedPackages = new Set(["openapi-fetch"]);

describe("browser client boundaries", () => {
  test("only imports local modules and declared browser dependencies", async () => {
    const violations: string[] = [];
    const sourceRoot = import.meta.dir + "/../src";
    const files = new Bun.Glob("**/*.{ts,tsx}");
    for await (const file of files.scan(sourceRoot)) {
      const source = await Bun.file(sourceRoot + "/" + file).text();
      for (const match of source.matchAll(/(?:from\s+|import\s*)["']([^"']+)["']/g)) {
        const specifier = match[1];
        if (!specifier.startsWith(".") && !allowedPackages.has(specifier)) {
          violations.push(`${file}: ${specifier}`);
        }
      }
    }
    expect(violations).toEqual([]);
  });
});
