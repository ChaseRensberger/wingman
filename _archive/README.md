# Archive

This directory holds source preserved for reference but **not part of the build**. Go automatically skips directories whose names begin with `_`, so nothing here is compiled, tested, vetted, or imported.

## Contents

### `actor/`
A channel-based actor-system implementation of fleets (collector + N agent actors, round-robin submission). Lived at `actor/` in the main tree. Only consumer was `examples/fleet/main.go` (also archived).

**Why archived:** parallel implementation with `fleet/` (goroutine pool) — the goroutine pool was what `internal/server/handlers_fleets.go` actually used. We're focusing on the core agent loop; orchestration primitives will be revisited later.

### `fleet/`
Goroutine-pool fleet implementation (`Fleet{MaxWorkers}` + `Run` / `RunStream`). Was used by `internal/server/handlers_fleets.go`.

**Why archived:** same as `actor/`. We'll redesign orchestration later when there's a real DAG / parallelism use case grounded in the new harness shape.

### `formations_runtime.go`, `handlers_formations.go`, `handlers_fleets.go`
Originally `internal/server/formations_runtime.go` (984 lines) — a YAML/JSON DAG executor with topo-sorted node execution, fleet fan-out, and edge-mapping expressions. Plus the chi handler files for the `/formations` and `/fleets` HTTP routes.

**Why archived:** the DAG executor has hard-coded `planner` node semantics (the `planner` node id triggers a `report.md` write requirement; certain nodes get `{section_id, status:done}` recovery prompts) that should be configurable validators on the formation definition rather than literals in the runtime. Rather than carry it forward through Tier 1-5 changes, it lives here until we choose to redesign it.

### `example-fleet/`
Originally `examples/fleet/main.go`. Used the actor-based fleet.

## Future
When/if we revive any of this, the rethink should:

- Pick **one** orchestration primitive (likely the goroutine pool — actor model overkill for worker pools)
- Generalize the `planner` / `section_id` semantics from `formations_runtime.go:380-410` and `:669` into per-node validators on the formation definition
- Build on the new `wingmodels` + `wingagent` packages instead of the old `core` / `provider` / `session` packages
