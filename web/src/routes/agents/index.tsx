import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { Button } from "@/components/core/button";
import { Input } from "@/components/core/input";
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
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/core/dialog";
import { wfetch } from "@/lib/client";
import { timeAgo } from "@/lib/utils";
import type { Agent, Provider, ProviderModel } from "@/lib/types";
import { PlusIcon } from "@phosphor-icons/react";

const builtInTools = ["bash", "read", "write", "edit", "glob", "grep", "webfetch", "perplexity_search"];

interface AgentForm {
  id?: string;
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
  tools: [],
  outputSchema: "",
};

function schemaText(agent: Agent): string {
  if (!agent.output_schema || Object.keys(agent.output_schema).length === 0) return "";
  return JSON.stringify(agent.output_schema, null, 2);
}

export const Route = createFileRoute("/agents/")({
  component: AgentsPage,
});

function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [models, setModels] = useState<Record<string, ProviderModel[]>>({});
  const [form, setForm] = useState<AgentForm>(emptyForm);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);

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
    load().catch((err) => alert(String(err)));
  }, []);

  function openNew() {
    setForm(emptyForm);
    setDialogOpen(true);
  }

  function openEdit(agent: Agent) {
    setForm({
      id: agent.id,
      name: agent.name,
      instructions: agent.instructions ?? "",
      provider: agent.provider ?? "",
      model: agent.model ?? "",
      tools: agent.tools ?? [],
      outputSchema: schemaText(agent),
    });
    setDialogOpen(true);
  }

  function toggleTool(tool: string) {
    setForm((prev) => ({
      ...prev,
      tools: prev.tools.includes(tool) ? prev.tools.filter((item) => item !== tool) : [...prev.tools, tool],
    }));
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
        provider: form.provider,
        model: form.model,
        tools: form.tools,
        output_schema,
      });
      if (form.id) {
        await wfetch(`/agents/${form.id}`, { method: "PUT", body });
      } else {
        await wfetch("/agents", { method: "POST", body });
      }
      setForm(emptyForm);
      setDialogOpen(false);
      await load();
    } catch (err) {
      alert(String(err));
    } finally {
      setSaving(false);
    }
  }

  async function remove(agent: Agent) {
    if (!confirm(`Delete agent ${agent.name}?`)) return;
    await wfetch(`/agents/${agent.id}`, { method: "DELETE" });
    await load();
  }

  const providerModels = models[form.provider] ?? [];

  return (
    <div className="mx-auto max-w-5xl px-4 py-6">
      <div className="mb-4 flex items-center justify-between">
        <div>
          <h1 className="text-base font-semibold">Agents</h1>
          <p className="text-sm text-muted-foreground">Create and maintain reusable Wingman agent definitions.</p>
        </div>
        <Button size="sm" onClick={openNew}>
          <PlusIcon className="size-4" />
          New agent
        </Button>
      </div>

      {loading ? (
        <div className="py-8 text-sm text-muted-foreground">Loading...</div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Provider</TableHead>
              <TableHead>Model</TableHead>
              <TableHead>Tools</TableHead>
              <TableHead>Created</TableHead>
              <TableHead className="w-0 text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {agents.map((agent) => (
              <TableRow key={agent.id}>
                <TableCell className="font-medium">{agent.name}</TableCell>
                <TableCell className="text-muted-foreground">{agent.provider || "-"}</TableCell>
                <TableCell className="text-muted-foreground">{agent.model || "-"}</TableCell>
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
                <TableCell className="text-right">
                  <Button size="sm" variant="outline" onClick={() => openEdit(agent)}>
                    Edit
                  </Button>{" "}
                  <Button size="sm" variant="destructive" onClick={() => remove(agent)}>
                    Delete
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <form onSubmit={save}>
            <DialogHeader>
              <DialogTitle>{form.id ? "Edit agent" : "New agent"}</DialogTitle>
            </DialogHeader>
            <div className="grid gap-3 py-4">
              <div className="grid gap-1">
                <label className="text-xs font-medium">Name</label>
                <Input
                  value={form.name}
                  onChange={(e) => setForm((prev) => ({ ...prev, name: e.target.value }))}
                />
              </div>
              <div className="grid gap-1">
                <label className="text-xs font-medium">Instructions</label>
                <Textarea
                  className="min-h-28"
                  value={form.instructions}
                  onChange={(e) => setForm((prev) => ({ ...prev, instructions: e.target.value }))}
                />
              </div>
              <div className="grid gap-1">
                <label className="text-xs font-medium">Provider</label>
                <select
                  className="h-9 rounded-md border border-input bg-background px-3 text-sm"
                  value={form.provider}
                  onChange={(e) => setForm((prev) => ({ ...prev, provider: e.target.value, model: "" }))}
                >
                  <option value="">Select provider</option>
                  {providers.map((provider) => (
                    <option key={provider.id} value={provider.id}>
                      {provider.name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="grid gap-1">
                <label className="text-xs font-medium">Model</label>
                <select
                  className="h-9 rounded-md border border-input bg-background px-3 text-sm"
                  value={form.model}
                  onChange={(e) => setForm((prev) => ({ ...prev, model: e.target.value }))}
                >
                  <option value="">Select model</option>
                  {providerModels.map((model) => (
                    <option key={model.id} value={model.id}>
                      {model.id}
                    </option>
                  ))}
                </select>
              </div>
              <div className="grid gap-2">
                <label className="text-xs font-medium">Tools</label>
                <div className="grid grid-cols-2 gap-2">
                  {builtInTools.map((tool) => (
                    <label key={tool} className="flex items-center gap-2 text-xs">
                      <input
                        type="checkbox"
                        checked={form.tools.includes(tool)}
                        onChange={() => toggleTool(tool)}
                      />
                      {tool}
                    </label>
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
            </div>
            <DialogFooter>
              <Button type="submit" disabled={saving || !form.name.trim()}>
                {saving ? "Saving..." : "Save"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
