<p align="center">
    <img src="./assets/Wingman.png" alt="Wingman Logo" width="200"/>
</p>

# Wingman

Wingman is an open-source service that runs AI agents. Web apps, command-line tools, and other clients can share one instance through its HTTP API.

It is written in Go. Plugins can add tools and change agent behavior.

## Install

`curl -fsSL https://wingman.actor/install | bash`

It adds Wingman to your shell `PATH` when it finds a suitable shell config. Pass `--no-modify-path` to skip that step.

## Enable

`wingman service start`

## Docs

[Introduction](https://docs.wingman.actor/)

[Quick Start](https://docs.wingman.actor/start-here/quickstart)
