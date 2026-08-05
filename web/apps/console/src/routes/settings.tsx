import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { Card, CardContent, CardHeader, CardTitle } from "@wingman/core/components/core/card";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { getDisplayName, setDisplayName } from "@/lib/greeting";
import { Input } from "@wingman/core/components/core/input";
import { ThemePreviewSwitcher } from "@wingman/core/components/theme-preview-switcher";

export const Route = createFileRoute("/settings")({ component: SettingsPage });

function SettingsPage() {
  const [displayName, setDisplayNameInput] = useState(getDisplayName());
  return <div className="mx-auto max-w-5xl px-4 py-6">
    <div className="mb-4"><PageBreadcrumb items={[{ label: "Settings" }]} /></div>
    <div className="space-y-4">
      <Card size="sm"><CardHeader><CardTitle>Display name</CardTitle></CardHeader><CardContent className="space-y-2"><Input className="max-w-md" value={displayName} onChange={(event) => { setDisplayNameInput(event.target.value); setDisplayName(event.target.value); }} placeholder="Your name" /><p className="text-sm text-muted-foreground">Used for personalized greetings when starting a new session. Leave blank to stay incognito.</p></CardContent></Card>
      <ThemePreviewSwitcher />
    </div>
  </div>;
}
