import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { Button } from "@/components/core/button";
import { Input } from "@/components/core/input";
import { Badge } from "@/components/core/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/core/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/core/table";
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

  return (
    <div className="mx-auto max-w-6xl px-4 py-6">
      <div className="mb-4">
        <PageBreadcrumb items={[{ label: "Providers" }]} />
        <p className="mt-1 text-sm text-muted-foreground">Configure provider auth and inspect available model capabilities.</p>
      </div>

      {loading ? (
        <div className="py-8 text-sm text-muted-foreground">Loading...</div>
      ) : (
        <div className="grid gap-4">
          {providers.map((provider) => {
            const configured = auth.providers[provider.id]?.configured;
            const providerModels = models[provider.id] ?? [];
            return (
              <Card key={provider.id}>
                <CardHeader>
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div>
                      <CardTitle>{provider.name}</CardTitle>
                      <CardDescription>{provider.id}</CardDescription>
                    </div>
                    <Badge variant={configured ? "default" : "secondary"}>{configured ? "configured" : "not configured"}</Badge>
                  </div>
                </CardHeader>
                <CardContent className="grid gap-4">
                  {provider.auth_types.includes("api_key") && (
                    <div className="flex flex-col gap-2 sm:flex-row">
                      <Input
                        type="password"
                        value={keys[provider.id] ?? ""}
                        placeholder="API key"
                        onChange={(e) => setKeys((prev) => ({ ...prev, [provider.id]: e.target.value }))}
                      />
                      <Button onClick={() => saveKey(provider)}>Save key</Button>
                      {configured && <Button variant="destructive" onClick={() => deleteKey(provider)}>Delete key</Button>}
                    </div>
                  )}

                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Model</TableHead>
                        <TableHead>Context</TableHead>
                        <TableHead>Output</TableHead>
                        <TableHead>Capabilities</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {providerModels.slice(0, 25).map((model) => (
                        <TableRow key={model.id}>
                          <TableCell className="font-medium">{model.id}</TableCell>
                          <TableCell className="text-muted-foreground">{model.context_window || "-"}</TableCell>
                          <TableCell className="text-muted-foreground">{model.max_output || "-"}</TableCell>
                          <TableCell className="flex flex-wrap gap-1">
                            {model.tools && <Badge variant="outline">tools</Badge>}
                            {model.images && <Badge variant="outline">images</Badge>}
                            {model.reasoning && <Badge variant="outline">reasoning</Badge>}
                            {model.structured_output && <Badge variant="outline">structured</Badge>}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                  {providerModels.length > 25 && <div className="text-xs text-muted-foreground">Showing 25 of {providerModels.length} models.</div>}
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}
