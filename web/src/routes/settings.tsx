import { createFileRoute } from "@tanstack/react-router";
import { DesktopIcon, MoonIcon, SunIcon } from "@phosphor-icons/react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/core/card";
import { RadioGroup, RadioGroupItem } from "@/components/core/radio-group";
import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { type Theme, useTheme } from "@/components/theme-provider";
import { cn } from "@/lib/utils";

export const Route = createFileRoute("/settings")({
  component: SettingsPage,
});

function SettingsPage() {
  const { theme, setTheme } = useTheme();
  const options = [
    { value: "light", label: "Light", icon: SunIcon },
    { value: "dark", label: "Dark", icon: MoonIcon },
    { value: "system", label: "System", icon: DesktopIcon },
  ] as const;

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
              className="inline-grid w-full max-w-md grid-cols-3 rounded-xl border bg-muted/45 p-1"
            >
              {options.map((option) => {
                const Icon = option.icon;
                const active = theme === option.value;
                return (
                <label
                  key={option.value}
                  className={cn(
                    "flex cursor-pointer items-center justify-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-all",
                    active
                      ? "bg-background text-foreground shadow-sm ring-1 ring-border/80"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  <RadioGroupItem value={option.value} className="sr-only" />
                  <Icon className="size-4" />
                  <span>{option.label}</span>
                </label>
              )})}
            </RadioGroup>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
