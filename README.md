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
- Independent of external dependencies, making it ideal for running in secure or airgapped environments.
- Highly extensible: plugin support via in-process Go modules or out-of-process JSON-RPC. Plugins can register tools, attach to lifecycle events, rewrite history, and more.

## Install

`curl -fsSL https://wingman.actor/install | bash`

It adds Wingman to your shell `PATH` when it finds a suitable shell config. Pass `--no-modify-path` to skip that step.

## Enable

`wingman service start`

## Docs

[Introduction](https://docs.wingman.actor/)

[Quick Start](https://docs.wingman.actor/start-here/quickstart)

[![Star History Chart](https://api.star-history.com/svg?repos=ChaseRensberger/wingman&type=Date)](https://star-history.com/#ChaseRensberger/wingman&Date)
