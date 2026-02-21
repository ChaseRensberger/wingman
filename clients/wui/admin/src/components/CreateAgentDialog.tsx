import { useState } from "react";
import { api, type CreateAgentRequest, type ProviderConfig } from "@/lib/api";
import { Button } from "@wingman/core/components/primitives/button";
import { Input } from "@wingman/core/components/primitives/input";
import { Label } from "@wingman/core/components/primitives/label";
import { Textarea } from "@wingman/core/components/primitives/textarea";
import { Badge } from "@wingman/core/components/primitives/badge";
import {
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@wingman/core/components/primitives/dialog";

const AVAILABLE_TOOLS = ["bash", "read", "write", "edit", "glob", "grep", "webfetch"];

type CreateAgentDialogProps = {
  onCreated: () => void;
};

export function CreateAgentDialog({ onCreated }: CreateAgentDialogProps) {
  const [name, setName] = useState("");
  const [instructions, setInstructions] = useState("");
  const [tools, setTools] = useState<string[]>([]);
  const [providerId, setProviderId] = useState("");
  const [model, setModel] = useState("");
  const [maxTokens, setMaxTokens] = useState("");
  const [temperature, setTemperature] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async () => {
    if (!name.trim()) return;
    setSubmitting(true);
    setError(null);
    try {
      const req: CreateAgentRequest = { name: name.trim() };
      if (instructions.trim()) req.instructions = instructions.trim();
      if (tools.length > 0) req.tools = tools;
      if (providerId.trim()) {
        const provider: ProviderConfig = { id: providerId.trim() };
        if (model.trim()) provider.model = model.trim();
        if (maxTokens) provider.max_tokens = parseInt(maxTokens, 10);
        if (temperature) provider.temperature = parseFloat(temperature);
        req.provider = provider;
      }
      await api.createAgent(req);
      onCreated();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to create agent");
    } finally {
      setSubmitting(false);
    }
  };

  const toggleTool = (tool: string) => {
    setTools((prev) => (prev.includes(tool) ? prev.filter((t) => t !== tool) : [...prev, tool]));
  };

  return (
    <DialogContent className="max-w-md max-h-[90vh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle>Create Agent</DialogTitle>
        <DialogDescription>Configure a new agent with tools and provider settings.</DialogDescription>
      </DialogHeader>
      <div className="space-y-4">
        {error && (
          <div className="rounded-md border border-destructive/50 bg-destructive/10 p-2 text-sm text-destructive">
            {error}
          </div>
        )}
        <div className="space-y-2">
          <Label htmlFor="name">Name</Label>
          <Input id="name" value={name} onChange={(e) => setName(e.target.value)} placeholder="my-agent" />
        </div>
        <div className="space-y-2">
          <Label htmlFor="instructions">Instructions</Label>
          <Textarea
            id="instructions"
            value={instructions}
            onChange={(e) => setInstructions(e.target.value)}
            placeholder="You are a helpful assistant..."
            rows={3}
          />
        </div>
        <div className="space-y-2">
          <Label>Tools</Label>
          <div className="flex flex-wrap gap-1.5">
            {AVAILABLE_TOOLS.map((tool) => (
              <Badge
                key={tool}
                variant={tools.includes(tool) ? "default" : "outline"}
                className="cursor-pointer"
                onClick={() => toggleTool(tool)}
              >
                {tool}
              </Badge>
            ))}
          </div>
        </div>
        <div className="space-y-2">
          <Label>Provider</Label>
          <div className="grid grid-cols-2 gap-2">
            <Input value={providerId} onChange={(e) => setProviderId(e.target.value)} placeholder="Provider ID" />
            <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="Model" />
            <Input
              value={maxTokens}
              onChange={(e) => setMaxTokens(e.target.value)}
              placeholder="Max tokens"
              type="number"
            />
            <Input
              value={temperature}
              onChange={(e) => setTemperature(e.target.value)}
              placeholder="Temperature"
              type="number"
              step="0.1"
            />
          </div>
        </div>
      </div>
      <DialogFooter>
        <Button onClick={handleSubmit} disabled={!name.trim() || submitting}>
          {submitting ? "Creating..." : "Create"}
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
