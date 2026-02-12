import { createSignal, Show } from "solid-js"
import type { TextareaRenderable } from "@opentui/core"
import { theme } from "../theme"
import { EmptyBorder } from "./border"
import { useSession } from "../context/session"

export function Prompt(props: {
  onSubmit?: () => void
  placeholder?: string
  visible?: boolean
}) {
  let input: TextareaRenderable
  const session = useSession()
  const [value, setValue] = createSignal("")

  function submit() {
    const text = value().trim()
    if (!text || session.isStreaming()) return
    session.sendMessage(text)
    setValue("")
    if (input && !input.isDestroyed) input.clear()
    props.onSubmit?.()
  }

  return (
    <box visible={props.visible !== false}>
      <box
        border={["left"]}
        borderColor={theme.primary}
        customBorderChars={{
          ...EmptyBorder,
          vertical: "┃",
          bottomLeft: "╹",
        }}
      >
        <box
          paddingLeft={2}
          paddingRight={2}
          paddingTop={1}
          flexShrink={0}
          backgroundColor={theme.backgroundElement}
          flexGrow={1}
        >
          <textarea
            ref={(r: TextareaRenderable) => { input = r }}
            placeholder={props.placeholder || "Ask anything..."}
            textColor={theme.text}
            focusedTextColor={theme.text}
            minHeight={1}
            maxHeight={6}
            cursorColor={theme.text}
            focusedBackgroundColor={theme.backgroundElement}
            focused={!session.isStreaming()}
            onContentChange={() => {
              if (input && !input.isDestroyed) setValue(input.plainText)
            }}
            keyBindings={[
              { name: "return", action: "submit" as const },
              { name: "return", meta: true, action: "newline" as const },
            ]}
            onSubmit={submit}
          />
          <box flexDirection="row" flexShrink={0} paddingTop={1} gap={1}>
            <text fg={theme.primary}>Build </text>
            <text fg={theme.text}>claude-sonnet-4-5</text>
            <text fg={theme.textMuted}>anthropic</text>
          </box>
        </box>
      </box>
      <box
        height={1}
        border={["left"]}
        borderColor={theme.primary}
        customBorderChars={{
          ...EmptyBorder,
          vertical: "╹",
        }}
      >
        <box
          height={1}
          border={["bottom"]}
          borderColor={theme.backgroundElement}
          customBorderChars={{
            ...EmptyBorder,
            horizontal: "▀",
          }}
        />
      </box>
      <box flexDirection="row" justifyContent="space-between">
        <Show
          when={session.isStreaming()}
          fallback={<text />}
        >
          <box flexDirection="row" gap={1} flexGrow={1} justifyContent="space-between">
            <box flexDirection="row" gap={1}>
              <text fg={theme.textMuted}>[⋯]</text>
              <text fg={theme.text}>{session.status()}</text>
            </box>
            <text fg={theme.text}>
              esc <span style={{ fg: theme.textMuted }}>interrupt</span>
            </text>
          </box>
        </Show>
      </box>
    </box>
  )
}
