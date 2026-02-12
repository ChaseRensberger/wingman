import { createMemo, For, Show, Switch, Match } from "solid-js"
import type { ScrollBoxRenderable } from "@opentui/core"
import { useTerminalDimensions } from "@opentui/solid"
import { theme } from "../../theme"
import { useSession } from "../../context/session"
import { Prompt } from "../../components/prompt"
import { UserMessage, AssistantMessage } from "../../components/message"
import { Header } from "./header"
import { Footer } from "./footer"
import { Sidebar } from "../../components/sidebar"

export function SessionView() {
  const session = useSession()
  const dimensions = useTerminalDimensions()
  const wide = createMemo(() => dimensions().width > 120)
  let scroll: ScrollBoxRenderable

  function toBottom() {
    setTimeout(() => {
      if (!scroll || scroll.isDestroyed) return
      scroll.scrollTo(scroll.scrollHeight)
    }, 50)
  }

  return (
    <box flexDirection="row">
      <box
        flexGrow={1}
        paddingBottom={1}
        paddingTop={1}
        paddingLeft={2}
        paddingRight={2}
        gap={1}
      >
        <Show when={!wide()}>
          <Header />
        </Show>

        <scrollbox
          ref={(r: ScrollBoxRenderable) => { scroll = r }}
          stickyScroll={true}
          stickyStart="bottom"
          flexGrow={1}
        >
          <Show
            when={session.messages().length > 0}
            fallback={
              <box justifyContent="center" alignItems="center" flexGrow={1} paddingTop={2}>
                <text fg={theme.textMuted}>
                  Send a message to get started...
                </text>
              </box>
            }
          >
            <For each={session.messages()}>
              {(message, index) => (
                <Switch>
                  <Match when={message.role === "user"}>
                    <UserMessage message={message} index={index()} />
                  </Match>
                  <Match when={message.role === "assistant"}>
                    <AssistantMessage message={message} />
                  </Match>
                </Switch>
              )}
            </For>
          </Show>
        </scrollbox>

        <Show when={session.error()}>
          <box
            border={["left"]}
            borderColor={theme.error}
            paddingTop={1}
            paddingBottom={1}
            paddingLeft={2}
            marginTop={1}
            backgroundColor={theme.backgroundPanel}
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
          >
            <text fg={theme.error}>{session.error()}</text>
          </box>
        </Show>

        <box flexShrink={0}>
          <Prompt onSubmit={toBottom} />
        </box>
        <Footer />
      </box>

      <Show when={wide()}>
        <Sidebar />
      </Show>
    </box>
  )
}
