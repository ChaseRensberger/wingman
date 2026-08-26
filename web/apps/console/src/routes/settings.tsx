import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@wingman/core/components/core/button";
import { Card, CardContent, CardHeader, CardTitle } from "@wingman/core/components/core/card";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { getDisplayName, setDisplayName } from "@/lib/greeting";
import { Input } from "@wingman/core/components/core/input";
import { ThemePreviewSwitcher } from "@wingman/core/components/theme-preview-switcher";
import { client } from "@/lib/client";
import { showErrorToast } from "@/lib/toast";
import { clientsQuery, queryKeys } from "@/lib/queries";

export const Route = createFileRoute("/settings")({ component: SettingsPage });

function SettingsPage() {
  const [displayName, setDisplayNameInput] = useState(getDisplayName());
  return (
    <div className="mx-auto max-w-5xl px-4 py-6">
      <div className="mb-4">
        <PageBreadcrumb items={[{ label: "Settings" }]} />
      </div>
      <div className="space-y-4">
        <ClientManagement />
        <Card size="sm">
          <CardHeader>
            <CardTitle>Console display name</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <Input
              className="max-w-md"
              value={displayName}
              onChange={(event) => {
                setDisplayNameInput(event.target.value);
                setDisplayName(event.target.value);
              }}
              placeholder="Your name"
            />
            <p className="text-sm text-muted-foreground">
              Used for personalized greetings when starting a new session.
            </p>
          </CardContent>
        </Card>
        <ThemePreviewSwitcher />
      </div>
    </div>
  );
}

type Client = { id: string; name: string };
function ClientManagement() {
  const [clientName, setClientName] = useState("");
  const [clientID, setClientID] = useState("");
  const queryClient = useQueryClient();
  const clientsResult = useQuery(clientsQuery);
  const createClient = useMutation({
    mutationFn: () =>
      client.clients.create({
        id: clientID.trim(),
        name: clientName.trim(),
      }) as Promise<{ client: Client }>,
    onSuccess: () => {
      setClientID("");
      setClientName("");
      return queryClient.invalidateQueries({ queryKey: queryKeys.clients });
    },
    onError: showErrorToast,
  });
  const clients = clientsResult.data ?? [];

  function submitClient() {
    if (clientID.trim() && clientName.trim()) createClient.mutate();
  }

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Clients</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          Named clients organize sessions and workspaces. They do not control access to Wingman.
        </p>
        <>
          <div className="flex flex-wrap gap-2">
            <Input
              value={clientID}
              onChange={(event) => setClientID(event.target.value)}
              placeholder="Client ID (cli_reference)"
            />
            <Input
              value={clientName}
              onChange={(event) => setClientName(event.target.value)}
              placeholder="Display name"
            />
            <Button
              size="sm"
              disabled={createClient.isPending || !clientID.trim() || !clientName.trim()}
              onClick={submitClient}
            >
              Create client
            </Button>
          </div>
          <div className="space-y-1 text-sm">
            {clients.map((client) => (
              <p key={client.id}>
                {client.name} <span className="font-mono text-muted-foreground">{client.id}</span>
              </p>
            ))}
          </div>
        </>
      </CardContent>
    </Card>
  );
}
