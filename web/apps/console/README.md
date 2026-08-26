# Wingman Console

Bundled Wingman management UI. It is a Vite/React app served by `wingman serve` at `/console`.

Ordinary API requests and SSE framing use `@wingman-actor/client`. Replay
cursors and reconnect policy stay local because they belong to the Console.
The connection banner reports daemon readiness and reloads the active route after
the daemon recovers. A protected Console uses the browser's HTTP Basic Auth
prompt; it has no password form or session cookie.

## Development

Run the Vite dev server, then proxy `/console` from the Go server:

```sh
bun --filter @wingman/console dev
wingman serve --console-dev-url http://127.0.0.1:5173
```

Open the proxied app at `http://127.0.0.1:2424/console/`, or the Vite app directly at `http://127.0.0.1:5173/console/`.

The Vite proxy reads `registration.json` from the Wingman state directory and
`service.env` from the Wingman configuration directory when it starts. Restart
Vite after the daemon URL or service credentials change.

## Build

Build the console app before building the Go binary so `web/apps/console/dist` exists for embedding:

```sh
bun run build:console
```
