# Wingman v0 Rename Map

## Decisions Needed

1. `cmd/wingman/main.go:33` — CLI usage string is `"AI agent framework"`. The CLI binary is the *Wingman* umbrella, not the harness specifically. **Proposal:** keep as-is since "agent framework" describes the umbrella product, but flagging because it uses the word "agent" in a loose sense.

2. `docs/src/content/docs/index.md:10` — "I built it because I wanted an agent harness". This describes the umbrella motivation. **Proposal:** keep "agent harness" as the descriptive phrase for what Wingman provides (the spec lists `wingharness` as "The agent harness"), but flagging for confirmation.

3. `docs/src/content/docs/architecture.md:27` and `docs/src/content/docs/index.md:20` — "agentic loop". This describes the loop's function (it drives an Agent definition). **Proposal:** change to "inference loop" for consistency with `loop/loop.go` doc, but flagging because it could be read as referring to the Agent primitive.

4. `wingmodels/model.go:137` — comment says "Execution is the agent layer's responsibility (see wingagent.Tool)." The "agent layer" is descriptive of the harness layer. **Proposal:** change to "harness layer", but flagging because it reads like an architectural label.

5. `wingmodels/event.go:148` — "the agent shows partial args to the UI". **Proposal:** change to "the session shows partial args" since it refers to the runtime consumer. Flagging because "agent" here might mean the Agent definition.

---

## 1. Package Path Renames

| Old import path | New import path |
|---|---|
| `github.com/chaserensberger/wingman/wingagent/loop` | `github.com/chaserensberger/wingman/wingharness/loop` |
| `github.com/chaserensberger/wingman/wingagent/plugin` | `github.com/chaserensberger/wingman/wingharness/plugin` |
| `github.com/chaserensberger/wingman/wingagent/plugin/compaction` | `github.com/chaserensberger/wingman/wingharness/plugin/compaction` |
| `github.com/chaserensberger/wingman/wingagent/server` | `github.com/chaserensberger/wingman/wingharness/server` |
| `github.com/chaserensberger/wingman/wingagent/session` | `github.com/chaserensberger/wingman/wingharness/session` |
| `github.com/chaserensberger/wingman/wingagent/storage` | `github.com/chaserensberger/wingman/wingharness/storage` |
| `github.com/chaserensberger/wingman/wingagent/tool` | `github.com/chaserensberger/wingman/wingharness/tool` |
| `github.com/chaserensberger/wingman/wingagent/hook` | `github.com/chaserensberger/wingman/wingharness/hook` |

**Directory moves (required before import path fixes will compile):**
- `wingagent/` → `wingharness/`
- `docs/src/content/docs/wingagent/` → `docs/src/content/docs/wingharness/`

`go.mod` module path (`github.com/chaserensberger/wingman`) is **unchanged**.

---

## 2. Exported Identifier Renames

**None.** The public API types (`Agent`, `Session`, `Tool`, `Plugin`, `Store`, `Server`, `Loop`, `Registry`, etc.) all map to the five primitives where `Agent` is intentionally preserved. No exported type, function, method, constant, interface, or struct field needs renaming as part of this refactor.

> Rationale: `Agent` is one of the five primitives (a definition). `Session` is already named correctly. `Tool`, `Plugin`, and the future `Formation` concept do not appear as exported identifiers yet. The package rename from `wingagent` to `wingharness` is the only exported-surface change.

---

## 3. Internal Identifier Renames

**None.** There are no unexported identifiers (variables, functions, types, struct fields) that use the old package name or ambiguous "agent" terminology in a way that conflicts with the new vocabulary. All unexported names (`runner`, `abortRegistry`, `storagePlugin`, `built`, `reg`, etc.) are already generic.

---

## 4. Comments, Docstrings, Log Strings, Error Messages

### `wingagent/loop/loop.go` → `wingharness/loop/loop.go`
- `1:1` — `// Package loop is the wingagent inference loop.` → `// Package loop is the wingharness inference loop.`
- `1:29` — `//   - Persistence. The caller (typically wingagent/session) hooks into` → `//   - Persistence. The caller (typically wingharness/session) hooks into`
- `1:36` — `// usage tracking; the wingagent/hook package ships a default compaction` → `// usage tracking; the wingharness/hook package ships a default compaction`
- `1:122` — `// long-running coding agents.` → `// long-running coding sessions.` (refers to the running instance, not the definition)

