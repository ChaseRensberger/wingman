import { createMemo, For, Show } from "solid-js"
import { theme } from "../theme"
import { useSession } from "../context/session"
import type { ToolCall } from "../types"

export function Sidebar() {
  const session = useSession()

  const allToolCalls = createMemo(() => {
    const calls: ToolCall[] = []
    for (const msg of session.messages()) {
      if (msg.toolCalls) calls.push(...msg.toolCalls)
    }
    return calls
  })

  const modifiedFiles = createMemo(() => {
    const files = new Set<string>()
    for (const call of allToolCalls()) {
      if (call.name === "write" || call.name === "edit") {
        try {
          const parsed = JSON.parse(call.input)
          const path = parsed.filePath || parsed.path
          if (path) files.add(path)
        } catch {}
      }
    }
    return Array.from(files)
  })

  const tokenDisplay = createMemo(() => {
    const t = session.totalTokens()
    if (t === 0) return "0"
    return t.toLocaleString()
  })

  return (
    <box
      backgroundColor={theme.backgroundPanel}
      width={42}
      height="100%"
      paddingTop={1}
      paddingBottom={1}
      paddingLeft={2}
      paddingRight={2}
    >
      <scrollbox flexGrow={1}>
        <box flexShrink={0} gap={1} paddingRight={1}>
          <box>
            <text fg={theme.text}>
              <b>Context</b>
            </text>
            <text fg={theme.textMuted}>{tokenDisplay()} tokens</text>
            <text fg={theme.textMuted}>{session.totalSteps()} steps</text>
          </box>

          <box>
            <text fg={theme.text}>
              <b>Tools</b>
            </text>
            <text fg={theme.textMuted}>
              {allToolCalls().length} call{allToolCalls().length !== 1 ? "s" : ""}
            </text>
          </box>

          <Show when={modifiedFiles().length > 0}>
            <box>
              <text fg={theme.text}>
                <b>Modified Files</b>
              </text>
              <For each={modifiedFiles()}>
                {(file) => (
                  <text fg={theme.textMuted} wrapMode="none">
                    {file}
                  </text>
                )}
              </For>
            </box>
          </Show>
        </box>
      </scrollbox>

      <box flexShrink={0} gap={1} paddingTop={1}>
        <text>
          <span style={{ fg: theme.success }}>•</span>{" "}
          <span style={{ fg: theme.primary, bold: true }}>Wing</span>
          <span style={{ bold: true }}>Code</span>{" "}
          <span style={{ fg: theme.textMuted }}>v0.1.0</span>
        </text>
      </box>
    </box>
  )
}
