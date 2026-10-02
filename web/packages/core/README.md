# WingUI

WingUI provides shared React components and themes for Wingman apps.
Import them from `@wingman/core`. Use the showcase app to view component states.

## Stack

- React 19
- Vite
- Tailwind CSS v4
- Base UI and Headless UI primitives
- React Compiler via Babel

## Development

Run these commands from `web/`. Install dependencies:

```bash
bun install
```

Run the showcase app:

```bash
bun --filter ui dev
```

Build the package and showcase:

```bash
bun run build:core
```

Run linting:

```bash
bun --filter ui lint
```

## Shared Imports

Import components and helpers:

```tsx
import { Button } from "@wingman/core/components/core/button";
import { cn } from "@wingman/core/lib/utils";
```