### `wingagent/loop/run.go` → `wingharness/loop/run.go`
- `1:112` — `// Compaction is the canonical user of this seam (shipped in wingagent/hook).` → `// Compaction is the canonical user of this seam (shipped in wingharness/hook).`

### `wingagent/session/session.go` → `wingharness/session/session.go`
- `1:1` — `// Package session is a thin stateful wrapper over wingagent/loop.` → `// Package session is a thin stateful wrapper over wingharness/loop.`
- `1:15` — `// Plugins (wingagent/plugin) are opt-in: nothing is installed by` → `// Plugins (wingharness/plugin) are opt-in: nothing is installed by`
- `1:24` — `// transport. The caller (typically wingagent/server) wires those in by` → `// transport. The caller (typically wingharness/server) wires those in by`

### `wingagent/tool/tool.go` → `wingharness/tool/tool.go`
- `1:2` — `// wingagent loop and the built-in tool implementations.` → `// wingharness loop and the built-in tool implementations.`
- `1:26` — `// Tool is the executor contract every wingagent tool implements. The loop` → `// Tool is the executor contract every wingharness tool implements. The loop`

### `wingagent/plugin/plugin.go` → `wingharness/plugin/plugin.go`
- `1:1` — `// Package plugin defines the wingagent plugin model: a Plugin is a` → `// Package plugin defines the wingharness plugin model: a Plugin is a`

### `wingagent/plugin/compaction/compaction.go` → `wingharness/plugin/compaction/compaction.go`
- `1:1` — `// Package compaction is the canonical wingagent plugin: it summarizes` → `// Package compaction is the canonical wingharness plugin: it summarizes`

### `wingagent/hook/doc.go` → `wingharness/hook/doc.go`
- `1:1` — `// Package hook is a sibling to wingagent/plugin: it ships small,` → `// Package hook is a sibling to wingharness/plugin: it ships small,`
- `1:4` — `// wingagent/plugin/compaction, which is the canonical multi-seam` → `// wingharness/plugin/compaction, which is the canonical multi-seam`

### `wingagent/storage/sqlite.go` → `wingharness/storage/sqlite.go`
- `1:1` — `// Package storage owns wingagent's SQLite-backed persistence: agents,` → `// Package storage owns wingharness's SQLite-backed persistence: agents,`

### `wingagent/storage/migrations/0001_init.sql` → `wingharness/storage/migrations/0001_init.sql`
- `1:1` — `-- 0001_init.sql: initial schema for wingagent storage.` → `-- 0001_init.sql: initial schema for wingharness storage.`

### `wingmodels/model.go`
- `1:25` — `// Used by the agent loop to decide compaction. Providers` → `// Used by the session loop to decide compaction. Providers` (running instance, not definition)
- `1:40` — `// in wingagent. Empty if no tools are offered.` → `// in wingharness. Empty if no tools are offered.`
- `1:137` — `// Execution is the agent layer's responsibility (see wingagent.Tool).` → `// Execution is the harness layer's responsibility (see wingharness.Tool).`

### `wingmodels/event.go`
- `1:27` — `// since wingagent executes tools client-side; the part type is reserved for` → `// since wingharness executes tools client-side; the part type is reserved for`
- `1:148` — `// Providers stream tool arguments as JSON fragments; the agent shows partial` → `// Providers stream tool arguments as JSON fragments; the session shows partial` (runtime consumer)
- `1:185` — `// v0.1 since wingagent executes tools client-side; reserved so providers can` → `// v0.1 since wingharness executes tools client-side; reserved so providers can`

### `wingmodels/providers/registry.go`
- `1:5` — `// imports each provider package to trigger registration. The wingagent` → `// imports each provider package to trigger registration. The wingharness`
- `1:6` — `// session/server layers then look up providers by id and call New(opts) to` → `// session/server layers then look up providers by id and call New(opts) to` (already correct)

