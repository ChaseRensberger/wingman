---
title: "Build a Coding TUI with Wingman"
description: "Use Wingman as the backend for a terminal-native coding agent UI."
group: "Editorial"
draft: false
order: 300
---

# Build a Coding TUI with Wingman

This is a tutorial on how to build a small terminal-native coding TUI on top of Wingman. It will be minimal and is meant to give some basic understanding on how to build a Wingman client.

By the end you will have a local TUI that can:

- Connect to a Wingman base url (`http://localhost:2323`).
- Create a Wingman client, register a coding agent, and start sessions.
- Stream assistant text and tool events into a terminal transcript.

## Prerequisites

*obviously you can use whatever tools/package managers/providers you like, i'm just gonna be explicit though with the instruction*

- Wingman, either install manually from the release or via the install script `curl -fsSL https://wingman.actor/install | bash`
- Bun
- `curl`
- `jq`
- An Anthropic API key

## 1. Start Wingman

Open one terminal for the Wingman server.

From a release install, run Wingman as a foreground process:

```bash
wingman serve
```

If you want Wingman managed by systemd instead:

```bash
wingman up
wingman status
```

Verify the server is reachable:

```bash
curl -sS http://localhost:2323/health
```

Expected response:

```json
{ "status": "ok" }
```

The default base URL is `http://localhost:2323`.

## 2. Configure Provider Auth


You can either store your api key in Wingman's local auth store:

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

or 

Set your Anthropic API key in the shell where you're running Wingman:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

Confirm that Wingman has a configured Anthropic credential:

```bash
curl -sS http://localhost:2323/provider/auth | jq
```

You should see `"configured": true` for `anthropic`.

## 3. Create A Client

Open a second terminal in the project directory you want the coding agent to work on. The session's `working_directory` will be this directory.

Register a client for this new application (generally this can be done by the client itself but for understanding we will do it manually):

```bash
CLIENT_ID=$(curl -sS -X POST http://localhost:2323/clients \
  -H "Content-Type: application/json" \
  -d '{"name":"wingcode"}' | jq -r .id)

printf 'client: %s\n' "$CLIENT_ID"
```

Client registration is currently just for attribution/organization. It lets Wingman list sessions created by this TUI separately from sessions created by other apps.

Create a coding agent (again can generally be done by the client itself):

```bash
AGENT_ID=$(curl -sS -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d '{
    "name": "Coding Agent",
    "instructions": "You are a concise coding agent. Inspect the codebase before editing. Make small, verifiable changes. Explain what you changed and what you ran.",
    "tools": ["read", "glob", "grep", "edit", "write", "bash"],
    "model_ref": "anthropic/claude-sonnet-4-6",
    "options": {"max_tokens": 4096}
  }' | jq -r .id)

printf 'agent: %s\n' "$AGENT_ID"
```

Create a session in the current directory:

```bash
SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d "$(jq -n \
    --arg title "$(basename "$PWD")" \
    --arg working_directory "$PWD" \
    '{title: $title, working_directory: $working_directory}')" | jq -r .id)

printf 'session: %s\n' "$SESSION_ID"
```

The working directory must already exist. Directory-scoped tools such as `read`, `grep`, `glob`, `edit`, `write`, and `bash` run relative to this directory.

Save the IDs for the TUI:

```bash
cat > .env.wingcode <<EOF
WINGMAN_BASE_URL=http://localhost:2323
WINGMAN_CLIENT_ID=$CLIENT_ID
WINGMAN_AGENT_ID=$AGENT_ID
WINGMAN_SESSION_ID=$SESSION_ID
EOF
```

## 4. Test Streaming with Curl

Before writing UI code, verify the backend loop works:

```bash
curl -N -X POST "http://localhost:2323/sessions/${SESSION_ID}/message/stream" \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d "{\"agent_id\":\"${AGENT_ID}\",\"message\":\"Summarize this project in one paragraph.\"}"
```

Wingman returns server-sent events:

