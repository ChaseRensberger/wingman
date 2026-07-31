import createClient, { type ClientOptions } from "openapi-fetch";

import type { components, paths } from "./schema";

export type { components, operations, paths } from "./schema";

export type ErrorResponse = components["schemas"]["ErrorResponse"];
export type SessionEvent = components["schemas"]["SessionEvent"];
export type RunStreamEvent = components["schemas"]["RunStreamEvent"];

export function createWingmanClient(options: ClientOptions = {}) {
  return createClient<paths>(options);
}