### `wingmodels/stream.go`
- `1:11` — `// (bb/pi-mono/packages/ai/src/utils/event-stream.ts), translated to Go using` → **KEEP** (external reference path)

---

## 5. Non-Go Touch Points

### `README.md` (repo root)
- `1:13` — `| **[WingAgent](wingagent)** | A portable agent runtime |` → `| **[WingHarness](wingharness)** | A portable agent harness |`
- `1:13` — Link target `wingagent` → `wingharness` (directory rename)
- **Count:** 2 occurrences (1 product name + 1 directory link)

### `docs/astro.config.mjs`
- `1:51` — `label: 'WingAgent',` → `label: 'WingHarness',`
- `1:53-59` — All slug paths under `wingagent/` → `wingharness/`:
  - `slug: 'wingagent/agents'` → `slug: 'wingharness/agents'`
  - `slug: 'wingagent/sessions'` → `slug: 'wingharness/sessions'`
  - `slug: 'wingagent/tools'` → `slug: 'wingharness/tools'`
  - `slug: 'wingagent/lifecycle'` → `slug: 'wingharness/lifecycle'`
  - `slug: 'wingagent/plugins'` → `slug: 'wingharness/plugins'`
  - `slug: 'wingagent/storage'` → `slug: 'wingharness/storage'`
  - `slug: 'wingagent/streaming'` → `slug: 'wingharness/streaming'`
- **Count:** 1 label + 7 slug paths

### `docs/src/content/docs/index.md`
- `1:15` — `**WingAgent** - An agent harness` → `**WingHarness** - The agent harness`
- `1:20` — `- \`\`wingagent\`\`` → `- \`\`wingharness\`\``
- `1:54` — Relative links `./wingagent/sessions`, `./wingagent/streaming`, `./wingagent/plugins` → `./wingharness/sessions`, etc.
- **Count:** ~5 occurrences

### `docs/src/content/docs/architecture.md`
- `1:17` — `- \`\`wingagent/\`\` is the agent layer.` → `- \`\`wingharness/\`\` is the agent layer.`
- `1:19` — `The HTTP server (\`wingagent/server\`)` → `The HTTP server (\`wingharness/server\`)`
- `1:24-29` — ASCII diagram package paths `wingagent/server`, `wingagent/storage`, `wingagent/session`, `wingagent/loop`, `wingagent/plugin`, `wingagent/tool` → `wingharness/...`
- `1:61` — `## The \`\`wingagent/loop\`\` package` → `## The \`\`wingharness/loop\`\` package`
- `1:71` — `See [Lifecycle hooks](./wingagent/lifecycle).` → `See [Lifecycle hooks](./wingharness/lifecycle).`
- `1:87` — `See [Sessions](./wingagent/sessions).` → `See [Sessions](./wingharness/sessions).`
- `1:95-96` — `wingagent/plugin/compaction` and `wingagent/storage` → `wingharness/plugin/compaction` and `wingharness/storage`
- `1:98` — `See [Plugins](./wingagent/plugins).` → `See [Plugins](./wingharness/plugins).`
- `1:102` — `wingagent/storage.Store` → `wingharness/storage.Store`
- `1:104` — `See [Storage](./wingagent/storage).` → `See [Storage](./wingharness/storage).`
- `1:108` — `wingagent/server` → `wingharness/server`
- **Count:** ~15 occurrences

### `docs/src/content/docs/sdk.md`
- `1:31-32` — Import paths `wingagent/session`, `wingagent/tool` → `wingharness/session`, `wingharness/tool`
- `1:155` — `See [Streaming](./wingagent/streaming).` → `See [Streaming](./wingharness/streaming).`
- `1:162` — `wingagent/plugin/compaction` → `wingharness/plugin/compaction`
- `1:170` — `See [Plugins](./wingagent/plugins)` → `See [Plugins](./wingharness/plugins)`
- `1:174` — `wingagent/tool` → `wingharness/tool`
- **Count:** ~7 occurrences

