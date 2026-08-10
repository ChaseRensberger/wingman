# Wingman Console

Bundled Wingman management UI. It is a Vite/React app served by `wingman serve` at `/console`.

Ordinary API requests and SSE framing use `@wingman-actor/client`. Replay
cursors and reconnect policy stay local because they belong to the Console.
The connection banner reports daemon readiness and reloads the active route after
the daemon recovers. The Console asks for the daemon password and stores only a
signed HttpOnly session cookie; the password never enters browser storage.

## Development

Run the Vite dev server, then proxy `/console` from the Go server:

```sh
bun --filter @wingman/console dev
wingman serve --console-dev-url http://127.0.0.1:5173
```

Open the proxied app at `http://127.0.0.1:2323/console/`, or the Vite app directly at `http://127.0.0.1:5173/console/`.

The Vite proxy reads `registration.json` and `password` from the Wingman state
directory when Vite starts. Restart Vite after the daemon URL or password
changes.

## Build

Build the console app before building the Go binary so `web/apps/console/dist` exists for embedding:

```sh
bun run build:console
```
