import { DesktopIcon, MoonIcon, SunIcon } from "@phosphor-icons/react"

import { Card, CardContent, CardHeader, CardTitle } from "#components/core/card"
import { RadioGroup, RadioGroupItem } from "#components/core/radio-group"
import { type ColorMode, useTheme } from "#components/theme-provider"
import { supportsColorMode, themes } from "#themes/registry"
import { cn } from "#lib/utils"

const colorModeOptions = [
  { value: "light", label: "Light", icon: SunIcon },
  { value: "dark", label: "Dark", icon: MoonIcon },
  { value: "system", label: "System", icon: DesktopIcon },
] as const

export function ThemePreviewSwitcher() {
  const { theme, colorMode, resolvedColorMode, setColorMode, setTheme } = useTheme()

  return (
    <div className="space-y-4">
      <Card size="sm">
        <CardHeader>
          <CardTitle>Color mode</CardTitle>
        </CardHeader>
        <CardContent>
          <RadioGroup
            value={colorMode}
            onValueChange={(value) => setColorMode(value as ColorMode)}
            className="inline-grid w-full max-w-md grid-cols-3 rounded-[var(--radius)] border bg-muted p-1"
          >
            {colorModeOptions.map((option) => {
              const Icon = option.icon
              const active = colorMode === option.value
              const available = supportsColorMode(theme, option.value)

              return (
                <label
                  key={option.value}
                  className={cn(
                    "flex items-center justify-center gap-2 rounded-[var(--radius)] px-3 py-2 text-sm font-medium transition-[color,background-color,box-shadow] duration-150",
                    available ? "cursor-pointer" : "cursor-not-allowed opacity-45",
                    active
                      ? "bg-background text-foreground shadow-sm ring-1 ring-border/80"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  <RadioGroupItem value={option.value} disabled={!available} className="sr-only" />
                  <Icon className="size-4" />
                  <span>{option.label}</span>
                </label>
              )
            })}
          </RadioGroup>
        </CardContent>
      </Card>
      <Card size="sm">
        <CardHeader>
          <CardTitle>Theme</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <RadioGroup value={theme.id} onValueChange={(value) => setTheme(value)} className="grid max-w-2xl gap-2 sm:grid-cols-3">
            {themes.map((option) => {
              const active = theme.id === option.id
              const previewMode = option.modes.includes(resolvedColorMode) ? resolvedColorMode : option.modes[0]

              return (
                <label
                  key={option.id}
                  className={cn(
                    "cursor-pointer rounded-[var(--radius)] border p-3 transition-[color,background-color,box-shadow] duration-150",
                    active ? "border-primary bg-primary/15 ring-1 ring-primary/30" : "hover:bg-muted"
                  )}
                >
                  <RadioGroupItem value={option.id} className="sr-only" />
                  <span data-theme={option.id} data-mode={previewMode} className="mb-3 flex overflow-hidden rounded border">
                    <span className="h-7 flex-1 bg-background" />
                    <span className="h-7 flex-1 bg-card" />
                    <span className="h-7 flex-1 bg-primary" />
                    <span className="h-7 flex-1 bg-destructive" />
                  </span>
                  <span className="flex items-center justify-between gap-2 text-sm font-medium">
                    {option.label}
                    {option.modes.length === 1 && <span className="text-xs font-normal text-muted-foreground">Dark</span>}
                  </span>
                </label>
              )
            })}
          </RadioGroup>
        </CardContent>
      </Card>
    </div>
  )
}
