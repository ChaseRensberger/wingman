import { createMemo, Show } from "solid-js"
import { theme } from "../theme"
import type { ToolCall } from "../types"

function formatToolInput(input: string): string {
  try {
    const parsed = JSON.parse(input)
    if (parsed.command) return String(parsed.command).slice(0, 80)
    if (parsed.filePath || parsed.path) return String(parsed.filePath || parsed.path)
    if (parsed.pattern) return String(parsed.pattern)
    if (parsed.url) return String(parsed.url).slice(0, 60)
    if (parsed.content) return `${String(parsed.content).slice(0, 40)}...`
    return input.slice(0, 60)
  } catch {
    return input.slice(0, 60)
  }
}

const TOOL_ICONS: Record<string, string> = {
  bash: "⚡",
  read: "📄",
  write: "✏",
  edit: "✎",
  glob: "🔍",
  grep: "🔎",
  webfetch: "🌐",
}

export function InlineToolCall(props: { tool: ToolCall }) {
  const icon = createMemo(() => TOOL_ICONS[props.tool.name] || "⚙")
  const complete = createMemo(() => props.tool.result !== undefined)
  const fg = createMemo(() => complete() ? theme.textMuted : theme.text)

  return (
    <box paddingLeft={3}>
      <text fg={fg()}>
        <span style={{ bold: true }}>{icon()}</span>{" "}
        <span style={{ fg: theme.textMuted }}>{props.tool.name}</span>{" "}
        {formatToolInput(props.tool.input)}
      </text>
      <Show when={props.tool.result && props.tool.name === "bash"}>
        <box paddingLeft={4}>
          <text fg={theme.textMuted}>
            {props.tool.result!.split("\n").slice(0, 3).join("\n")}
            {(props.tool.result!.split("\n").length > 3) ? "\n..." : ""}
          </text>
        </box>
      </Show>
    </box>
  )
}

export function BlockToolCall(props: { tool: ToolCall }) {
  const complete = createMemo(() => props.tool.result !== undefined)

  return (
    <box
      border={["left"]}
      borderColor={complete() ? theme.borderSubtle : theme.primary}
      customBorderChars={{
        topLeft: " ",
        topRight: " ",
        bottomLeft: " ",
        bottomRight: " ",
        horizontal: " ",
        vertical: "▏",
        topT: " ",
        bottomT: " ",
        leftT: " ",
        rightT: " ",
        cross: " ",
      }}
      paddingLeft={2}
      marginTop={1}
      backgroundColor={theme.backgroundPanel}
    >
      <box paddingTop={1} paddingBottom={1}>
        <text fg={theme.text}>
          <b>{props.tool.name}</b>
        </text>
        <Show when={props.tool.input}>
          <text fg={theme.textMuted}>{formatToolInput(props.tool.input)}</text>
        </Show>
        <Show when={props.tool.result}>
          <box marginTop={1}>
            <text fg={theme.textMuted}>
              {props.tool.result!.slice(0, 200)}
              {props.tool.result!.length > 200 ? "..." : ""}
            </text>
          </box>
        </Show>
      </box>
    </box>
  )
}
