import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useQueries, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Input } from "@wingman/core/components/core/input";
import { Badge } from "@wingman/core/components/core/badge";
import { Card } from "@wingman/core/components/core/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@wingman/core/components/core/table";
import { Spinner } from "@wingman/core/components/core/spinner";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { providerModelsQuery, providersQuery } from "@/lib/queries";
import { showErrorToast } from "@/lib/toast";
import type { Provider } from "@/lib/types";

function formatAuthType(authType: Provider["auth_types"][number]) {
  return authType.name || authType.type.replaceAll("_", " ");
}

function authStatusLabel(provider: Provider) {
  return provider.auth.configured || provider.auth.source === "disabled"
    ? "Configured"
    : "Unconfigured";
}

export const Route = createFileRoute("/providers/")({
  component: ProvidersPage,
});

function ProvidersPage() {
  const navigate = useNavigate();
  const [filter, setFilter] = useState("");
  const providersResult = useQuery(providersQuery);
  const providers = providersResult.data ?? [];
  const modelResults = useQueries({
    queries: providers.map((provider) => providerModelsQuery(provider.id)),
  });
  const models = Object.fromEntries(
    providers.map((provider, index) => [provider.id, modelResults[index].data ?? []]),
  );
  const loading = providersResult.isPending || modelResults.some((result) => result.isPending);

  useEffect(() => {
    if (providersResult.error) showErrorToast(providersResult.error);
  }, [providersResult.error]);

  const configuredCount = providers.filter((provider) => provider.auth.configured).length;
  const modelCount = Object.values(models).reduce(
    (total, providerModels) => total + providerModels.length,
    0,
  );
  const filteredProviders = providers.filter((provider) => {
    const haystack = `${provider.name} ${provider.id}`.toLowerCase();
    return haystack.includes(filter.toLowerCase());
  });

  return (
    <div className="mx-auto max-w-6xl px-4 py-6">
      <div className="mb-4 flex flex-col gap-4">
        <div>
          <PageBreadcrumb items={[{ label: "Providers" }]} />
        </div>
        <div className="grid gap-2 sm:grid-cols-3">
          <Card size="sm" className="gap-0 px-3 py-2">
            <div className="text-xs text-muted-foreground">Providers</div>
            <div className="text-lg font-semibold">{providers.length}</div>
          </Card>
          <Card size="sm" className="gap-0 px-3 py-2">
            <div className="text-xs text-muted-foreground">Configured</div>
            <div className="text-lg font-semibold">{configuredCount}</div>
          </Card>
          <Card size="sm" className="gap-0 px-3 py-2">
            <div className="text-xs text-muted-foreground">Models</div>
            <div className="text-lg font-semibold">{modelCount}</div>
          </Card>
        </div>
      </div>

      <Input
        placeholder="Filter providers..."
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        className="mb-4"
      />

      {loading ? (
        <div className="flex items-center gap-3 py-8 text-sm text-muted-foreground">
          <Spinner size={24} />
          <span>Loading...</span>
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Provider</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Route</TableHead>
              <TableHead>Auth</TableHead>
              <TableHead>Models</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredProviders.map((provider) => {
              return (
                <TableRow
                  key={provider.id}
                  className="cursor-pointer"
                  onClick={() =>
                    navigate({ to: "/providers/$providerId", params: { providerId: provider.id } })
                  }
                >
                  <TableCell>
                    <div className="font-medium">{provider.name}</div>
                    <div className="text-xs text-muted-foreground">{provider.id}</div>
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={authStatusLabel(provider) === "Configured" ? "default" : "secondary"}
                    >
                      {authStatusLabel(provider)}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="max-w-[18rem] truncate font-mono text-xs text-muted-foreground">
                      {provider.route.base_url || "-"}
                    </div>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {provider.auth_types.map(formatAuthType).join(", ") || "-"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {models[provider.id]?.length ?? 0}
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