### `docs/src/content/docs/getting-started.md`
- `1:38-40` — Import paths `wingagent/plugin/compaction`, `wingagent/session`, `wingagent/tool` → `wingharness/...`
- `1:107` — `See [Streaming](./wingagent/streaming)` → `See [Streaming](./wingharness/streaming)`
- `1:151-152` — Relative links `wingagent/sessions`, `wingagent/plugins` → `wingharness/...`
- **Count:** ~6 occurrences

### `docs/src/content/docs/server.md`
- `1:60` — `./wingagent/storage#the-storage-plugin` → `./wingharness/storage#the-storage-plugin`
- `1:66` — `See [Storage](./wingagent/storage)` and `[Sessions](./wingagent/sessions)` → `wingharness/...`
- `1:72` — `The full envelope schema is in [Streaming](./wingagent/streaming).` → `wingharness/streaming`
- **Count:** 3 occurrences

### `docs/src/content/docs/api.md`
- `1:153` — `See [Streaming](./wingagent/streaming)` → `See [Streaming](./wingharness/streaming)`
- **Count:** 1 occurrence

### `docs/src/content/docs/philosophy.md`
- `1:14` — `The agent loop in \`wingagent/loop\`` → `The agent loop in \`wingharness/loop\``
  - **Note:** "agent loop" here is descriptive (the loop that runs agents). If Decision #3 resolves to "inference loop", also change `agent loop` → `inference loop`.
- **Count:** 1-2 occurrences

### `docs/src/content/docs/wingmodels/parts.md`
- `1:40` — Import path `wingagent/plugin/compaction` → `wingharness/plugin/compaction`
- **Count:** 1 occurrence

### `docs/src/content/docs/wingmodels/streaming.md`
- `1:10` — `see [Streaming](../wingagent/streaming)` → `see [Streaming](../wingharness/streaming)`
- **Count:** 1 occurrence

### `docs/src/content/docs/wingagent/agents.md` → `docs/src/content/docs/wingharness/agents.md`
- `1:101-102` — Import paths `wingagent/session`, `wingagent/tool` → `wingharness/...`
- **Count:** 2 occurrences

### `docs/src/content/docs/wingagent/sessions.md` → `docs/src/content/docs/wingharness/sessions.md`
- `1:12` — `` `wingagent/loop` `` → `` `wingharness/loop` ``
- `1:32-33` — Import paths `wingagent/session`, `wingagent/tool` → `wingharness/...`
- `1:76` — `See [Streaming](../wingagent/streaming).` → `See [Streaming](../wingharness/streaming).`
- **Count:** 3 occurrences

### `docs/src/content/docs/wingagent/tools.md` → `docs/src/content/docs/wingharness/tools.md`
- `1:14` — `` `wingagent/tool` `` → `` `wingharness/tool` ``
- `1:29` — `` `wingagent/tool` `` → `` `wingharness/tool` ``
- `1:33-34` — Import paths `wingagent/session`, `wingagent/tool` → `wingharness/...`
- **Count:** 3 occurrences

### `docs/src/content/docs/wingagent/plugins.md` → `docs/src/content/docs/wingharness/plugins.md`
- `1:16` — `` `wingagent/loop` `` → `` `wingharness/loop` ``
- `1:23` — Import path `wingagent/plugin/compaction` → `wingharness/plugin/compaction`
- `1:83-84` — Import paths `wingagent/loop`, `wingagent/plugin` → `wingharness/...`
- `1:125` — `wingagent/plugin/compaction` → `wingharness/plugin/compaction`
- `1:134` — Import path `wingagent/plugin/compaction` → `wingharness/plugin/compaction`
- `1:160` — `wingagent/storage` → `wingharness/storage`
- `1:164-165` — Import paths `wingagent/session`, `wingagent/storage` → `wingharness/...`
- **Count:** ~8 occurrences

### `docs/src/content/docs/wingagent/storage.md` → `docs/src/content/docs/wingharness/storage.md`
- `1:10` — `wingagent/storage.Store` → `wingharness/storage.Store`
- `1:52-53` — Import paths `wingagent/session`, `wingagent/storage` → `wingharness/...`
- `1:148` — `wingagent/storage/migrations/0001_init.sql` → `wingharness/storage/migrations/0001_init.sql`
- `1:161` — Import path `wingagent/storage` → `wingharness/storage`
- `1:230` — `wingagent/storage/migrations/0001_init.sql` → `wingharness/storage/migrations/0001_init.sql`
- `1:243` — Import path `wingagent/storage` → `wingharness/storage`
- **Count:** ~7 occurrences

