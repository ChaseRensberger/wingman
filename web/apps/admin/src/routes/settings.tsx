import { createFileRoute } from "@tanstack/react-router";
import { useState, useEffect } from "react";
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
  const [url, setUrl] = useState(getBaseUrl());
  const [saved, setSaved] = useState(false);
  const [status, setStatus] = useState<"unknown" | "connected" | "error">("unknown");
  const [checking, setChecking] = useState(false);

  useEffect(() => {
    checkHealth();
  }, []);

  const checkHealth = async () => {
    setChecking(true);
    try {
      await api.health();
      setStatus("connected");
    } catch {
      setStatus("error");
    } finally {
      setChecking(false);
    }
  };

  const handleSave = () => {
    setBaseUrl(url.trim());
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
    checkHealth();
  };

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-sm">Server Connection</CardTitle>
            <Badge variant={status === "connected" ? "default" : status === "error" ? "destructive" : "secondary"}>
              {checking ? "Checking..." : status === "connected" ? "Connected" : status === "error" ? "Unreachable" : "Unknown"}
            </Badge>
          </div>
          <CardDescription className="text-xs">
            Configure the base URL for the Wingman HTTP server.
          </CardDescription>
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
          <Button variant="outline" size="sm" onClick={checkHealth} disabled={checking}>
            {checking ? "Checking..." : "Test Connection"}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
