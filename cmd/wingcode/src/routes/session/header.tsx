import { createMemo, Show } from "solid-js"
import { theme } from "../../theme"
import { SplitBorder } from "../../components/border"
import { useSession } from "../../context/session"

export function Header() {
  const session = useSession()

  const title = createMemo(() => {
    const msgs = session.messages()
    const first = msgs.find(m => m.role === "user")
    if (!first) return "New Session"
    const text = first.content.trim()
    return text.length > 50 ? text.slice(0, 47) + "..." : text
  })

  const tokenDisplay = createMemo(() => {
    const t = session.totalTokens()
    if (t === 0) return undefined
    return t.toLocaleString()
  })

  return (
    <box flexShrink={0}>
      <box
        paddingTop={1}
        paddingBottom={1}
        paddingLeft={2}
        paddingRight={1}
        border={["left"]}
        borderColor={theme.border}
        customBorderChars={SplitBorder.customBorderChars}
        flexShrink={0}
        backgroundColor={theme.backgroundPanel}
      >
        <box flexDirection="row" justifyContent="space-between" gap={1}>
          <text fg={theme.text}>
            <b># {title()}</b>
          </text>
          <Show when={tokenDisplay()}>
            <text fg={theme.textMuted} wrapMode="none" flexShrink={0}>
              {tokenDisplay()}
            </text>
          </Show>
        </box>
      </box>
    </box>
  )
}
