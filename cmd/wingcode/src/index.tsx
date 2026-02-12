import { render } from "@opentui/solid"
import { onMount } from "solid-js"
import { App } from "./app"
import { RouteProvider } from "./context/route"
import { SessionProvider, useSession } from "./context/session"
import { api } from "./api"

const INSTRUCTIONS = `You are a helpful coding assistant called WingCode. You help users write, edit, and understand code.

Be concise and direct. When writing code:
- Use the write tool for new files
- Use the edit tool for modifying existing files
- Use the bash tool for running commands
- Use the read tool to examine files
- Use glob and grep to search the codebase

Always explain what you're doing briefly. Follow existing code conventions.`

const TOOLS = ["bash", "read", "write", "edit", "glob", "grep", "webfetch"]

async function main() {
  try {
    await api.health()
  } catch {
    console.error("Error: wingman server not running at localhost:2323")
    console.error("Start it with: wingman serve")
    process.exit(1)
  }

  const apiKey = process.env.ANTHROPIC_API_KEY
  if (apiKey) {
    try {
      await api.setProviderAuth("anthropic", apiKey)
    } catch {}
  }

  const agent = await api.createAgent({
    name: "Build",
    instructions: INSTRUCTIONS,
    tools: TOOLS,
    provider: {
      id: "anthropic",
      model: "claude-sonnet-4-5-20250514",
      max_tokens: 16384,
      temperature: null,
    },
  })

  const session = await api.createSession(process.cwd())

  render(
    () => (
      <RouteProvider>
        <SessionProvider>
          <Init agentID={agent.id} sessionID={session.id} />
        </SessionProvider>
      </RouteProvider>
    ),
    {
      targetFps: 60,
      exitOnCtrlC: false,
    },
  )
}

function Init(props: { agentID: string; sessionID: string }) {
  const session = useSession()

  onMount(() => {
    session.setAgentID(props.agentID)
    session.setSessionID(props.sessionID)
  })

  return <App />
}

main()
