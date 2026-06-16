# WingUI

WingUI is the shared React component library for Wingman projects. It contains reusable visual primitives, theme tokens, and a local showcase app for checking component states.

The package is intentionally separate from app-specific UI code. Apps such as the docs site, hero site, and bundled console UI import shared primitives from `@wingman/core`.

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

Run Bun commands from `web/`; `packages/core/` is part of the nested Bun workspace.

Run the showcase app:

```bash
bun --filter ui dev
```

Build the package/showcase:

```bash
bun run build:core
```

Run linting:

```bash
bun --filter ui lint
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

## Shared Imports

Apps import shared primitives directly from the workspace package:

```tsx
import { Button } from "@wingman/core/components/core/button"
import { cn } from "@wingman/core/lib/utils"
```
