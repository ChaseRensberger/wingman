import { Switch, Match } from "solid-js"
import { useTerminalDimensions, useKeyboard, useRenderer } from "@opentui/solid"
import { theme } from "./theme"
import { useRoute } from "./context/route"
import { useSession } from "./context/session"
import { Home } from "./routes/home"
import { SessionView } from "./routes/session"

export function App() {
  const route = useRoute()
  const session = useSession()
  const dimensions = useTerminalDimensions()
  const renderer = useRenderer()

  useKeyboard((key) => {
    if (key.ctrl && key.name === "c") {
      renderer.destroy()
    }
    if (key.name === "escape") {
      if (session.isStreaming()) {
        session.abort()
      }
    }
  })

  return (
    <box
      width={dimensions().width}
      height={dimensions().height}
      backgroundColor={theme.background}
    >
      <Switch>
        <Match when={route.data().type === "home"}>
          <Home />
        </Match>
        <Match when={route.data().type === "session"}>
          <SessionView />
        </Match>
      </Switch>
    </box>
  )
}
