import { useEffect, useState } from "react";
import { api, type AuthCredential } from "@/lib/api";
import { Button } from "@wingman/core/components/primitives/button";
import { Input } from "@wingman/core/components/primitives/input";
import { Label } from "@wingman/core/components/primitives/label";
import {
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@wingman/core/components/primitives/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@wingman/core/components/primitives/select";

const PROVIDERS = [
  { id: "anthropic", label: "Anthropic", authType: "api_key" },
  { id: "openai", label: "OpenAI", authType: "api_key" },
  { id: "ollama", label: "Ollama", authType: "base_url" },
];

type CreateAuthDialogProps = {
  onCreated: () => void;
};

export function CreateAuthDialog({ onCreated }: CreateAuthDialogProps) {
  const [provider, setProvider] = useState("");
  const [authType, setAuthType] = useState("api_key");
  const [apiKey, setApiKey] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const selectedProvider = PROVIDERS.find((p) => p.id === provider);

  useEffect(() => {
    if (selectedProvider) {
      setAuthType(selectedProvider.authType);
    }
  }, [selectedProvider]);

  const handleSubmit = async () => {
    if (!provider) return;
    setSubmitting(true);
    setError(null);
    try {
      const credential: AuthCredential = { type: authType };
      if (authType === "api_key") {
        credential.key = apiKey.trim();
      } else if (authType === "base_url") {
        credential.access_token = baseUrl.trim();
      }
      await api.setProvidersAuth({ providers: { [provider]: credential } });
      onCreated();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save auth");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <DialogContent className="max-w-sm">
      <DialogHeader>
        <DialogTitle>Add Provider Auth</DialogTitle>
        <DialogDescription>Configure credentials for a provider.</DialogDescription>
      </DialogHeader>
      <div className="space-y-4">
        {error && (
          <div className="rounded-md border border-destructive/50 bg-destructive/10 p-2 text-sm text-destructive">
            {error}
          </div>
        )}
        <div className="space-y-2">
          <Label>Provider</Label>
          <Select value={provider} onValueChange={setProvider}>
            <SelectTrigger>
              <SelectValue placeholder="Select provider" />
            </SelectTrigger>
            <SelectContent>
              {PROVIDERS.map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {p.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {provider && authType === "api_key" && (
          <div className="space-y-2">
            <Label htmlFor="api-key">API Key</Label>
            <Input
              id="api-key"
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder="sk-..."
            />
          </div>
        )}
        {provider && authType === "base_url" && (
          <div className="space-y-2">
            <Label htmlFor="base-url">Base URL</Label>
            <Input
              id="base-url"
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
              placeholder="http://localhost:11434"
            />
          </div>
        )}
      </div>
      <DialogFooter>
        <Button onClick={handleSubmit} disabled={!provider || submitting}>
          {submitting ? "Saving..." : "Save"}
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
