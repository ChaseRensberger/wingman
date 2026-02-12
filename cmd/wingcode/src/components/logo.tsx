import { theme } from "../theme"

export function Logo() {
  return (
    <box flexDirection="column" alignItems="center">
      <text>
        <span style={{ fg: theme.primary, bold: true }}>Wing</span>
        <span style={{ bold: true }}>Code</span>
      </text>
      <text fg={theme.textMuted}>AI-powered coding assistant</text>
    </box>
  )
}
