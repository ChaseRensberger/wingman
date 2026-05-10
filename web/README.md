# Wingman Web

Bundled Wingman management UI. It is a Vite/React app served by `wingman serve` at `/ui`.

## Development

Run the Vite dev server, then proxy `/ui` from the Go server:

```sh
bun run dev
wingman serve --ui-dev http://127.0.0.1:5173
```

## Build

Build the web app before building the Go binary so `web/dist` exists for embedding:

```sh
bun run build
```
