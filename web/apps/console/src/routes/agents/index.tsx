import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { useForm } from "@tanstack/react-form";
import { Button } from "@wingman/core/components/core/button";
import { Input } from "@wingman/core/components/core/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@wingman/core/components/core/select";
import { Textarea } from "@wingman/core/components/core/textarea";
import { Badge } from "@wingman/core/components/core/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@wingman/core/components/core/table";
import { Empty, EmptyDescription, EmptyTitle } from "@wingman/core/components/core/empty";
import { Field, FieldLabel, FieldError } from "@wingman/core/components/core/field";
import { wfetch } from "@/lib/client";
import { isProviderSelectable } from "@/lib/providers";
import { showErrorToast } from "@/lib/toast";
import { timeAgo } from "@/lib/utils";
import { emptyForm, agentFormSchema, buildAgentPayload } from "@/lib/agent-form";
import type { Agent, Provider, ProviderModel, ToolCatalogItem, ToolsResponse } from "@/lib/types";
import { MagnifyingGlassIcon, PlusIcon, XIcon } from "@phosphor-icons/react";
import { HexWaveSpinner } from "@/components/hex-wave-spinner";
import { PageBreadcrumb } from "@/components/page-breadcrumb";

export const Route = createFileRoute("/agents/")({
  component: AgentsPage,
});

