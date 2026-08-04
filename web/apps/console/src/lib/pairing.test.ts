import { afterEach, describe, expect, test } from "bun:test";

import { pairingCredentialFromFragment, pairingCredentialFromInput, redeemPairingCredential } from "./pairing";

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
});

describe("pairing credentials", () => {
  test("extracts a credential and removes only its fragment parameter", () => {
    expect(pairingCredentialFromFragment("#view=sessions&pairing=pair%2Fcredential&theme=dark")).toEqual({
      credential: "pair/credential",
      fragment: "#view=sessions&theme=dark",
    });
  });

  test("accepts either a raw credential or a full pairing URL", () => {
    expect(pairingCredentialFromInput(" raw-credential ")).toBe("raw-credential");
    expect(pairingCredentialFromInput("https://wingman.example/console#pairing=pair%2Fcredential")).toBe("pair/credential");
  });

  test("redeems credentials with same-origin cookies", async () => {
    let request: RequestInit | undefined;
    globalThis.fetch = async (_input, init) => {
      request = init;
      return new Response(null, { status: 204 });
    };

    await redeemPairingCredential("credential");

    expect(request).toMatchObject({
      method: "POST",
      credentials: "same-origin",
      body: JSON.stringify({ credential: "credential", mode: "cookie" }),
    });
  });

  test("surfaces failed redemptions", async () => {
    globalThis.fetch = async () => new Response(JSON.stringify({ error: { message: "Pairing link expired" } }), { status: 401 });

    await expect(redeemPairingCredential("expired")).rejects.toThrow("Pairing link expired");
  });
});
