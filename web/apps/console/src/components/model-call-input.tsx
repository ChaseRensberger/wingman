import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";

import { client } from "@/lib/client";
import { modelCallChanges } from "@/lib/model-input";
import type { ModelCall, ModelCallInput } from "@/lib/types";
import { Button } from "@wingman/core/components/core/button";

function InputSection({
  title,
  children,
  open = false,
}: {
  title: string;
  children: ReactNode;
  open?: boolean;
}) {
  return (
    <details className="rounded-lg border" open={open}>
      <summary className="cursor-pointer px-3 py-2 text-sm font-medium">{title}</summary>
      <div className="space-y-3 border-t p-3">{children}</div>
    </details>
  );
}

function InputText({ text }: { text: string }) {
  return (
    <pre
      tabIndex={0}
      data-scrollable
      className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted/30 p-3 font-mono text-xs leading-5"
    >
      {text}
    </pre>
  );
}

export function ModelInputContent({ input }: { input: ModelCallInput }) {
  return (
    <div className="space-y-3">
      {input.instructions && input.instructions.length > 0 && (
        <InputSection title={`Instruction sources · ${input.instructions.length}`} open>
          <p className="text-xs text-muted-foreground">
            Guidance discovered before system-prompt transforms for this call. The recorded system
            prompt below shows the final text.
          </p>
          {input.instructions.map((source) => (
            <InputSection key={source.path} title={`${source.kind} · ${source.path}`}>
              {source.resolved_path && (
                <p className="break-all text-xs text-muted-foreground">
                  Discovered through read: {source.requested_path} → {source.resolved_path}
                  {source.tool_use_id ? ` · tool use ${source.tool_use_id}` : ""}
                </p>
              )}
              <p className="break-all text-xs text-muted-foreground">SHA-256: {source.sha256}</p>
              <InputText text={source.content ?? ""} />
            </InputSection>
          ))}
        </InputSection>
      )}
      <InputSection title="System prompt" open>
        {input.system ? (
          <InputText text={input.system} />
        ) : (
          <p className="text-sm text-muted-foreground">No system prompt.</p>
        )}
      </InputSection>
      <InputSection title={`Messages · ${input.messages.length}`} open>
        {input.messages.length === 0 && (
          <p className="text-sm text-muted-foreground">No messages.</p>
        )}
        {input.messages.map((message, index) => (
          <InputSection
            key={index}
            title={`${index + 1}. ${message.role} · ${message.content.length} parts`}
          >
            <p className="break-all text-xs text-muted-foreground">
              {message.id
                ? `Source message ${message.id}${message.revision ? ` · revision ${message.revision}` : ""}`
                : "No source message ID recorded."}
            </p>
            {message.content.map((part, partIndex) => (
              <div key={partIndex}>
                <div className="mb-1 text-xs font-medium">{part.type}</div>
                <InputText
                  text={
                    part.type === "text" && "text" in part && typeof part.text === "string"
                      ? part.text
                      : part.type === "reasoning" &&
                          "reasoning" in part &&
                          typeof part.reasoning === "string"
                        ? part.reasoning
                        : JSON.stringify(part, null, 2)
                  }
                />
              </div>
            ))}
          </InputSection>
        ))}
      </InputSection>
      <InputSection title={`Available tools · ${input.tools.length}`}>
        <p className="text-xs text-muted-foreground">
          Definitions offered to the model for this call. These are not tool executions.
        </p>
        {input.tools.length === 0 && <p className="text-sm text-muted-foreground">No tools.</p>}
        {input.tools.map((tool, index) => (
          <InputSection key={index} title={tool.name}>
            <InputText text={tool.description} />
            <InputText text={JSON.stringify(tool.input_schema, null, 2)} />
          </InputSection>
        ))}
        {input.tool_choice && <p className="text-xs">Tool choice: {input.tool_choice}</p>}
      </InputSection>
      {input.output_schema && (
        <InputSection title="Output schema">
          <InputText text={JSON.stringify(input.output_schema, null, 2)} />
        </InputSection>
      )}
    </div>
  );
}

export function ModelCallInputView({
  call,
  previous,
  onInspect,
}: {
  call: ModelCall;
  previous?: ModelCall;
  onInspect: (id: string) => void;
}) {
  const recorded = Boolean(call.trace?.input_hash);
  const query = useQuery({
    queryKey: ["model-call-input", call.session_id, call.id],
    queryFn: () =>
      client.sessions.modelCalls.input(call.session_id, call.id) as Promise<{
        input: ModelCallInput | null;
      }>,
    enabled: recorded,
    staleTime: Infinity,
  });
  const changes = modelCallChanges(call, previous);
  return (
    <section className="space-y-4">
      <div>
        <h2 className="text-sm font-medium">Effective model input</h2>
        <p className="mt-1 text-xs text-muted-foreground">
          {call.model_ref || call.model_id || "Unknown model"} · step {call.step} · attempt{" "}
          {call.attempt} · {call.status}
        </p>
        <p className="mt-2 text-sm text-muted-foreground">
          Recorded after context transforms, before provider translation. This is the content
          prepared for this attempt, not the provider's HTTP payload or proof of receipt.
        </p>
        <p className="mt-1 text-xs text-muted-foreground">
          Conversation history and execution events can differ from this input.
        </p>
      </div>
      {previous && (
        <div className="rounded-lg border p-3 text-xs text-muted-foreground">
          <p>
            {changes.length
              ? changes.join(" · ")
              : "No model, system-prompt, or tool-schema changes detected in the available request manifests."}
          </p>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="mt-1"
            onClick={() => onInspect(previous.id)}
          >
            Inspect preceding call
          </Button>
        </div>
      )}
      {!recorded || (query.isSuccess && !query.data.input) ? (
        <p className="text-sm text-muted-foreground">
          Input content was not recorded for this call. It cannot be reconstructed reliably from the
          current conversation.
        </p>
      ) : query.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">
          Loading recorded input…
        </p>
      ) : query.isError ? (
        <div role="alert" className="text-sm text-destructive">
          <p>Could not load recorded input: {query.error.message}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void query.refetch()}>
            Retry
          </Button>
        </div>
      ) : (
        query.data.input && <ModelInputContent input={query.data.input} />
      )}
    </section>
  );
}
