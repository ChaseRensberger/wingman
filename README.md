<p align="center">
    <img src="./assets/Wingman.png" alt="Wingman Logo" width="200"/>
</p>

# Wingman

The open-source client-agnostic agent harness

> Wingman is not production ready. Expect frequent API and data model changes for the time being.

## What is Wingman?

Wingman is yet another agent harness, but this one is:

- Written in Go.
- Client agnostic: multiple clients/UIs on one machine can use Wingman as a shared dependency. Wingman is decoupled from any specific use case, so it does not come bundled with a coding TUI, but you can run a coding TUI on top of it.
- Independent from common harness dependencies: no Vercel AI SDK, no models.dev, etc. This makes it better suited for secure or airgapped environments.
- Highly extensible: plugin support via in-process Go modules or out-of-process JSON-RPC. Plugins can register tools, attach to lifecycle events, rewrite history, and more.

## Quick Start

Install the latest release:

```bash
curl -fsSL https://wingman.actor/install | bash
```

Install the latest snapshot build from `main`:

```bash
curl -fsSL https://wingman.actor/install | bash -s -- --snapshot
```

Restart your shell if the installer added `~/.wingman/bin` to your `PATH`, then verify the binary:

```bash
wingman version
```

Start Wingman in the foreground:

```bash
wingman serve
```

The server listens on `127.0.0.1:2323` by default and stores data in SQLite at `~/.local/share/wingman/wingman.db`.

Open the bundled web UI:

```text
http://127.0.0.1:2323/web
```

On Linux, install and start Wingman as a systemd service when you want it running in the background:

```bash
wingman up
```

`wingman up` prompts for `sudo` if needed, installs `wingman.service`, and starts it.

## Features

- **Client-agnostic runtime** - Run Wingman as the backend for any client that depends on LLM functionality.
- **Extendable** - Strong plugin support so you can extend session behavior however you want.
- **Provider-agnostic** - Wingman ships its own provider-agnostic model SDK, WingModels.
- **Context handoff** - Swap between provider/model combinations with minimal, and often zero, data loss.
- **SQLite-backed sessions** - Store agents, sessions, messages, parts, and provider auth in a local SQLite database.
- **HTTP API** - Communicate with Wingman via HTTP from your own clients.

**Want to learn more?** [Check out the site](https://wingman.actor) & [Read the docs](https://docs.wingman.actor)

Also I made [a hackernews client](https://news.wingman.actor).