function AgentsPage() {
  const navigate = useNavigate();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [models, setModels] = useState<Record<string, ProviderModel[]>>({});
  const [toolCatalog, setToolCatalog] = useState<ToolCatalogItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const [filterOpen, setFilterOpen] = useState(false);
  const filterInputRef = useRef<HTMLInputElement>(null);

  const form = useForm({
    defaultValues: emptyForm,
    validators: {
      onBlur: agentFormSchema,
      onSubmit: agentFormSchema,
    },
    onSubmit: async ({ value }) => {
      setSaving(true);
      try {
        const body = JSON.stringify(buildAgentPayload(value));
        await wfetch("/agents", { method: "POST", body });
        form.reset();
        setCreateOpen(false);
        await load();
      } catch (err) {
        showErrorToast(err);
      } finally {
        setSaving(false);
      }
    },
  });

  async function load() {
    try {
      const [agentData, providerData, toolData] = await Promise.all([
        wfetch("/agents") as Promise<Agent[]>,
        wfetch("/provider") as Promise<Provider[]>,
        wfetch("/tools") as Promise<ToolsResponse>,
      ]);
      setAgents(agentData);
      setProviders(providerData);
      setToolCatalog(toolData.tools ?? []);
      const selectableProviders = providerData.filter(isProviderSelectable);
      const modelEntries = await Promise.all(
        selectableProviders.map(async (provider) => {
          try {
            const data = (await wfetch(`/provider/${provider.id}/models`)) as Record<string, ProviderModel>;
            return [provider.id, Object.values(data).sort((a, b) => a.id.localeCompare(b.id))] as const;
          } catch {
            return [provider.id, []] as const;
          }
        }),
      );
      setModels(Object.fromEntries(modelEntries));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load().catch((err) => showErrorToast(err));
  }, []);

  useEffect(() => {
    if (filterOpen) filterInputRef.current?.focus();
  }, [filterOpen]);

  function openNew() {
    form.reset();
    form.setFieldValue("tools", toolCatalog.map((tool) => tool.name));
    setCreateOpen((open) => !open);
  }

  const availableTools = toolCatalog.map((tool) => tool.name);
  const selectableProviders = providers.filter(isProviderSelectable);
  const filteredAgents = agents.filter((agent) => {
    const haystack = `${agent.name} ${agent.model_ref || ""} ${(agent.tools ?? []).join(" ")}`.toLowerCase();
    return haystack.includes(filter.toLowerCase());
  });

  return (
    <div className="mx-auto max-w-5xl px-4 py-6">
      <div className="mb-4">
        <PageBreadcrumb items={[{ label: "Agents" }]} />
        <div className="mt-4 flex items-center justify-between gap-3">
          <Button size="sm" onClick={openNew}>
            <PlusIcon className="size-4" />
            New
          </Button>

          <div
            className={`flex h-9 items-center rounded-md border bg-card text-muted-foreground shadow-sm transition-all duration-200 focus-within:text-foreground hover:bg-accent hover:text-foreground ${
              filterOpen || filter ? "w-64 gap-2 px-2" : "w-9 justify-center"
            }`}
          >
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              className="size-4 shrink-0 rounded-sm p-0"
              onClick={() => setFilterOpen(true)}
              aria-label="Filter agents"
            >
              <MagnifyingGlassIcon className="size-4" />
            </Button>
            <input
              ref={filterInputRef}
              placeholder="Filter agents..."
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              tabIndex={filterOpen || filter ? 0 : -1}
              className={`h-7 min-w-0 border-0 bg-transparent p-0 text-sm text-inherit outline-none placeholder:text-muted-foreground ${
                filterOpen || filter ? "w-full opacity-100" : "w-0 opacity-0"
              }`}
            />
            {(filterOpen || filter) && (
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                className="size-4 shrink-0 rounded-sm p-0 text-muted-foreground hover:text-foreground"
                onClick={() => {
                  setFilter("");
                  setFilterOpen(false);
                }}
                aria-label="Close filter"
              >
                <XIcon className="size-3" />
              </Button>
            )}
          </div>
        </div>
      </div>

      {createOpen && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            e.stopPropagation();
            form.handleSubmit();
          }}
          noValidate
          className="mb-4 rounded-xl border bg-card p-4 shadow-sm shadow-primary/5"
        >
          <div className="grid gap-3">
            <form.Field
              name="name"
              children={(field) => (
                <Field className="gap-1" data-invalid={field.state.meta.errors.length > 0 || undefined}>
                  <FieldLabel className="text-xs">Name <span aria-hidden="true">*</span><span className="sr-only"> required</span></FieldLabel>
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
                <Field className="gap-1" data-invalid={field.state.meta.errors.length > 0 || undefined}>
                  <FieldLabel className="text-xs">Instructions</FieldLabel>
                  <Textarea
                    className="min-h-28"
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
              selector={(state) => state.values.provider}
              children={(provider) => {
                const providerModels = models[provider] ?? [];
                return (
                  <div className="grid gap-3 sm:grid-cols-2">
                    <form.Field
                      name="provider"
                      children={(field) => (
                        <Field className="gap-1" data-invalid={field.state.meta.errors.length > 0 || undefined}>
                          <FieldLabel className="text-xs">Provider</FieldLabel>
                          <Select
                            value={field.state.value}
                            onValueChange={(value) => {
                              field.handleChange(value ?? "");
                              form.setFieldValue("model", "");
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
                          <FieldError errors={field.state.meta.errors as Array<{ message?: string }>} />
                        </Field>
                      )}
                    />
                    <form.Field
                      name="model"
                      children={(field) => (
                        <Field className="gap-1" data-invalid={field.state.meta.errors.length > 0 || undefined}>
                          <FieldLabel className="text-xs">Model</FieldLabel>
                          <Select
                            value={field.state.value}
                            onValueChange={(value) => field.handleChange(value ?? "")}
                            disabled={!provider || providerModels.length === 0}
                          >
                            <SelectTrigger>
                              <SelectValue placeholder={provider ? "Select model" : "Select provider first"} />
                            </SelectTrigger>
                            <SelectContent>
                              {providerModels.map((model) => (
                                <SelectItem key={model.id} value={model.id}>
                                  {model.id}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                          <FieldError errors={field.state.meta.errors as Array<{ message?: string }>} />
                        </Field>
                      )}
                    />
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
                      <Button type="button" variant="ghost" size="xs" onClick={() => field.handleChange(availableTools)}>
                        All on
                      </Button>
                      <Button type="button" variant="ghost" size="xs" onClick={() => field.handleChange([])}>
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
                        <Badge variant={field.state.value.includes(tool.name) ? "default" : "outline"}>{tool.name}</Badge>
                      </Button>
                    ))}
                  </div>
                </div>
              )}
            />
            <form.Field
              name="outputSchema"
              children={(field) => (
                <Field className="gap-1" data-invalid={field.state.meta.errors.length > 0 || undefined}>
                  <FieldLabel className="text-xs">Output schema JSON</FieldLabel>
                  <Textarea
                    className="min-h-24"
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
              <Button type="submit" disabled={saving}>{saving ? "Saving..." : "Create"}</Button>
            </div>
          </div>
        </form>
      )}

      {loading ? (
        <div className="flex items-center gap-3 py-8 text-sm text-muted-foreground">
          <HexWaveSpinner size={24} />
          <span>Loading...</span>
        </div>
      ) : filteredAgents.length === 0 && filter ? (
        <Empty>
          <EmptyTitle>No agents found</EmptyTitle>
          <EmptyDescription>Try a different search.</EmptyDescription>
        </Empty>
      ) : filteredAgents.length === 0 ? (
        <Empty>
          <EmptyTitle>No agents yet</EmptyTitle>
          <EmptyDescription>Create an agent to define reusable model instructions and tools.</EmptyDescription>
        </Empty>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Model</TableHead>
              <TableHead>Tools</TableHead>
              <TableHead>Created</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredAgents.map((agent) => (
              <TableRow
                key={agent.id}
                className="cursor-pointer"
                onClick={() => navigate({ to: "/agents/$agentId", params: { agentId: agent.id } })}
              >
                <TableCell className="font-medium">{agent.name}</TableCell>
                <TableCell className="text-muted-foreground">{agent.model_ref || "-"}</TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {(agent.tools ?? []).map((tool) => (
                      <Badge key={tool} variant="outline">
                        {tool}
                      </Badge>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="text-muted-foreground">{timeAgo(agent.created_at)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
