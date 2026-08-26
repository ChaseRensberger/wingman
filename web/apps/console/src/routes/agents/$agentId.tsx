import { useEffect, useRef, useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useForm } from "@tanstack/react-form";
import { useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { Badge } from "@wingman/core/components/core/badge";
import { Button } from "@wingman/core/components/core/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@wingman/core/components/core/select";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@wingman/core/components/core/alert-dialog";
import { Input } from "@wingman/core/components/core/input";
import { Textarea } from "@wingman/core/components/core/textarea";
import { Field, FieldLabel, FieldError } from "@wingman/core/components/core/field";
import { Spinner } from "@wingman/core/components/core/spinner";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { client } from "@/lib/client";
import { isProviderSelectable } from "@/lib/providers";
import { showErrorToast } from "@/lib/toast";
import type { Agent } from "@/lib/types";
import { splitModelRef } from "@/lib/utils";
import { agentFormSchema, buildAgentPayload } from "@/lib/agent-form";
import {
  agentQuery,
  providerModelsQuery,
  providersQuery,
  queryKeys,
  toolsQuery,
} from "@/lib/queries";

function formFromAgent(agent: Agent) {
  const modelRef = splitModelRef(agent.model_ref);
  return {
    name: agent.name,
    instructions: agent.instructions ?? "",
    provider: modelRef.provider,
    model: modelRef.model,
    variant: modelRef.variant,
    tools: agent.tools ?? [],
    outputSchema:
      agent.output_schema && Object.keys(agent.output_schema).length > 0
        ? JSON.stringify(agent.output_schema, null, 2)
        : "",
  };
}

export const Route = createFileRoute("/agents/$agentId")({
  component: AgentDetailPage,
});

function AgentDetailPage() {
  const { agentId } = Route.useParams();
  const navigate = useNavigate();
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const loadedAgentID = useRef("");
  const queryClient = useQueryClient();
  const agentResult = useQuery(agentQuery(agentId));
  const providersResult = useQuery(providersQuery);
  const toolsResult = useQuery(toolsQuery);
  const agent = agentResult.data ?? null;
  const providers = providersResult.data ?? [];
  const toolCatalog = toolsResult.data?.tools.tools ?? [];
  const selectableProviders = providers.filter(isProviderSelectable);
  const modelResults = useQueries({
    queries: selectableProviders.map((provider) => providerModelsQuery(provider.id)),
  });
  const models = Object.fromEntries(
    selectableProviders.map((provider, index) => [provider.id, modelResults[index].data ?? []]),
  );
  const initializing = loadedAgentID.current !== agentId;
  const loading =
    agentResult.isPending ||
    providersResult.isPending ||
    toolsResult.isPending ||
    modelResults.some((result) => result.isPending) ||
    (initializing && (agentResult.isFetching || modelResults.some((result) => result.isFetching)));

  useEffect(() => {
    const error = agentResult.error ?? providersResult.error ?? toolsResult.error;
    if (error) showErrorToast(error);
  }, [agentResult.error, providersResult.error, toolsResult.error]);

  const form = useForm({
    defaultValues: formFromAgent({
      id: "",
      name: "",
      instructions: "",
      model_ref: "",
      tools: [],
      output_schema: undefined,
      created_at: "",
      updated_at: "",
    } as Agent),
    validators: {
      onBlur: agentFormSchema,
      onSubmit: agentFormSchema,
    },
    onSubmit: async ({ value }) => {
      setSaving(true);
      try {
        const updated = (await client.agents.update(agentId, buildAgentPayload(value))) as Agent;
        queryClient.setQueryData(queryKeys.agent(agentId), updated);
        await queryClient.invalidateQueries({ queryKey: queryKeys.agents });
        form.reset(formFromAgent(updated));
      } catch (err) {
        showErrorToast(err);
      } finally {
        setSaving(false);
      }
    },
  });

  useEffect(() => {
    if (!agent || loading || loadedAgentID.current === agent.id) return;
    const values = formFromAgent(agent);
    const variants =
      models[values.provider]?.find((model) => model.id === values.model)?.variants ?? [];
    if (values.variant && !variants.includes(values.variant)) values.variant = "";
    form.reset(values);
    loadedAgentID.current = agent.id;
  }, [agent, form, loading, models]);

  async function remove() {
    if (!agent) return;
    setDeleting(true);
    try {
      await client.agents.delete(agent.id);
      queryClient.setQueryData<Agent[]>(queryKeys.agents, (previous) =>
        previous?.filter((item) => item.id !== agent.id),
      );
      navigate({ to: "/agents" });
    } catch (err) {
      showErrorToast(err);
      setDeleting(false);
    }
  }

  const availableTools = toolCatalog.map((tool) => tool.name);
  const crumbLabel = agent?.name || agentId;

  return (
    <div className="mx-auto max-w-5xl px-4 py-6">
      <div className="mb-4 flex items-center justify-between gap-4">
        <PageBreadcrumb items={[{ label: "Agents", to: "/agents" }, { label: crumbLabel }]} />
        {agent && (
          <AlertDialog>
            <AlertDialogTrigger
              render={<Button size="sm" variant="destructive" disabled={deleting} />}
            >
              {deleting ? "Deleting..." : "Delete"}
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete agent?</AlertDialogTitle>
                <AlertDialogDescription>
                  This will permanently delete {agent.name}. This action cannot be undone.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
                <AlertDialogAction variant="destructive" onClick={remove} disabled={deleting}>
                  {deleting ? "Deleting..." : "Delete"}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        )}
      </div>

      {loading ? (
        <div className="flex items-center gap-3 py-8 text-sm text-muted-foreground">
          <Spinner size={24} />
          <span>Loading...</span>
        </div>
      ) : !agent ? (
        <div className="py-8 text-sm text-muted-foreground">Agent not found.</div>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            e.stopPropagation();
            form.handleSubmit();
          }}
          noValidate
          className="grid gap-4 rounded-lg border bg-card p-4"
        >
          <form.Field
            name="name"
            children={(field) => (
              <Field
                className="gap-1"
                data-invalid={field.state.meta.errors.length > 0 || undefined}
              >
                <FieldLabel className="text-xs">
                  Name <span aria-hidden="true">*</span>
                  <span className="sr-only"> required</span>
                </FieldLabel>
                <Input
                  name={field.name}
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(e) => field.handleChange(e.target.value)}
                  aria-invalid={field.state.meta.errors.length > 0}
                />
                <FieldError errors={field.state.meta.errors as Array<{ message?: string }>} />
              </Field>
            )}
          />
          <form.Field
            name="instructions"
            children={(field) => (
              <Field
                className="gap-1"
                data-invalid={field.state.meta.errors.length > 0 || undefined}
              >
                <FieldLabel className="text-xs">Instructions</FieldLabel>
                <Textarea
                  className="min-h-40"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(e) => field.handleChange(e.target.value)}
                  aria-invalid={field.state.meta.errors.length > 0}
                />
                <FieldError errors={field.state.meta.errors as Array<{ message?: string }>} />
              </Field>
            )}
          />
          <form.Subscribe
            selector={(state) => [state.values.provider, state.values.model] as const}
            children={([provider, selectedModel]) => {
              const providerModels = models[provider] ?? [];
              const variants =
                providerModels.find((model) => model.id === selectedModel)?.variants ?? [];
              return (
                <div className="grid gap-3 sm:grid-cols-2">
                  <form.Field
                    name="provider"
                    children={(field) => (
                      <Field
                        className="gap-1"
                        data-invalid={field.state.meta.errors.length > 0 || undefined}
                      >
                        <FieldLabel className="text-xs">Provider</FieldLabel>
                        <Select
                          value={field.state.value}
                          onValueChange={(value) => {
                            field.handleChange(value ?? "");
                            form.setFieldValue("model", "");
                            form.setFieldValue("variant", "");
                          }}
                        >
                          <SelectTrigger>
                            <SelectValue placeholder="Select provider" />
                          </SelectTrigger>
                          <SelectContent>
                            {selectableProviders.map((p) => (
                              <SelectItem key={p.id} value={p.id}>
                                {p.name}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        <FieldError
                          errors={field.state.meta.errors as Array<{ message?: string }>}
                        />
                      </Field>
                    )}
                  />
                  <form.Field
                    name="model"
                    children={(field) => (
                      <Field
                        className="gap-1"
                        data-invalid={field.state.meta.errors.length > 0 || undefined}
                      >
                        <FieldLabel className="text-xs">Model</FieldLabel>
                        <Select
                          value={field.state.value}
                          onValueChange={(value) => {
                            field.handleChange(value ?? "");
                            form.setFieldValue("variant", "");
                          }}
                          disabled={!provider || providerModels.length === 0}
                        >
                          <SelectTrigger>
                            <SelectValue
                              placeholder={provider ? "Select model" : "Select provider first"}
                            />
                          </SelectTrigger>
                          <SelectContent>
                            {providerModels.map((model) => (
                              <SelectItem key={model.id} value={model.id}>
                                {model.id}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        <FieldError
                          errors={field.state.meta.errors as Array<{ message?: string }>}
                        />
                      </Field>
                    )}
                  />
                  {variants.length > 0 && (
                    <form.Field
                      name="variant"
                      children={(field) => (
                        <Field
                          className="gap-1"
                          data-invalid={field.state.meta.errors.length > 0 || undefined}
                        >
                          <FieldLabel className="text-xs">Variant</FieldLabel>
                          <Select
                            value={field.state.value || null}
                            onValueChange={(value) => field.handleChange(value ?? "")}
                          >
                            <SelectTrigger>
                              <SelectValue placeholder="Select variant">
                                {(value) => value ?? "Provider default"}
                              </SelectValue>
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem>Provider default</SelectItem>
                              {variants.map((variant) => (
                                <SelectItem key={variant} value={variant}>
                                  {variant}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                          <FieldError
                            errors={field.state.meta.errors as Array<{ message?: string }>}
                          />
                        </Field>
                      )}
                    />
                  )}
                </div>
              );
            }}
          />
          <form.Field
            name="tools"
            children={(field) => (
              <div className="grid gap-2">
                <div className="flex items-center justify-between gap-3">
                  <label className="text-xs font-medium">Tools</label>
                  <div className="flex gap-1">
                    <Button
                      type="button"
                      variant="ghost"
                      size="xs"
                      onClick={() => field.handleChange(availableTools)}
                    >
                      All on
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="xs"
                      onClick={() => field.handleChange([])}
                    >
                      All off
                    </Button>
                  </div>
                </div>
                <div className="flex flex-wrap gap-2">
                  {toolCatalog.map((tool) => (
                    <Button
                      key={tool.name}
                      type="button"
                      onClick={() => {
                        const current = field.state.value;
                        const next = current.includes(tool.name)
                          ? current.filter((item) => item !== tool.name)
                          : [...current, tool.name];
                        field.handleChange(next);
                      }}
                      variant="ghost"
                      className="h-auto rounded-md p-0"
                      title={`${tool.source}${tool.server ? `: ${tool.server}` : ""}`}
                    >
                      <Badge
                        variant={field.state.value.includes(tool.name) ? "default" : "outline"}
                      >
                        {tool.name}
                      </Badge>
                    </Button>
                  ))}
                </div>
              </div>
            )}
          />
          <form.Field
            name="outputSchema"
            children={(field) => (
              <Field
                className="gap-1"
                data-invalid={field.state.meta.errors.length > 0 || undefined}
              >
                <FieldLabel className="text-xs">Output schema JSON</FieldLabel>
                <Textarea
                  className="min-h-32 font-mono text-xs"
                  placeholder="Optional JSON Schema"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(e) => field.handleChange(e.target.value)}
                  aria-invalid={field.state.meta.errors.length > 0}
                />
                <FieldError errors={field.state.meta.errors as Array<{ message?: string }>} />
              </Field>
            )}
          />
          <div className="flex justify-end">
            <Button type="submit" disabled={saving}>
              {saving ? "Saving..." : "Save changes"}
            </Button>
          </div>
        </form>
      )}
    </div>
  );
}
