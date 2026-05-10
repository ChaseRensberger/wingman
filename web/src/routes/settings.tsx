import { createFileRoute } from "@tanstack/react-router";
import { Card, CardAction, CardDescription, CardHeader, CardTitle } from "@/components/core/card";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { ThemeToggle } from "@/components/theme-toggle";

export const Route = createFileRoute("/settings")({
  component: SettingsPage,
});

function SettingsPage() {
  return (
    <div className="mx-auto max-w-5xl px-4 py-6">
      <div className="mb-4">
        <PageBreadcrumb items={[{ label: "Settings" }]} />
      </div>

      <Card size="sm">
        <CardHeader>
          <div>
            <CardTitle>Appearance</CardTitle>
            <CardDescription>Choose how Wingman should render the interface.</CardDescription>
          </div>
          <CardAction>
            <ThemeToggle />
          </CardAction>
        </CardHeader>
      </Card>
    </div>
  );
}
