---
title: "Build a Coding TUI with Wingman"
description: "Use Wingman as the backend for a terminal-native coding agent UI."
group: "Editorial"
order: 300
---

# Build a Coding TUI with Wingman

Wingman is the agent harness, not the client. That split is the useful part: you can build a Claude Code or OpenCode shaped terminal UI without reimplementing model routing, agent sessions, persistence, tool execution, or streaming.

This article outlines a small coding-agent TUI with Wingman as the backend and a simplified OpenTUI frontend. It borrows the shape of OpenCode's TUI, but strips it down to the smallest client that is still useful.

## Target shape

The client has three jobs:

- Render the conversation in a terminal.
- Capture user prompts and keybindings.
- Translate UI actions into Wingman HTTP requests.

Wingman owns the rest:

- Provider auth and model calls.
- Agent definitions.
- Session history and working directory metadata.
- Tool execution for `read`, `grep`, `glob`, `edit`, `write`, `bash`, and any other allowed tools.
- Streaming run events over server-sent events.

The result is a local app with this boundary:

```text
OpenTUI client
  prompt, transcript, keybindings, display state
        |
        | HTTP + server-sent events
        v
Wingman server
  agents, sessions, tools, model routing, persistence
```

## Backend contract

Start Wingman first:

```bash
wingman serve
```

During development from the repository:

```bash
go run ./cmd/wingman serve
```

The default base URL is `http://localhost:2323`.

### Register a client

Client registration is optional, but a TUI should do it so its sessions can be listed separately from other apps using the same Wingman daemon.

```bash
CLIENT_ID=$(curl -sS -X POST http://localhost:2323/clients \
  -H "Content-Type: application/json" \
  -d '{"name":"wingman-tui"}' | jq -r .id)
```

Send that value on later requests:

```http
X-Wingman-Client: cli_...
```

This is attribution and organization, not auth.

### Create a coding agent

A coding TUI usually wants a broad tool set scoped to the user's current project:

```bash
AGENT_ID=$(curl -sS -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d '{
    "name": "Coding Agent",
    "instructions": "You are a concise coding agent. Inspect the codebase before editing. Make small, verifiable changes.",
    "tools": ["read", "glob", "grep", "edit", "write", "bash"],
    "provider": "anthropic",
    "model": "claude-haiku-4-5",
    "options": {"max_tokens": 4096}
  }' | jq -r .id)
```

Provider auth still has to be configured separately through `/provider/auth`. The TUI can expose a setup screen later; the first version can assume the user already ran the Quick Start auth step.

### Create a session

Sessions hold conversation history and the working directory used by directory-scoped tools.

```bash
SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d "{\"title\":\"$(basename "$PWD")\",\"working_directory\":\"$PWD\"}" | jq -r .id)
```

The working directory must exist. Tools like `read`, `grep`, `glob`, `edit`, `write`, and `bash` run relative to that directory.

### Stream a prompt

Use the streaming endpoint for an interactive UI:

```bash
curl -N -X POST "http://localhost:2323/sessions/${SESSION_ID}/message/stream" \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d "{\"agent_id\":\"${AGENT_ID}\",\"message\":\"Summarize this project.\"}"
```

Wingman returns server-sent events shaped like this:

```text
event: stream_part
data: {"type":"stream_part","version":1,"data":{...}}

event: done
data: {"type":"done","version":1,"data":{"usage":{...},"steps":2}}
```

The important event types for a minimal TUI are:

| Event | UI behavior |
|---|---|
| `stream_part` | Append assistant text or reasoning deltas to the in-progress message. |
| `tool_start` | Add a pending tool row. |
| `tool_end` | Mark the tool row complete and show the result summary. |
| `message` | Reconcile the final persisted message if present. |
| `error` | Show an error block and unlock the prompt. |
| `done` | Mark the run complete, store usage/step metadata, and unlock the prompt. |

Use `POST /sessions/{id}/abort` for `ctrl+c` or an explicit stop keybinding.

## Frontend shape

OpenCode's TUI is a full application: routes, dialogs, plugin slots, model selectors, permission prompts, session trees, prompt history, sidebars, and custom event plumbing. A first Wingman TUI does not need all of that.

