import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState, useCallback } from "react";
import { api, type ProvidersAuthResponse, type AuthCredential } from "@/lib/api";
import { Button } from "@wingman/core/components/primitives/button";
import { Input } from "@wingman/core/components/primitives/input";
import { Label } from "@wingman/core/components/primitives/label";
import { Badge } from "@wingman/core/components/primitives/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@wingman/core/components/primitives/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@wingman/core/components/primitives/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@wingman/core/components/primitives/select";
import { Plus, Trash2, KeyRound } from "lucide-react";

export const Route = createFileRoute("/auth")({
  component: AuthPage,
});

const PROVIDERS = [
  { id: "anthropic", label: "Anthropic", authType: "api_key" },
  { id: "openai", label: "OpenAI", authType: "api_key" },
  { id: "ollama", label: "Ollama", authType: "base_url" },
];

function AuthPage() {
  const [auth, setAuth] = useState<ProvidersAuthResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  const fetchAuth = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await api.getProvidersAuth();
      setAuth(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to fetch auth");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchAuth();
  }, [fetchAuth]);

  const handleDelete = async (provider: string) => {
    try {
      await api.deleteProviderAuth(provider);
      fetchAuth();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to delete auth");
    }
  };

  const providers = auth?.providers ?? {};

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Auth</h1>
        <Dialog open={createOpen} onOpenChange={setCreateOpen}>
          <DialogTrigger asChild>
            <Button size="sm">
              <Plus className="size-4 mr-1" />
              Add Provider
            </Button>
          </DialogTrigger>
          <CreateAuthDialog
            onCreated={() => {
              setCreateOpen(false);
              fetchAuth();
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
      ) : Object.keys(providers).length === 0 ? (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <KeyRound className="size-10 text-muted-foreground mb-3" />
          <p className="text-sm text-muted-foreground">No providers configured. Add one to get started.</p>
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {Object.entries(providers).map(([name, info]) => (
            <Card key={name}>
              <CardHeader className="pb-2">
                <div className="flex items-center justify-between">
                  <CardTitle className="text-sm font-medium capitalize">{name}</CardTitle>
                  <div className="flex items-center gap-2">
                    <Badge variant={info.configured ? "default" : "secondary"}>
                      {info.configured ? "Configured" : "Not configured"}
                    </Badge>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-7"
                      onClick={() => handleDelete(name)}
                    >
                      <Trash2 className="size-3.5 text-muted-foreground" />
                    </Button>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="pt-0">
                <p className="text-xs text-muted-foreground">Type: {info.type}</p>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {auth?.updated_at && (
        <p className="text-xs text-muted-foreground">
          Last updated {new Date(auth.updated_at).toLocaleString()}
        </p>
      )}
    </div>
  );
}

function CreateAuthDialog({ onCreated }: { onCreated: () => void }) {
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
