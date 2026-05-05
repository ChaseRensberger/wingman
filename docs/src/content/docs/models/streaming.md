---
title: "Streaming"
group: "WingModels"
draft: true
order: 104
---

# Streaming

Wingman has two layers of streaming. This page covers the lower layer — provider stream parts. For the loop lifecycle events and SSE wire format, see [Streaming](../agent/streaming).

## Provider stream parts

`StreamPart` is the wire-level event emitted by the provider during one assistant turn. It is a discriminated union; `Kind()` returns the discriminator string.

A turn proceeds:

```
stream-start (warnings)?
(text-* | reasoning-* | tool-input-* | tool-call)*
response-metadata?
error*
finish (usage, reason, message)
```

Tool calls follow a three-phase flow:

```
tool-input-start (id, name)
tool-input-delta (id, delta) ...
tool-input-end   (id)
tool-call        (id, name, parsed input)
```

The provider MUST emit exactly one `FinishPart` as the terminator. Errors mid-stream are surfaced as `ErrorPart` events; the `FinishPart` that follows carries `FinishReasonError` or `FinishReasonAborted`.

### Wingman additions over AI SDK v3

- `FinishPart` carries the assembled `*Message` so consumers can grab the final message without rebuilding it from deltas.
- `FinishReasonAborted` exists on top of the AI SDK enum for context-cancellation semantics.
- The assembled `*Message` is stamped with `FinishReason` and a `MessageOrigin` (`Provider`, `API`, `ModelID`). Providers set both before pushing the terminal `FinishPart`. `MessageOrigin.SameModel(other)` returns true only when both `API` and `ModelID` match; the `models/transform` package uses this to decide whether reasoning blocks survive into the next turn.

### Discriminator constants

`models` exports stable constants (`KindStreamStart`, `KindTextDelta`, `KindToolCall`, `KindFinish`, …) — see `models/event.go`. Wire payloads use the hyphenated form (`text-delta`, `tool-call`, …).