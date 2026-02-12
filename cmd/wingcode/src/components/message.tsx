import { createMemo, For, Show } from "solid-js"
import { theme } from "../theme"
import { SplitBorder } from "./border"
import { InlineToolCall, BlockToolCall } from "./tool-call"
import type { Message } from "../types"

const BLOCK_TOOLS = new Set(["write", "edit"])

export function UserMessage(props: { message: Message; index: number }) {
  return (
    <box
      border={["left"]}
      borderColor={theme.primary}
      customBorderChars={SplitBorder.customBorderChars}
      marginTop={props.index === 0 ? 0 : 1}
    >
      <box
        paddingTop={1}
        paddingBottom={1}
        paddingLeft={2}
        backgroundColor={theme.backgroundPanel}
        flexShrink={0}
      >
        <text fg={theme.text}>{props.message.content}</text>
      </box>
    </box>
  )
}

export function AssistantMessage(props: { message: Message }) {
  const toolCalls = createMemo(() => props.message.toolCalls || [])
  const hasContent = createMemo(() => props.message.content.trim().length > 0)

  return (
    <>
      <For each={toolCalls()}>
        {(tool) => (
          <Show
            when={BLOCK_TOOLS.has(tool.name)}
            fallback={<InlineToolCall tool={tool} />}
          >
            <BlockToolCall tool={tool} />
          </Show>
        )}
      </For>
      <Show when={hasContent()}>
        <box paddingLeft={3} marginTop={1} flexShrink={0}>
          <text fg={theme.text}>{props.message.content.trim()}</text>
        </box>
      </Show>
    </>
  )
}
