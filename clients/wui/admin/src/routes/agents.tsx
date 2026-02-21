import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState, useCallback } from "react";
import { api, type Agent } from "@/lib/api";
import { Button } from "@wingman/core/components/primitives/button";
import { Badge } from "@wingman/core/components/primitives/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@wingman/core/components/primitives/card";
import { Dialog, DialogTrigger } from "@wingman/core/components/primitives/dialog";
import { Plus, Trash2, ChevronRight, Bot } from "lucide-react";
import { CreateAgentDialog } from "@/components/CreateAgentDialog";

export const Route = createFileRoute("/agents")({
  component: AgentsPage,
});

function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [selectedAgent, setSelectedAgent] = useState<Agent | null>(null);

  const fetchAgents = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await api.listAgents();
      setAgents(data ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to fetch agents");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchAgents();
  }, [fetchAgents]);

  const handleDelete = async (id: string) => {
    try {
      await api.deleteAgent(id);
      if (selectedAgent?.id === id) setSelectedAgent(null);
      fetchAgents();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to delete agent");
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Agents</h1>
        <Dialog open={createOpen} onOpenChange={setCreateOpen}>
          <DialogTrigger asChild>
            <Button size="sm">
              <Plus className="size-4 mr-1" />
              New Agent
            </Button>
          </DialogTrigger>
          <CreateAgentDialog
            onCreated={() => {
              setCreateOpen(false);
              fetchAgents();
            }}
          />
        </Dialog>
      </div>

      {error && (
        <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-muted-foreground">Loading...</div>
      ) : agents.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <Bot className="size-10 text-muted-foreground mb-3" />
          <p className="text-sm text-muted-foreground">No agents yet. Create one to get started.</p>
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {agents.map((agent) => (
            <Card
              key={agent.id}
              className="cursor-pointer transition-colors hover:bg-accent/50"
              onClick={() => setSelectedAgent(selectedAgent?.id === agent.id ? null : agent)}
            >
              <CardHeader className="pb-2">
                <div className="flex items-center justify-between">
                  <CardTitle className="text-sm font-medium">{agent.name}</CardTitle>
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-7"
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDelete(agent.id);
                      }}
                    >
                      <Trash2 className="size-3.5 text-muted-foreground" />
                    </Button>
                    <ChevronRight
                      className={`size-4 text-muted-foreground transition-transform ${selectedAgent?.id === agent.id ? "rotate-90" : ""}`}
                    />
                  </div>
                </div>
                {agent.provider && (
                  <CardDescription className="text-xs">
                    {agent.provider.id}{agent.provider.model ? ` / ${agent.provider.model}` : ""}
                  </CardDescription>
                )}
              </CardHeader>
              {selectedAgent?.id === agent.id && (
                <CardContent className="pt-0 space-y-3">
                  {agent.instructions && (
                    <div>
                      <p className="text-xs font-medium text-muted-foreground mb-1">Instructions</p>
                      <p className="text-xs whitespace-pre-wrap">{agent.instructions}</p>
                    </div>
                  )}
                  {agent.tools && agent.tools.length > 0 && (
                    <div>
                      <p className="text-xs font-medium text-muted-foreground mb-1">Tools</p>
                      <div className="flex flex-wrap gap-1">
                        {agent.tools.map((tool) => (
                          <Badge key={tool} variant="secondary" className="text-xs">
                            {tool}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  )}
                  {agent.provider && (
                    <div>
                      <p className="text-xs font-medium text-muted-foreground mb-1">Provider</p>
                      <div className="text-xs space-y-0.5">
                        <p>ID: {agent.provider.id}</p>
                        {agent.provider.model && <p>Model: {agent.provider.model}</p>}
                        {agent.provider.max_tokens !== undefined && agent.provider.max_tokens > 0 && (
                          <p>Max Tokens: {agent.provider.max_tokens}</p>
                        )}
                        {agent.provider.temperature !== undefined && agent.provider.temperature > 0 && (
                          <p>Temperature: {agent.provider.temperature}</p>
                        )}
                      </div>
                    </div>
                  )}
                  <p className="text-xs text-muted-foreground">
                    Created {new Date(agent.created_at).toLocaleDateString()}
                  </p>
                </CardContent>
              )}
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
