import { useEffect, useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
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
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { wfetch } from "@/lib/client";
import { showErrorToast } from "@/lib/toast";
import type { Agent, Provider, ProviderModel } from "@/lib/types";
import { splitModelRef } from "@/lib/utils";

const builtInTools = ["apply_patch", "bash", "read", "write", "edit", "glob", "grep", "webfetch", "websearch"];

interface AgentForm {
  name: string;
  instructions: string;
  provider: string;
  model: string;
  tools: string[];
  outputSchema: string;
}

function formFromAgent(agent: Agent): AgentForm {
  const tools = new Set(builtInTools);
  const modelRef = splitModelRef(agent.model_ref);
  return {
    name: agent.name,
    instructions: agent.instructions ?? "",
    provider: modelRef.provider,
    model: modelRef.model,
    tools: (agent.tools ?? []).filter((tool) => tools.has(tool)),
    outputSchema: agent.output_schema && Object.keys(agent.output_schema).length > 0
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
  const [agent, setAgent] = useState<Agent | null>(null);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [models, setModels] = useState<Record<string, ProviderModel[]>>({});
  const [form, setForm] = useState<AgentForm | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);

  async function load() {
    try {
      const [agentData, providerData] = await Promise.all([
        wfetch(`/agents/${agentId}`) as Promise<Agent>,
        wfetch("/provider") as Promise<Provider[]>,
      ]);
      setAgent(agentData);
      setForm(formFromAgent(agentData));
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
  }, [agentId]);

  function toggleTool(tool: string) {
    setForm((prev) => {
      if (!prev) return prev;
      return {
        ...prev,
        tools: prev.tools.includes(tool) ? prev.tools.filter((item) => item !== tool) : [...prev.tools, tool],
      };
    });
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!form?.name.trim()) return;
    setSaving(true);
    try {
      let output_schema: Record<string, unknown> | undefined;
      if (form.outputSchema.trim()) {
        output_schema = JSON.parse(form.outputSchema);
      }
      const updated = (await wfetch(`/agents/${agentId}`, {
        method: "PUT",
        body: JSON.stringify({
          name: form.name.trim(),
          instructions: form.instructions,
          model_ref: form.provider && form.model ? `${form.provider}/${form.model}` : "",
          tools: form.tools,
          output_schema,
        }),
      })) as Agent;
      setAgent(updated);
      setForm(formFromAgent(updated));
    } catch (err) {
      showErrorToast(err);
    } finally {
      setSaving(false);
    }
  }

  async function remove() {
    if (!agent) return;
    setDeleting(true);
    try {
      await wfetch(`/agents/${agent.id}`, { method: "DELETE" });
      navigate({ to: "/agents" });
    } catch (err) {
      showErrorToast(err);
      setDeleting(false);
    }
  }

  const providerModels = form ? models[form.provider] ?? [] : [];
  const crumbLabel = agent?.name || agentId;

  return (
    <div className="mx-auto max-w-5xl px-4 py-6">
      <div className="mb-4 flex items-center justify-between gap-4">
        <PageBreadcrumb items={[{ label: "Agents", to: "/agents" }, { label: crumbLabel }]} />
        {agent && (
          <AlertDialog>
            <AlertDialogTrigger render={<Button size="sm" variant="destructive" disabled={deleting} />}>
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
        <div className="py-8 text-sm text-muted-foreground">Loading...</div>
      ) : !agent || !form ? (
        <div className="py-8 text-sm text-muted-foreground">Agent not found.</div>
      ) : (
        <form onSubmit={save} className="grid gap-4 rounded-lg border bg-card p-4">
          <div className="grid gap-1">
            <label className="text-xs font-medium">Name</label>
            <Input
              value={form.name}
              onChange={(e) => setForm((prev) => prev && { ...prev, name: e.target.value })}
            />
          </div>
          <div className="grid gap-1">
            <label className="text-xs font-medium">Instructions</label>
            <Textarea
              className="min-h-40"
              value={form.instructions}
              onChange={(e) => setForm((prev) => prev && { ...prev, instructions: e.target.value })}
            />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="grid gap-1">
              <label className="text-xs font-medium">Provider</label>
              <Select
                value={form.provider}
                onValueChange={(value) => setForm((prev) => prev && { ...prev, provider: value ?? "", model: "" })}
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
                onValueChange={(value) => setForm((prev) => prev && { ...prev, model: value ?? "" })}
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
                <Button type="button" variant="ghost" size="xs" onClick={() => setForm((prev) => prev && { ...prev, tools: builtInTools })}>
                  All on
                </Button>
                <Button type="button" variant="ghost" size="xs" onClick={() => setForm((prev) => prev && { ...prev, tools: [] })}>
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
              className="min-h-32 font-mono text-xs"
              placeholder="Optional JSON Schema"
              value={form.outputSchema}
              onChange={(e) => setForm((prev) => prev && { ...prev, outputSchema: e.target.value })}
            />
          </div>
          <div className="flex justify-end">
            <Button type="submit" disabled={saving || !form.name.trim()}>
              {saving ? "Saving..." : "Save changes"}
            </Button>
          </div>
        </form>
      )}
    </div>
  );
}
