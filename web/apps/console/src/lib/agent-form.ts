import { z } from "zod";

export interface AgentForm {
  name: string;
  instructions: string;
  provider: string;
  model: string;
  tools: string[];
  outputSchema: string;
}

export const emptyForm: AgentForm = {
  name: "",
  instructions: "",
  provider: "",
  model: "",
  tools: [],
  outputSchema: "",
};

export const agentFormSchema = z.object({
  name: z.string().refine((v) => v.trim().length > 0, {
    message: "Name is required",
  }),
  instructions: z.string(),
  provider: z.string(),
  model: z.string(),
  tools: z.array(z.string()),
  outputSchema: z.string().refine(
    (val) => {
      if (!val.trim()) return true;
      try {
        const parsed = JSON.parse(val);
        return typeof parsed === "object" && parsed !== null && !Array.isArray(parsed);
      } catch {
        return false;
      }
    },
    { message: "Must be empty or a valid JSON object" }
  ),
});

export function buildAgentPayload(form: AgentForm) {
  let output_schema: Record<string, unknown> | undefined;
  if (form.outputSchema.trim()) {
    output_schema = JSON.parse(form.outputSchema);
  }
  return {
    name: form.name.trim(),
    instructions: form.instructions,
    model_ref: form.provider && form.model ? `${form.provider}/${form.model}` : "",
    tools: form.tools,
    output_schema,
  };
}
