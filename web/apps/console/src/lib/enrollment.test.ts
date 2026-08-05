import { afterEach, describe, expect, test } from "bun:test";

import { enrollmentCredentialFromFragment, enrollmentCredentialFromInput, redeemEnrollmentCredential } from "./enrollment";

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
});

describe("enrollment credentials", () => {
  test("extracts a credential and removes only its fragment parameter", () => {
    expect(enrollmentCredentialFromFragment("#view=sessions&enrollment=credential%2Fvalue&theme=dark")).toEqual({
      credential: "credential/value",
      fragment: "#view=sessions&theme=dark",
    });
  });

  test("accepts either a raw credential or an enrollment URL", () => {
    expect(enrollmentCredentialFromInput(" raw-credential ")).toBe("raw-credential");
    expect(enrollmentCredentialFromInput("https://wingman.example/anything#enrollment=credential%2Fvalue")).toBe("credential/value");
  });

  test("redeems credentials with same-origin cookies", async () => {
    let request: RequestInit | undefined;
    globalThis.fetch = async (_input, init) => {
      request = init;
      return new Response(null, { status: 204 });
    };

    await redeemEnrollmentCredential("credential");

    expect(request).toMatchObject({
      method: "POST",
      credentials: "same-origin",
      body: JSON.stringify({ credential: "credential", mode: "cookie" }),
    });
  });

  test("surfaces failed redemptions", async () => {
    globalThis.fetch = async () => new Response(JSON.stringify({ error: { message: "Enrollment credential expired" } }), { status: 401 });

    await expect(redeemEnrollmentCredential("expired")).rejects.toThrow("Enrollment credential expired");
  });
});
