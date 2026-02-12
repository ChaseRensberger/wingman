import { theme } from "../theme"
import { Logo } from "../components/logo"
import { Prompt } from "../components/prompt"
import { useRoute } from "../context/route"
import { useSession } from "../context/session"

export function Home() {
  const route = useRoute()
  const session = useSession()

  return (
    <>
      <box
        flexGrow={1}
        justifyContent="center"
        alignItems="center"
        paddingLeft={2}
        paddingRight={2}
        gap={1}
      >
        <box height={3} />
        <Logo />
        <box width="100%" maxWidth={75} zIndex={1000} paddingTop={1}>
          <Prompt
            placeholder='Ask anything... "Fix a TODO in the codebase"'
            onSubmit={() => {
              const sid = session.sessionID()
              if (sid) {
                route.navigate({ type: "session", sessionID: sid })
              }
            }}
          />
        </box>
        <box height={3} width="100%" maxWidth={75} alignItems="center" paddingTop={2}>
          <text fg={theme.textMuted}>
            Enter to send · Esc to interrupt · Ctrl+C to exit
          </text>
        </box>
      </box>
      <box
        paddingTop={1}
        paddingBottom={1}
        paddingLeft={2}
        paddingRight={2}
        flexDirection="row"
        flexShrink={0}
        gap={2}
      >
        <text fg={theme.textMuted}>{process.cwd()}</text>
        <box flexGrow={1} />
        <text fg={theme.textMuted}>v0.1.0</text>
      </box>
    </>
  )
}
