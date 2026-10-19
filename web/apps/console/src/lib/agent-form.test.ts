import { expect, test } from "bun:test";

import { buildAgentPayload, type AgentForm } from "./agent-form";

test("buildAgentPayload includes the selected model variant", () => {
  const form: AgentForm = {
    name: "Reviewer",
    instructions: "Review the change.",
    provider: "openai",
    model: "gpt-5.6-terra",
    variant: "high",
    tools: [],
    plugins: ["wingcode"],
    outputSchema: "",
  };
  expect(buildAgentPayload(form).model_ref).toBe("openai/gpt-5.6-terra#high");
  expect(buildAgentPayload(form).plugins).toEqual(["wingcode"]);
  expect(buildAgentPayload({ ...form, plugins: [] }).plugins).toEqual([]);
});