Start with one screen:

- A header with the project name, current model, and session ID.
- A scrollable transcript.
- A footer with current run status.
- A focused multiline prompt.
- Keybindings for submit, abort, new session, and quit.

Solid is a good fit with OpenTUI because signals map cleanly to streaming updates.

```bash
bunx create-tui@latest -t solid wingman-tui
cd wingman-tui
bun install
```

Install whatever HTTP/SSE helpers you want, or keep it dependency-free and use `fetch` plus a small SSE parser.

## Minimal client module

This is intentionally small. It is a client-local wrapper around Wingman's HTTP API, not an official SDK.

```ts
type WingmanEvent = {
  type: string
  version: number
  data: unknown
}

export function createWingmanClient(input: { baseURL: string; clientID?: string }) {
  const headers = () => ({
    "Content-Type": "application/json",
    ...(input.clientID ? { "X-Wingman-Client": input.clientID } : {}),
  })

  return {
    async createSession(body: { title: string; working_directory: string }) {
      const response = await fetch(`${input.baseURL}/sessions`, {
        method: "POST",
        headers: headers(),
        body: JSON.stringify(body),
      })
      if (!response.ok) throw new Error(await response.text())
      return response.json() as Promise<{ id: string }>
    },

    async abortSession(sessionID: string) {
      await fetch(`${input.baseURL}/sessions/${sessionID}/abort`, {
        method: "POST",
        headers: headers(),
      })
    },

    async *streamMessage(body: { sessionID: string; agentID: string; message: string }) {
      const response = await fetch(`${input.baseURL}/sessions/${body.sessionID}/message/stream`, {
        method: "POST",
        headers: { ...headers(), Accept: "text/event-stream" },
        body: JSON.stringify({ agent_id: body.agentID, message: body.message }),
      })
      if (!response.ok) throw new Error(await response.text())
      if (!response.body) return

      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ""

      while (true) {
        const chunk = await reader.read()
        if (chunk.done) break
        buffer += decoder.decode(chunk.value, { stream: true })

        const frames = buffer.split("\n\n")
        buffer = frames.pop() ?? ""

        for (const frame of frames) {
          const data = frame
            .split("\n")
            .find((line) => line.startsWith("data: "))
            ?.slice("data: ".length)
          if (data) yield JSON.parse(data) as WingmanEvent
        }
      }
    },
  }
}
```

For production, add reconnect behavior only for passive event feeds. Do not automatically replay a prompt stream unless you know the original request did not start a run.

## Minimal OpenTUI screen

The UI state can be plain Solid signals:

