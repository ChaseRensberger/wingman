import { apiErrorFromResponse } from "./client";

export function enrollmentCredentialFromFragment(fragment: string): { credential?: string; fragment: string } {
	const parameters = new URLSearchParams(fragment.startsWith("#") ? fragment.slice(1) : fragment);
	const credential = parameters.get("enrollment") || undefined;
  if (!credential) return { fragment };

	parameters.delete("enrollment");
  const remaining = parameters.toString();
  return { credential, fragment: remaining ? `#${remaining}` : "" };
}

export function enrollmentCredentialFromInput(value: string): string {
  const input = value.trim();
  try {
    const url = new URL(input);
		return enrollmentCredentialFromFragment(url.hash).credential ?? "";
  } catch {
    return input;
  }
}

export async function redeemEnrollmentCredential(credential: string): Promise<void> {
	const response = await fetch("/auth/enrollments/redeem", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ credential, mode: "cookie" }),
  });
  if (!response.ok) throw await apiErrorFromResponse(response);
}
