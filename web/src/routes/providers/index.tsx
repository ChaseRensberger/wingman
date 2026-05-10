import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { Button } from "@/components/core/button";
import { Input } from "@/components/core/input";
import { Badge } from "@/components/core/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/core/table";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/core/dialog";
import { wfetch } from "@/lib/client";
import type { Provider, ProviderAuthResponse, ProviderModel } from "@/lib/types";
import { PageBreadcrumb } from "@/components/page-breadcrumb";

export const Route = createFileRoute("/providers/")({
  component: ProvidersPage,
});

function ProvidersPage() {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [auth, setAuth] = useState<ProviderAuthResponse>({ providers: {} });
  const [models, setModels] = useState<Record<string, ProviderModel[]>>({});
  const [keys, setKeys] = useState<Record<string, string>>({});
  const [filter, setFilter] = useState("");
  const [selectedProviderId, setSelectedProviderId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  async function load() {
    try {
      const [providerData, authData] = await Promise.all([
        wfetch("/provider") as Promise<Provider[]>,
        wfetch("/provider/auth") as Promise<ProviderAuthResponse>,
      ]);
      setProviders(providerData);
      setAuth(authData);

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

  async function saveKey(provider: Provider) {
    const key = keys[provider.id]?.trim();
    if (!key) return;
    await wfetch("/provider/auth", {
      method: "PUT",
      body: JSON.stringify({ providers: { [provider.id]: { type: "api_key", key } } }),
    });
    setKeys((prev) => ({ ...prev, [provider.id]: "" }));
    await load();
  }

  async function deleteKey(provider: Provider) {
    await wfetch(`/provider/auth/${provider.id}`, { method: "DELETE" });
    await load();
  }

  const selectedProvider = providers.find((provider) => provider.id === selectedProviderId) ?? null;
  const configuredCount = providers.filter((provider) => auth.providers[provider.id]?.configured).length;
  const modelCount = Object.values(models).reduce((total, providerModels) => total + providerModels.length, 0);
  const filteredProviders = providers.filter((provider) => {
    const haystack = `${provider.name} ${provider.id}`.toLowerCase();
    return haystack.includes(filter.toLowerCase());
  });

  function providerCapabilities(provider: Provider) {
    const providerModels = models[provider.id] ?? [];
    const capabilities = [
      providerModels.some((model) => model.tools) && "tools",
      providerModels.some((model) => model.images) && "images",
      providerModels.some((model) => model.reasoning) && "reasoning",
      providerModels.some((model) => model.structured_output) && "structured",
    ].filter(Boolean) as string[];
    return capabilities;
  }

  return (
    <div className="mx-auto max-w-6xl px-4 py-6">
      <div className="mb-4 flex flex-col gap-4">
        <div>
          <PageBreadcrumb items={[{ label: "Providers" }]} />
        </div>
        <div className="grid gap-2 sm:grid-cols-3">
          <div className="rounded-lg border bg-card px-3 py-2">
            <div className="text-xs text-muted-foreground">Providers</div>
            <div className="text-lg font-semibold">{providers.length}</div>
          </div>
          <div className="rounded-lg border bg-card px-3 py-2">
            <div className="text-xs text-muted-foreground">Configured</div>
            <div className="text-lg font-semibold">{configuredCount}</div>
          </div>
          <div className="rounded-lg border bg-card px-3 py-2">
            <div className="text-xs text-muted-foreground">Models</div>
            <div className="text-lg font-semibold">{modelCount}</div>
          </div>
        </div>
      </div>

      <Input
        placeholder="Filter providers..."
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        className="mb-4"
      />

      {loading ? (
        <div className="py-8 text-sm text-muted-foreground">Loading...</div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Provider</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Auth</TableHead>
              <TableHead>Models</TableHead>
              <TableHead>Capabilities</TableHead>
              <TableHead className="w-0 text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredProviders.map((provider) => {
              const configured = auth.providers[provider.id]?.configured;
              const capabilities = providerCapabilities(provider);
              return (
                <TableRow key={provider.id}>
                  <TableCell>
                    <div className="font-medium">{provider.name}</div>
                    <div className="text-xs text-muted-foreground">{provider.id}</div>
                  </TableCell>
                  <TableCell>
                    <Badge variant={configured ? "default" : "secondary"}>
                      {configured ? "Configured" : "Needs key"}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {provider.auth_types.map((authType) => authType.replace("_", " ")).join(", ") || "-"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">{models[provider.id]?.length ?? 0}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {capabilities.length > 0 ? capabilities.map((capability) => (
                        <Badge key={capability} variant="outline">
                          {capability}
                        </Badge>
                      )) : <span className="text-muted-foreground">-</span>}
                    </div>
                  </TableCell>
                  <TableCell className="text-right">
                    <Button size="sm" variant="outline" onClick={() => setSelectedProviderId(provider.id)}>
                      Manage key
                    </Button>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}

      <Dialog open={!!selectedProvider} onOpenChange={(open) => !open && setSelectedProviderId(null)}>
        <DialogContent>
          {selectedProvider && (
            <div className="grid gap-4">
              <DialogHeader>
                <DialogTitle>Manage {selectedProvider.name}</DialogTitle>
              </DialogHeader>
              <div className="grid gap-1 text-sm">
                <div className="text-xs text-muted-foreground">Provider ID</div>
                <div className="font-mono text-xs">{selectedProvider.id}</div>
              </div>
              <div className="flex items-center justify-between rounded-lg border bg-card px-3 py-2">
                <div>
                  <div className="text-sm font-medium">API key</div>
                  <div className="text-xs text-muted-foreground">
                    {auth.providers[selectedProvider.id]?.configured ? "A key is configured for this provider." : "No key configured."}
                  </div>
                </div>
                <Badge variant={auth.providers[selectedProvider.id]?.configured ? "default" : "secondary"}>
                  {auth.providers[selectedProvider.id]?.configured ? "Configured" : "Missing"}
                </Badge>
              </div>
              {selectedProvider.auth_types.includes("api_key") ? (
                <Input
                  type="password"
                  value={keys[selectedProvider.id] ?? ""}
                  placeholder={auth.providers[selectedProvider.id]?.configured ? "New API key" : "API key"}
                  onChange={(e) => setKeys((prev) => ({ ...prev, [selectedProvider.id]: e.target.value }))}
                />
              ) : (
                <div className="text-sm text-muted-foreground">This provider does not support API key auth.</div>
              )}
              <DialogFooter>
                {auth.providers[selectedProvider.id]?.configured && (
                  <Button variant="destructive" onClick={() => deleteKey(selectedProvider)}>
                    Delete key
                  </Button>
                )}
                <Button
                  onClick={() => saveKey(selectedProvider)}
                  disabled={!selectedProvider.auth_types.includes("api_key") || !keys[selectedProvider.id]?.trim()}
                >
                  {auth.providers[selectedProvider.id]?.configured ? "Replace key" : "Save key"}
                </Button>
              </DialogFooter>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