```tsx
import { render, useKeyboard, useRenderer } from "@opentui/solid"
import { For, createSignal } from "solid-js"
import { createWingmanClient } from "./wingman-client"

type Row =
  | { kind: "user"; text: string }
  | { kind: "assistant"; text: string }
  | { kind: "tool"; text: string }
  | { kind: "error"; text: string }

const wingman = createWingmanClient({
  baseURL: "http://localhost:2323",
  clientID: process.env.WINGMAN_CLIENT_ID,
})

function App() {
  const renderer = useRenderer()
  const [rows, setRows] = createSignal<Row[]>([])
  const [prompt, setPrompt] = createSignal("")
  const [running, setRunning] = createSignal(false)
  const sessionID = process.env.WINGMAN_SESSION_ID ?? "ses_..."
  const agentID = process.env.WINGMAN_AGENT_ID ?? "agt_..."

  useKeyboard((key) => {
    if (key.name === "escape") renderer.destroy()
    if (key.ctrl && key.name === "c" && running()) void wingman.abortSession(sessionID)
  })

  async function submit() {
    const message = prompt().trim()
    if (!message || running()) return

    setPrompt("")
    setRunning(true)
    setRows((current) => [...current, { kind: "user", text: message }, { kind: "assistant", text: "" }])

    try {
      for await (const event of wingman.streamMessage({ sessionID, agentID, message })) {
        applyEvent(event)
      }
    } catch (error) {
      setRows((current) => [...current, { kind: "error", text: String(error) }])
    } finally {
      setRunning(false)
    }
  }

  function applyEvent(event: { type: string; data: unknown }) {
    if (event.type === "tool_start") {
      setRows((current) => [...current, { kind: "tool", text: "Running tool..." }])
      return
    }

    if (event.type === "tool_end") {
      setRows((current) => [...current, { kind: "tool", text: "Tool finished." }])
      return
    }

    if (event.type === "stream_part") {
      const text = extractTextDelta(event.data)
      if (!text) return
      setRows((current) => {
        const next = [...current]
        const last = next[next.length - 1]
        if (last?.kind === "assistant") last.text += text
        return next
      })
    }
  }

  return (
    <box flexDirection="column" height="100%">
      <box borderBottom paddingLeft={1} paddingRight={1}>
        <text>Wingman TUI {running() ? "running" : "idle"}</text>
      </box>

      <scrollbox flexGrow={1} padding={1} focused>
        <For each={rows()}>
          {(row) => (
            <text>
              <span fg={row.kind === "user" ? "#8bd5ff" : row.kind === "error" ? "#ff6b6b" : "#d6deeb"}>
                {row.kind}: {row.text}
              </span>
            </text>
          )}
        </For>
      </scrollbox>

      <box borderTop padding={1}>
        <textarea
          value={prompt()}
          onInput={setPrompt}
          height={4}
          focused={!running()}
          placeholder="Ask Wingman to inspect, edit, test, or explain this project..."
          onSubmit={submit}
        />
      </box>
    </box>
  )
}

function extractTextDelta(data: unknown) {
  if (typeof data !== "object" || data === null) return ""
  if ("text" in data && typeof data.text === "string") return data.text
  if ("delta" in data && typeof data.delta === "string") return data.delta
  return ""
}

render(() => <App />, { exitOnCtrlC: false })
```

This sketch leaves details open on purpose. The exact `stream_part` payload can evolve with Wingman's event model, so keep event translation isolated in a small function instead of scattering it through render components.

## State model

Keep two layers of state:

| State | Owner | Examples |
|---|---|---|
| Durable state | Wingman | sessions, messages, parts, working directory, client ID |
| Display state | TUI | focused pane, draft prompt, scroll position, collapsed tool output, selected session |

On startup, the TUI should:

1. Register or load its Wingman client ID.
2. List sessions with `X-Wingman-Client`.
3. Resume the most recent session or create a new one for the current directory.
4. Fetch the selected session to populate the transcript.
5. Submit future prompts through the streaming endpoint.

Do not store another copy of the transcript as the source of truth. Cache enough for rendering, but let Wingman be the durable record.

## Useful keybindings

Start with a small keymap:

| Key | Action |
|---|---|
| `enter` or `ctrl+j` | Submit prompt. |
| `ctrl+c` | Abort current run if running; otherwise quit or clear input. |
| `escape` | Quit. |
| `ctrl+n` | New session in the same working directory. |
| `ctrl+l` | Open session picker. |
| `ctrl+t` | Toggle tool output visibility. |

Avoid building a full command palette until the base loop feels good. A TUI lives or dies on the prompt, transcript, and interrupt path.

## What to copy from OpenCode

Copy the architectural instincts, not the whole app.

Good ideas to borrow:

- OpenTUI/Solid as the rendering layer.
- A provider/context boundary around the HTTP client.
- Batched event handling so streams do not rerender the whole app on every byte.
- A transcript view that treats tool calls as first-class rows, not hidden logs.
- A narrow prompt component with history, paste handling, and optional file references.

Things to skip in the first version:

- Plugin slots.
- Workspace orchestration.
- Theme galleries.
- Session trees and forks.
- Remote control endpoints.
- Complex permission dialogs unless your Wingman setup has a policy layer that needs them.

Wingman's value is that the client can stay thin. If your TUI starts implementing agent orchestration, tool dispatch, or transcript persistence itself, push that work back across the HTTP boundary.

## Next steps

Once the single-session loop works, add features in this order:

1. Session picker scoped by `X-Wingman-Client`.
2. Agent/model selector.
3. Tool output expansion and collapse.
4. Prompt history and draft persistence.
5. File references that expand into prompt context before sending.
6. A setup screen for provider auth.

That gets you a practical coding agent without compromising the core idea: Wingman is the backend harness, and the terminal UI is just one client among many.
