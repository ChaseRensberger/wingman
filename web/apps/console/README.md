# Wingman Console

The Console is Wingman's browser interface. `wingman serve` serves it at `/console/`.
Enter the server's HTTP Basic Auth credentials in the browser prompt.

## Development

From `web/`, run the Vite server and Wingman in separate terminals:

```sh
bun --filter @wingman/console dev
wingman serve --console-dev-url http://127.0.0.1:5173
```

Open the proxied app at `http://127.0.0.1:2424/console/`, or the Vite app directly at `http://127.0.0.1:5173/console/`.

The Vite proxy reads `registration.json` from the Wingman state directory and
`service.env` from the Wingman configuration directory when it starts. Restart
Vite after the daemon URL or service credentials change.

## Build

From `web/`, build the Console before you build the Go binary with the `webdist` tag:

```sh
bun run build:console
```
