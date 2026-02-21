import { createFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { getBaseUrl, setBaseUrl, api } from "@/lib/api";
import { Button } from "@wingman/core/components/primitives/button";
import { Input } from "@wingman/core/components/primitives/input";
import { Label } from "@wingman/core/components/primitives/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@wingman/core/components/primitives/card";
import { Badge } from "@wingman/core/components/primitives/badge";
import { Check } from "lucide-react";

export const Route = createFileRoute("/settings")({
  component: SettingsPage,
});

function SettingsPage() {
  const initialBaseUrl = getBaseUrl();
  const [url, setUrl] = useState(initialBaseUrl);
  const [savedBaseUrl, setSavedBaseUrl] = useState(initialBaseUrl);
  const [saved, setSaved] = useState(false);

  const healthQuery = useQuery({
    queryKey: ["health", savedBaseUrl],
    queryFn: () => api.health(),
    retry: false,
  });

  const handleSave = () => {
    const next = url.trim();
    if (!next) return;

    setBaseUrl(next);
    setSavedBaseUrl(next);

    setSaved(true);
    setTimeout(() => setSaved(false), 2000);

    if (next === savedBaseUrl) {
      healthQuery.refetch();
    }
  };

  const statusText = healthQuery.isFetching
    ? "Checking..."
    : healthQuery.isSuccess
      ? "Connected"
      : healthQuery.isError
        ? "Unreachable"
        : "Unknown";

  const statusVariant = healthQuery.isSuccess ? "default" : healthQuery.isError ? "destructive" : "secondary";

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-sm">Server Connection</CardTitle>
            <Badge variant={statusVariant}>{statusText}</Badge>
          </div>
          <CardDescription className="text-xs">Configure the base URL for the Wingman HTTP server.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="base-url">Base URL</Label>
            <div className="flex gap-2">
              <Input
                id="base-url"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="http://localhost:9999"
                className="flex-1"
              />
              <Button onClick={handleSave} disabled={!url.trim()}>
                {saved ? <Check className="size-4" /> : "Save"}
              </Button>
            </div>
          </div>

          <Button variant="outline" size="sm" onClick={() => healthQuery.refetch()} disabled={healthQuery.isFetching}>
            {healthQuery.isFetching ? "Checking..." : "Test Connection"}
          </Button>

          {healthQuery.error instanceof Error && (
            <p className="text-xs text-destructive">{healthQuery.error.message}</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
