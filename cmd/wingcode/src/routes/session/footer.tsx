import { theme } from "../../theme"

export function Footer() {
  return (
    <box flexDirection="row" justifyContent="space-between" gap={1} flexShrink={0}>
      <text fg={theme.textMuted}>{process.cwd()}</text>
      <box gap={2} flexDirection="row" flexShrink={0}>
        <text fg={theme.text}>
          <span style={{ fg: theme.success }}>•</span> anthropic
        </text>
      </box>
    </box>
  )
}
