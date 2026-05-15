import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { Input } from "@/components/core/input";
import { Badge } from "@/components/core/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/core/table";
import { wfetch } from "@/lib/client";
import type { Provider, ProviderModel } from "@/lib/types";
import { PageBreadcrumb } from "@/components/page-breadcrumb";

function formatAuthType(authType: Provider["auth_types"][number]) {
  return authType.name || authType.type.replaceAll("_", " ");
}

function authStatusLabel(provider: Provider) {
  if (provider.auth.source === "env") return "Env key";
  if (provider.auth.configured) return "Configured";
  return "Needs key";
}

export const Route = createFileRoute("/providers/")({
  component: ProvidersPage,
});

function ProvidersPage() {
  const navigate = useNavigate();
  const [providers, setProviders] = useState<Provider[]>([]);
  const [models, setModels] = useState<Record<string, ProviderModel[]>>({});
  const [filter, setFilter] = useState("");
  const [loading, setLoading] = useState(true);

  async function load() {
    try {
      const providerData = (await wfetch("/provider")) as Provider[];
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

  const configuredCount = providers.filter((provider) => provider.auth.configured).length;
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
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredProviders.map((provider) => {
              const capabilities = providerCapabilities(provider);
              return (
                <TableRow
                  key={provider.id}
                  className="cursor-pointer"
                  onClick={() => navigate({ to: "/providers/$providerId", params: { providerId: provider.id } })}
                >
                  <TableCell>
                    <div className="font-medium">{provider.name}</div>
                    <div className="text-xs text-muted-foreground">{provider.id}</div>
                  </TableCell>
                  <TableCell>
                    <Badge variant={provider.auth.configured ? "default" : "secondary"}>
                      {authStatusLabel(provider)}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {provider.auth_types.map(formatAuthType).join(", ") || "-"}
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
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
