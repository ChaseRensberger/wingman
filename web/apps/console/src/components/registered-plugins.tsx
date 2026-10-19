import { useQuery } from "@tanstack/react-query";
import type { PluginsResponse } from "@wingman-actor/client";

import { client } from "@/lib/client";
import { Button } from "@wingman/core/components/core/button";

export function RegisteredPluginsContent({ inventory }: { inventory: PluginsResponse }) {
  const plugins = inventory.plugins ?? [];
  return (
    <div className="space-y-2">
      {plugins.length === 0 && (
        <p className="text-sm text-muted-foreground">No plugins are registered.</p>
      )}
      {plugins.map((plugin) => (
        <div
          key={plugin.id}
          className="flex items-center justify-between gap-3 rounded-lg border p-3"
        >
          <span className="min-w-0 break-words text-sm font-medium">{plugin.id}</span>
          <span className="shrink-0 text-xs text-muted-foreground">
            {plugin.enabled ? "Enabled" : "Disabled"}
          </span>
        </div>
      ))}
    </div>
  );
}

export function RegisteredPlugins() {
  const query = useQuery({
    queryKey: ["plugins"],
    queryFn: () => client.plugins.list(),
    refetchOnMount: "always",
  });

  return (
    <section className="space-y-3" aria-label="Registered plugins">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-medium">Plugins</h2>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          Refresh
        </Button>
      </div>
      {query.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">
          Loading plugins…
        </p>
      ) : query.isError ? (
        <p role="alert" className="text-sm text-destructive">
          Could not load plugins: {query.error.message}
        </p>
      ) : (
        <RegisteredPluginsContent inventory={query.data} />
      )}
    </section>
  );
}