### `docs/src/content/docs/wingagent/lifecycle.md` → `docs/src/content/docs/wingharness/lifecycle.md`
- No `wingagent` string occurrences; only conceptual terms (hooks, sinks, loop). No changes needed.
- **Count:** 0

### `docs/src/content/docs/wingagent/streaming.md` → `docs/src/content/docs/wingharness/streaming.md`
- No `wingagent` string occurrences.
- **Count:** 0

### `hero/src/routes/index.tsx`
- `1:111` — `WingAgent - A portable agent runtime` → `WingHarness - The agent harness`
- **Count:** 1 occurrence

---

## 6. Notes

### Files skipped and why
- `bb/` — Explicitly off-limits per instructions (vendored reference repos).
- `_archive/` — Explicitly off-limits per instructions.
- `wingbase/` — Explicitly off-limits per instructions; flagged for parent review.
- `node_modules/`, `dist/`, `build/`, `.git/` — Explicitly off-limits per instructions.
- `docs/dist/` and `docs/.astro/` — Generated build artifacts; will be rebuilt from source.
- `hero/dist/` — Generated build artifact; will be rebuilt from source.
- `utils/delete_all_agents.sh`, `utils/delete_all_sessions.sh`, `utils/delete_database.sh` — These operate on the `agents` and `sessions` database tables, which are primitive names (`Agent` is preserved). No rename needed.
- `examples/wingmodels/main.go` — No `wingagent` references; only uses `wingmodels`.
- `.goreleaser.yaml` — No `wingagent` references; only references `cmd/wingman`.
- `go.mod` — Module path does not contain `wingagent`; no change.

### Patterns noticed
1. **Every import path change is mechanical:** a global find-replace of `wingagent/` → `wingharness/` in `.go` files will fix all imports.
2. **Doc link rot risk:** The docs site has many relative markdown links (`./wingagent/...` and `../wingagent/...`). These must move in sync with the directory rename. A simple `sed -i 's/wingagent/wingharness/g'` on the `docs/src/content/docs/` tree is safe because the word `wingagent` never appears in docs in a context where it should be preserved.
3. **Migration comment only:** `0001_init.sql` has a single comment reference to `wingagent storage`. The SQL itself uses table names (`agents`, `sessions`, `messages`, `parts`, `auth`) that align with the primitives and do **not** need changing.
4. **No "sub-agent" language found in code:** The grep for `sub-agent`, `subagent`, `child agent`, `spawn agent`, `multi-agent` returned zero hits in Go files. The only occurrences are in `spec.md` (which is the specification document, not code).
5. **No workflow/orchestration language inside `wingagent/`:** The grep for `workflow`, `orchestration`, `orchestrate`, `multi-agent runtime` inside `wingagent/` returned zero hits in Go files.

### Suggested execution order
1. **Move directory:** `git mv wingagent wingharness`
2. **Move docs directory:** `git mv docs/src/content/docs/wingagent docs/src/content/docs/wingharness`
3. **Bulk replace import paths** in all `.go` files: `sed -i 's|wingagent/|wingharness/|g' **/*.go`
4. **Bulk replace doc links and product names** in all `.md` and `.mdx` files under `docs/src/content/docs/`: `sed -i 's|wingagent|wingharness|g' docs/src/content/docs/**/*.md`
5. **Update `docs/astro.config.mjs`** sidebar label and slugs.
6. **Update `README.md`** product name and link.
7. **Update `hero/src/routes/index.tsx`** product name.
8. **Update SQL migration comment** in `wingharness/storage/migrations/0001_init.sql`.
9. **Run `go build ./...` and `go vet ./...`** to verify import paths.
10. **Run `go test ./...`** to verify behavior.
11. **Rebuild docs** (`cd docs && bun build`) and hero (`cd hero && bun build`) to catch any broken links.