```text
event: stream_part
data: {"type":"stream_part","version":1,"data":{...}}

event: done
data: {"type":"done","version":1,"data":{"usage":{...},"steps":2}}
```

The minimal TUI will handle these event types:

| Event | UI behavior |
|---|---|
| `stream_part` | Append assistant text to the in-progress message. |
| `tool_start` | Add a pending tool row. |
| `tool_end` | Mark the tool row complete and show a short result. |
| `message` | Ignore for now, or use later to reconcile persisted history. |
| `error` | Show an error block and unlock the prompt. |
| `done` | Mark the run complete and unlock the prompt. |

Use this endpoint for interrupts:

```bash
curl -sS -X POST "http://localhost:2323/sessions/${SESSION_ID}/abort" \
  -H "X-Wingman-Client: ${CLIENT_ID}" | jq
```

## 5. Create the TUI Project

Create a Solid OpenTUI app:

```bash
bun create tui --name wingcode
cd wingcode
bun install
```

Copy the Wingman environment file into the TUI project:

```bash
cp ../.env.wingcode .env
```

## 6. Add a Wingman HTTP Client

Create `src/wingman-client.ts`:

```ts
export type WingmanEvent = {
  type: string
  version: number
  data: unknown
}

export function createWingmanClient(input: { baseURL: string; clientID?: string }) {
  const baseURL = input.baseURL.replace(/\/$/, "")

  const jsonHeaders = () => ({
    "Content-Type": "application/json",
    ...(input.clientID ? { "X-Wingman-Client": input.clientID } : {}),
  })

  return {
    async abortSession(sessionID: string) {
      const response = await fetch(`${baseURL}/sessions/${sessionID}/abort`, {
        method: "POST",
        headers: jsonHeaders(),
      })
      if (!response.ok) throw new Error(await response.text())
    },

    async *streamMessage(input: { sessionID: string; agentID: string; message: string }) {
      const response = await fetch(`${baseURL}/sessions/${input.sessionID}/message/stream`, {
        method: "POST",
        headers: { ...jsonHeaders(), Accept: "text/event-stream" },
        body: JSON.stringify({ agent_id: input.agentID, message: input.message }),
      })

      if (!response.ok) throw new Error(await response.text())
      if (!response.body) throw new Error("Wingman returned an empty stream")

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
          const event = parseSSEFrame(frame)
          if (event) yield event
        }
      }
    },
  }
}

function parseSSEFrame(frame: string) {
  const data = frame
    .split("\n")
    .filter((line) => line.startsWith("data: "))
    .map((line) => line.slice("data: ".length))
    .join("\n")

  if (!data) return null
  return JSON.parse(data) as WingmanEvent
}
```

## 7. Add Runtime Config

Create `src/config.ts`:

```ts
export function getConfig() {
  const config = {
    baseURL: process.env.WINGMAN_BASE_URL ?? "http://localhost:2323",
    clientID: process.env.WINGMAN_CLIENT_ID,
    agentID: process.env.WINGMAN_AGENT_ID,
    sessionID: process.env.WINGMAN_SESSION_ID,
  }

  const missing = Object.entries(config)
    .filter(([key, value]) => key !== "clientID" && !value)
    .map(([key]) => key)

  if (missing.length > 0) {
    throw new Error(`Missing required environment values: ${missing.join(", ")}`)
  }

  return config as {
    baseURL: string
    clientID?: string
    agentID: string
    sessionID: string
  }
}
```

The client ID is optional in Wingman, but this tutorial uses one so the TUI's sessions are easy to find later.

## 8. Build the OpenTUI Screen

Replace the generated entrypoint with `src/index.tsx`:

