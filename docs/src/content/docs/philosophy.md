---
title: "Philosophy"
group: "Overview"
draft: false
order: 1
---

# Philosophy

Wingman is opinionated about a few things. These are the principles that drive the day-to-day design decisions. If you're trying to figure out *why* the code is shaped the way it is, this page is the answer.

## As complicated as it needs to be, as simple as it can be

The inference loop in `agent/loop` is roughly the size of [pi-mono](https://github.com/anthropics/pi-mono) — well under a thousand lines. That's not an accident; it's a budget. Anytime a feature could be implemented either inside the loop or outside of it as a plugin, the default answer is "outside, as a plugin." Compaction lives outside. Storage lives outside. Anything you'd think of as a "capability" rather than "the loop itself" lives outside.

The flip side: where the loop *does* need to do something, it does it directly. There's no plugin runtime, no event bus, no pub/sub abstraction layered between the loop and its hooks. A hook is a function field on a struct. The loop calls it. If you can't read the entire control flow of `loop.Run` in one sitting, the design has failed.

## Hooks participate, sinks observe

This distinction shows up everywhere and is worth internalizing:

- **Hooks** are in the critical path. They run synchronously, can change loop behavior (rewrite messages, skip a tool call, supply initial history), and one fn-per-seam composes across plugins. An error in a hook fails the run.
- **Sinks** are out of the critical path. They receive events fan-out, can't influence the loop, and a slow or panicking sink doesn't break the loop.

The same plugin commonly contributes both. The storage plugin uses a `BeforeRun` hook to *load* prior history (a participating decision: it changes what the loop sees) and a sink to *save* new messages (an observing reaction: it doesn't change the loop). Mixing the two up — trying to make a sink rewrite history, or trying to use a hook for fire-and-forget logging — is a sign the design has drifted.

## Plugins are opt-in, always

A bare `session.New()` runs the loop with zero hooks, zero extra tools, and zero extra sinks. There are no "default plugins." If you want compaction, install the compaction plugin. If you want persistence, install the storage plugin. If you want both, install both.

This matters because every implicit default is a thing you have to learn about, debug around, or work to disable. By making the empty-handed case truly empty, the cost of `session.New()` is what it looks like — nothing more.

## One source of truth per concern

Wherever the codebase has to decide which copy of something is authoritative, the answer is unambiguous:

- The loop's `Config.Messages` and `BeforeRun` are mutually exclusive. Use one or the other; if both are set, `Run` returns a config error rather than guessing.
- After `loop.Run` returns, the session adopts the loop's terminal message slice wholesale. If a `BeforeStep` hook rewrote the slice mid-run, the session sees the rewrite — not its own pre-call snapshot.
- Plugin `Name()` must be unique within a session. Two plugins claiming the same name fails the run. There's no "later wins" or "first wins" — there's a clear error.

The pattern: when two systems could disagree, make the disagreement impossible to express. It's better to fail loudly at construction than to debug a silently desynchronized state later.

## The wire format isn't ours to invent

`StreamPart` matches Vercel AI SDK v3 `LanguageModelV3StreamPart` exactly. We don't have a "Wingman wire format." The AI SDK has spent a lot of time figuring out the right shape for streaming model output, and reusing it means anyone who knows that vocabulary already knows ours. Wingman adds two small things — `FinishPart` carries the assembled `*Message`, and `FinishReasonAborted` exists alongside the standard reasons — but the part-type vocabulary, the discriminator names, and the streaming semantics are theirs.

Similarly, "reasoning" not "thinking," because that's what the wire format calls it.

## KSUIDs over ULIDs

IDs are KSUIDs (27 base62 characters after a stable prefix) for three reasons: smaller wire size than ULIDs, time-resolution-sortable without monotonic-entropy state, and valid through year 2150. The prefix (`agt_`, `ses_`, `msg_`, `prt_`, `tlu_`) makes IDs self-describing in logs and URLs, and `store.ParseID` validates them at API boundaries so a session id passed where an agent id was expected fails fast with a clear message.

## Reference points

When in doubt, look at:

- **[pi-mono](https://github.com/anthropics/pi-mono)** for "how small can a useful agent loop be." This is the size budget.
- **[opencode](https://github.com/opencode-ai/opencode)** for hook composition patterns. The slicing approach to composable hooks is theirs; the BeforeRun / BeforeStep / TransformContext distinction is shaped by what worked there.
- **[Vercel AI SDK v3](https://sdk.vercel.ai/)** for streaming wire format. We follow their part vocabulary.

## What v0.1 does *not* try to do

- **External plugin loading.** Plugins in v0.1 are compile-time Go values. MCP-style external tools and Yaegi-script hooks are deferred to v0.2+. The constraint forces every plugin to be type-checked and reviewable as Go code, which is the right tradeoff while the API is still moving.
- **Built-in fleets, formations, or actor systems.** These existed in earlier prototypes and have been removed. Multi-agent orchestration is a separate problem from the agent loop, and conflating the two made both harder. If you need orchestration, build it on top.
- **Provider-side fallbacks or auto-retry.** Providers never throw mid-stream — they emit `ErrorPart` and continue. Retry policy is application-level concern, not loop-level.
- **Magic environment variable lookups during request handling.** The HTTP server reads provider credentials from its SQLite auth store. Set them with `PUT /provider/auth`. The SDK can do whatever it wants; the server stays explicit.

## When in doubt

Ask: does this make the loop bigger? If yes, can it be a plugin instead? Does this introduce a default? If yes, can the default be empty? Does this require two systems to agree? If yes, can they share one source of truth, or fail loudly when they disagree?

These are not absolute rules. They're the tiebreakers when a design decision is genuinely up for grabs.
