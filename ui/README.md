# WingUI

WingUI is the shared React component library for Wingman projects. It contains reusable visual primitives, theme tokens, and a local showcase app for checking component states.

The package is intentionally separate from app-specific UI code. Apps such as the docs site, hero site, and bundled web UI can copy or adapt components when their runtime needs differ, but `ui/` is the canonical source for shared primitives.

## Stack

- React 19
- Vite
- Tailwind CSS v4
- Base UI and Headless UI primitives
- React Compiler via Babel

## Development

Install dependencies:

```bash
bun install
```

Run the showcase app:

```bash
bun run dev
```

Build the package/showcase:

```bash
bun run build
```

Run linting:

```bash
bun run lint
```

## Structure

```text
src/components/core/       Reusable component primitives
src/components/            Theme controls and app-level showcase helpers
src/showcases/             Component examples used by the showcase app
src/globals.css            Tailwind import and design tokens
src/lib/utils.ts           Shared className helpers
```

## Component Guidelines

- Keep primitives reusable and app-agnostic.
- Preserve accessibility behavior from Base UI/Headless UI where applicable.
- Prefer small component APIs over speculative configuration.
- Put visual examples in `src/showcases` instead of bloating primitives.
- Update the showcase when adding or changing a public component state.

## Drift Checks

Some apps may vendor copies of shared components or design tokens. From the repo root, use the drift checker when deciding whether changes should be backpropagated into WingUI:

```bash
bun scripts/check-ui-drift.mjs
```

Use `--diff` to inspect file-level differences:

```bash
bun scripts/check-ui-drift.mjs --diff
```
