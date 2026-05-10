# Wingman Web

Bundled Wingman management UI. It is a Vite/React app served by `wingman serve` at `/web`.

## Development

Run the Vite dev server, then proxy `/web` from the Go server:

```sh
bun run dev
wingman serve --ui-dev http://127.0.0.1:5173
```

Open the proxied app at `http://127.0.0.1:2323/web/`, or the Vite app directly at `http://127.0.0.1:5173/web/`.

## Build

Build the web app before building the Go binary so `web/dist` exists for embedding:

```sh
bun run build
```