```tsx
import { render, useKeyboard, useRenderer } from "@opentui/solid"
import type { TextareaRenderable } from "@opentui/core"
import { For, createSignal } from "solid-js"
import { getConfig } from "./config"
import { createWingmanClient, type WingmanEvent } from "./wingman-client"

type Row =
	| { kind: "user"; text: string }
	| { kind: "assistant"; text: string }
	| { kind: "tool"; text: string }
	| { kind: "error"; text: string }
	| { kind: "system"; text: string }

const config = getConfig()
const wingman = createWingmanClient({
	baseURL: config.baseURL,
	clientID: config.clientID,
})

function App() {
	const renderer = useRenderer()
	let promptInput: TextareaRenderable | undefined
	const [rows, setRows] = createSignal<Row[]>([
		{ kind: "system", text: `session ${config.sessionID}` },
		{ kind: "system", text: "ctrl+j submits, ctrl+c aborts, escape quits" },
	])
	const [prompt, setPrompt] = createSignal("")
	const [running, setRunning] = createSignal(false)

	useKeyboard((key) => {
		if (key.name === "escape") {
			renderer.destroy()
			return
		}

		if (key.ctrl && key.name === "c") {
			if (running()) void abortRun()
			else renderer.destroy()
			return
		}

		if (key.ctrl && key.name === "j") {
			void submit()
		}
	})

	async function abortRun() {
		try {
			await wingman.abortSession(config.sessionID)
			setRows((current) => [...current, { kind: "system", text: "abort requested" }])
		} catch (error) {
			setRows((current) => [...current, { kind: "error", text: String(error) }])
		}
	}

	async function submit() {
		const message = prompt().trim()
		if (!message || running()) return

		setPrompt("")
		promptInput?.setText("")
		setRunning(true)
		setRows((current) => [...current, { kind: "user", text: message }, { kind: "assistant", text: "" }])

		try {
			for await (const event of wingman.streamMessage({
				sessionID: config.sessionID,
				agentID: config.agentID,
				message,
			})) {
				applyEvent(event)
			}
		} catch (error) {
			setRows((current) => [...current, { kind: "error", text: String(error) }])
		} finally {
			setRunning(false)
		}
	}

	function applyEvent(event: WingmanEvent) {
		if (event.type === "stream_part") {
			const text = extractTextDelta(event.data)
			if (text) appendAssistantText(text)
			return
		}

		if (event.type === "tool_start") {
			setRows((current) => [...current, { kind: "tool", text: formatToolEvent("running", event.data) }])
			return
		}

		if (event.type === "tool_end") {
			setRows((current) => [...current, { kind: "tool", text: formatToolEvent("finished", event.data) }])
			return
		}

		if (event.type === "error") {
			setRows((current) => [...current, { kind: "error", text: formatEventData(event.data) }])
			return
		}

		if (event.type === "done") {
			setRows((current) => [...current, { kind: "system", text: "run complete" }])
		}
	}

	function appendAssistantText(text: string) {
		setRows((current) => {
			const next = [...current]
			const last = next[next.length - 1]
			if (last?.kind === "assistant") last.text += text
			else next.push({ kind: "assistant", text })
			return next
		})
	}

	return (
		<box flexDirection="column" height="100%">
			<box border={["bottom"]} paddingLeft={1} paddingRight={1} flexDirection="row">
				<text fg="#8bd5ff">Wingman TUI</text>
				<text>  </text>
				<text fg={running() ? "#f9c74f" : "#90be6d"}>{running() ? "running" : "idle"}</text>
			</box>

			<scrollbox flexGrow={1} padding={1} focused>
				<For each={rows()}>
					{(row) => (
						<box flexDirection="row">
							<text fg={colorFor(row.kind)}>{labelFor(row.kind)} </text>
							<text>{row.text}</text>
						</box>
					)}
				</For>
			</scrollbox>

			<box border={["top"]} padding={1} flexDirection="column">
				<text fg="#6c7086">Prompt</text>
				<textarea
					ref={promptInput}
					initialValue={prompt()}
					onContentChange={() => setPrompt(promptInput?.plainText ?? "")}
					height={4}
					focused={!running()}
					placeholder="Ask Wingman to inspect, edit, test, or explain this project..."
					wrapMode="word"
				/>
			</box>
		</box>
	)
}

function extractTextDelta(data: unknown) {
	if (typeof data === "string") return data
	if (typeof data !== "object" || data === null) return ""
	if ("text" in data && typeof data.text === "string") return data.text
	if ("delta" in data && typeof data.delta === "string") return data.delta
	if ("content" in data && typeof data.content === "string") return data.content
	return ""
}

function formatToolEvent(status: string, data: unknown) {
	if (typeof data !== "object" || data === null) return `tool ${status}`
	const name = "tool_name" in data && typeof data.tool_name === "string" ? data.tool_name : "tool"
	return `${name} ${status}`
}

function formatEventData(data: unknown) {
	if (typeof data === "string") return data
	if (typeof data !== "object" || data === null) return String(data)
	if ("error" in data && typeof data.error === "string") return data.error
	if ("message" in data && typeof data.message === "string") return data.message
	return JSON.stringify(data)
}

function colorFor(kind: Row["kind"]) {
	switch (kind) {
		case "user":
			return "#8bd5ff"
		case "assistant":
			return "#d6deeb"
		case "tool":
			return "#f9c74f"
		case "error":
			return "#ff6b6b"
		case "system":
			return "#6c7086"
	}
}

function labelFor(kind: Row["kind"]) {
	switch (kind) {
		case "user":
			return "you>"
		case "assistant":
			return "ai>"
		case "tool":
			return "tool>"
		case "error":
			return "error>"
		case "system":
			return "system>"
	}
}

render(() => <App />, { exitOnCtrlC: false })
```

