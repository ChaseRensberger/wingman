# Wingman Console

Bundled Wingman management UI. It is a Vite/React app served by `wingman serve` at `/console`.

## Development

Run the Vite dev server, then proxy `/console` from the Go server:

```sh
bun --filter wingman-console dev
wingman serve --ui-dev http://127.0.0.1:5173
```

Open the proxied app at `http://127.0.0.1:2323/console/`, or the Vite app directly at `http://127.0.0.1:5173/console/`.

## Build

Build the console app before building the Go binary so `web/apps/console/dist` exists for embedding:

```sh
bun run build:console
```
