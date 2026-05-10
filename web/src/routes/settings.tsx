import { createFileRoute } from "@tanstack/react-router";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/core/card";
import { RadioGroup, RadioGroupItem } from "@/components/core/radio-group";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { type Theme, useTheme } from "@/components/theme-provider";

export const Route = createFileRoute("/settings")({
  component: SettingsPage,
});

function SettingsPage() {
  const { theme, setTheme } = useTheme();

  return (
    <div className="mx-auto max-w-5xl px-4 py-6">
      <div className="mb-4">
        <PageBreadcrumb items={[{ label: "Settings" }]} />
      </div>

      <div className="grid gap-4">
        <Card size="sm">
          <CardHeader>
            <div>
              <CardTitle>Theme</CardTitle>
              <CardDescription>Choose how Wingman should render the interface.</CardDescription>
            </div>
          </CardHeader>
          <CardContent>
            <RadioGroup
              value={theme}
              onValueChange={(value) => setTheme(value as Theme)}
              className="grid gap-2 sm:grid-cols-3"
            >
              {[
                { value: "light", label: "Light", description: "Use the light interface." },
                { value: "dark", label: "Dark", description: "Use the dark interface." },
                { value: "system", label: "System", description: "Follow your OS setting." },
              ].map((option) => (
                <label
                  key={option.value}
                  className="flex cursor-pointer gap-3 rounded-lg border bg-background p-3 transition-colors hover:bg-accent"
                >
                  <RadioGroupItem value={option.value} className="mt-0.5" />
                  <span>
                    <span className="block text-sm font-medium">{option.label}</span>
                    <span className="block text-xs text-muted-foreground">{option.description}</span>
                  </span>
                </label>
              ))}
            </RadioGroup>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
