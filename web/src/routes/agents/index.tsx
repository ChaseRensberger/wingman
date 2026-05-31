import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/core/button";
import { Input } from "@/components/core/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/core/select";
import { Textarea } from "@/components/core/textarea";
import { Badge } from "@/components/core/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/core/table";
import { Empty, EmptyDescription, EmptyTitle } from "@/components/core/empty";
import { wfetch } from "@/lib/client";
import { showErrorToast } from "@/lib/toast";
import { timeAgo } from "@/lib/utils";
import type { Agent, Provider, ProviderModel } from "@/lib/types";
import { MagnifyingGlassIcon, PlusIcon, XIcon } from "@phosphor-icons/react";
import { PageBreadcrumb } from "@/components/page-breadcrumb";

const builtInTools = ["apply_patch", "bash", "read", "write", "edit", "glob", "grep", "webfetch", "websearch"];

interface AgentForm {
  name: string;
  instructions: string;
  provider: string;
  model: string;
  tools: string[];
  outputSchema: string;
}

const emptyForm: AgentForm = {
  name: "",
  instructions: "",
  provider: "",
  model: "",
  tools: builtInTools,
  outputSchema: "",
};

export const Route = createFileRoute("/agents/")({
  component: AgentsPage,
});

function AgentsPage() {
  const navigate = useNavigate();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [models, setModels] = useState<Record<string, ProviderModel[]>>({});
  const [form, setForm] = useState<AgentForm>(emptyForm);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const [filterOpen, setFilterOpen] = useState(false);
  const filterInputRef = useRef<HTMLInputElement>(null);

  async function load() {
    try {
      const [agentData, providerData] = await Promise.all([
        wfetch("/agents") as Promise<Agent[]>,
        wfetch("/provider") as Promise<Provider[]>,
      ]);
      setAgents(agentData);
      setProviders(providerData);
      const modelEntries = await Promise.all(
        providerData.map(async (provider) => {
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

  function toggleTool(tool: string) {
    setForm((prev) => ({
      ...prev,
      tools: prev.tools.includes(tool) ? prev.tools.filter((item) => item !== tool) : [...prev.tools, tool],
    }));
  }

  function openNew() {
    setForm(emptyForm);
    setCreateOpen((open) => !open);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!form.name.trim()) return;
    setSaving(true);
    try {
      let output_schema: Record<string, unknown> | undefined;
      if (form.outputSchema.trim()) {
        output_schema = JSON.parse(form.outputSchema);
      }
      const body = JSON.stringify({
        name: form.name.trim(),
        instructions: form.instructions,
        model_ref: form.provider && form.model ? `${form.provider}/${form.model}` : "",
        tools: form.tools,
        output_schema,
      });
      await wfetch("/agents", { method: "POST", body });
      setForm(emptyForm);
      setCreateOpen(false);
      await load();
    } catch (err) {
      showErrorToast(err);
    } finally {
      setSaving(false);
    }
  }

  const providerModels = models[form.provider] ?? [];
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
        <form onSubmit={save} className="mb-4 rounded-xl border bg-card p-4 shadow-sm shadow-primary/5">
          <div className="grid gap-3">
            <div className="grid gap-1">
              <label className="text-xs font-medium">Name</label>
              <Input value={form.name} onChange={(e) => setForm((prev) => ({ ...prev, name: e.target.value }))} />
            </div>
            <div className="grid gap-1">
              <label className="text-xs font-medium">Instructions</label>
              <Textarea
                className="min-h-28"
                value={form.instructions}
                onChange={(e) => setForm((prev) => ({ ...prev, instructions: e.target.value }))}
              />
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="grid gap-1">
                <label className="text-xs font-medium">Provider</label>
                <Select
                  value={form.provider}
                  onValueChange={(value) => setForm((prev) => ({ ...prev, provider: value ?? "", model: "" }))}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select provider" />
                  </SelectTrigger>
                  <SelectContent>
                    {providers.map((provider) => (
                      <SelectItem key={provider.id} value={provider.id}>
                        {provider.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1">
                <label className="text-xs font-medium">Model</label>
                <Select
                  value={form.model}
                  onValueChange={(value) => setForm((prev) => ({ ...prev, model: value ?? "" }))}
                  disabled={!form.provider || providerModels.length === 0}
                >
                  <SelectTrigger>
                    <SelectValue placeholder={form.provider ? "Select model" : "Select provider first"} />
                  </SelectTrigger>
                  <SelectContent>
                    {providerModels.map((model) => (
                      <SelectItem key={model.id} value={model.id}>
                        {model.id}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="grid gap-2">
              <div className="flex items-center justify-between gap-3">
                <label className="text-xs font-medium">Tools</label>
                <div className="flex gap-1">
                  <Button type="button" variant="ghost" size="xs" onClick={() => setForm((prev) => ({ ...prev, tools: builtInTools }))}>
                    All on
                  </Button>
                  <Button type="button" variant="ghost" size="xs" onClick={() => setForm((prev) => ({ ...prev, tools: [] }))}>
                    All off
                  </Button>
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                {builtInTools.map((tool) => (
                  <Button
                    key={tool}
                    type="button"
                    onClick={() => toggleTool(tool)}
                    variant="ghost"
                    className="h-auto rounded-md p-0"
                  >
                    <Badge variant={form.tools.includes(tool) ? "default" : "outline"}>{tool}</Badge>
                  </Button>
                ))}
              </div>
            </div>
            <div className="grid gap-1">
              <label className="text-xs font-medium">Output schema JSON</label>
              <Textarea
                className="min-h-24"
                placeholder="Optional JSON Schema"
                value={form.outputSchema}
                onChange={(e) => setForm((prev) => ({ ...prev, outputSchema: e.target.value }))}
              />
            </div>
            <div className="flex justify-end">
              <Button type="submit" disabled={saving || !form.name.trim()}>
                {saving ? "Saving..." : "Create"}
              </Button>
            </div>
          </div>
        </form>
      )}

      {loading ? (
        <div className="py-8 text-sm text-muted-foreground">Loading...</div>
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