This screen keeps all event translation in `applyEvent`, `extractTextDelta`, and `formatToolEvent`. That isolation matters because stream payloads can evolve without forcing changes throughout the render tree.

## 9. Run the TUI

Load the environment file and start the app:

```bash
set -a
source .env
set +a
bun run start
```

Try these prompts:

```text
Summarize this project in five bullets.
```

```text
Find the docs entrypoint and explain how docs pages are organized.
```

```text
Inspect the test setup and tell me the safest command to run before committing.
```

Use these keys:

| Key | Action |
|---|---|
| `ctrl+j` | Submit the prompt. |
| `ctrl+c` | Abort the current Wingman run. If idle, quit. |
| `escape` | Quit. |

## 10. Troubleshooting

If `curl http://localhost:2323/health` fails, Wingman is not running or is listening on a different host or port. Start it with `wingman serve` or `go run ./cmd/wingman serve`.

If the TUI says `Missing required environment values`, check that `.env` contains these values:

```bash
cat .env
```

You need:

```text
WINGMAN_BASE_URL=http://localhost:2323
WINGMAN_CLIENT_ID=cli_...
WINGMAN_AGENT_ID=agt_...
WINGMAN_SESSION_ID=ses_...
```

If Wingman returns an auth error, configure provider auth again:

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

If the model cannot call tools, confirm the agent was created with the coding tools:

```bash
curl -sS "http://localhost:2323/agents/${AGENT_ID}" \
  -H "X-Wingman-Client: ${CLIENT_ID}" | jq
```

If streaming returns `model_ref is required when agent has no model_ref`, recreate the agent with the `model_ref` field from step 3 and update `WINGMAN_AGENT_ID` in `.env.wingman-tui`.

If `ctrl+c` kills the process instead of aborting the run, confirm the render call uses `exitOnCtrlC: false`:

```tsx
render(() => <App />, { exitOnCtrlC: false })
```

## What to Add Next

The single-session loop is the core. Add features in this order:

1. List sessions filtered by `X-Wingman-Client` and resume the most recent one.
2. Add a session picker.
3. Add an agent/model selector.
4. Expand and collapse tool output.
5. Persist prompt history locally.
6. Add file references that expand into prompt context before sending.
7. Add a setup screen for provider auth.

Keep the boundary intact: Wingman owns agents, sessions, model routing, tools, and persistence. The terminal UI owns rendering, input, keybindings, and display-only state.
