# Wingman

The open-source client-agnostic agent harness

> Wingman is not production ready. Expect frequent API and data model changes for the time being.

## What is Wingman?

Wingman is yet another agent harness, but this one is:

- Written in Go.
- Client agnostic: multiple clients/UIs on one machine can use Wingman as a shared dependency. Wingman is decoupled from any specific use case, so it does not come bundled with a coding TUI, but you can run a coding TUI on top of it.
- Independent from common harness dependencies: no Vercel AI SDK, no models.dev, etc. This makes it better suited for secure or airgapped environments.
- Highly extensible: plugin support via in-process Go modules or out-of-process JSON-RPC. Plugins can register tools, attach to lifecycle events, rewrite history, and more.

## Install

I'll provide better package manager support soon but for now you'll need to grab from the releases page or use the install script.

```bash
curl -fsSL https://wingman.actor/install | bash
```

```bash
sudo wingman up
```

`sudo wingman up` installs and starts `wingman.service` with systemd. The server listens on `127.0.0.1:2323` (by default) and stores data in SQLite at `~/.local/share/wingman/wingman.db` by default.

## Features

- **Client-agnostic runtime** - Run Wingman as the backend for any client that depends on LLM functionality.
- **Extendable** - Strong plugin support so you can extend session behavior however you want.
- **Provider-agnostic** - Wingman ships its own provider-agnostic model SDK, WingModels.
- **Context handoff** - Swap between provider/model combinations with minimal, and often zero, data loss.
- **Bring your own storage** - Wingman ships with a default SQLite adapter, but the storage provider is also agnostic.
- **HTTP API** - Communicate with Wingman via HTTP. Stdio and other protocols are coming later.

**Want to learn more?** [Check out the site](https://wingman.actor) & [Read the docs](https://docs.wingman.actor)

Also I made [a hackernews client](https://news.wingman.actor).
