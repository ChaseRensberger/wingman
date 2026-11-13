---
title: "Executable plugin runtime"
description: "Runtime transforms, prompt and HTTP hooks, sign-in, and event subscriptions."
---

# Executable plugin runtime

The `plugin` Go SDK supports the following contracts through `plugin.Extension`.
Each callback runs with the invoking client's identity and the captured executable generation.

## Runtime transforms

`Extension.Transforms` holds ordered callbacks over owned maps. A callback can add, edit, or delete entries.

| Registration            | Draft                                                                         |
| ----------------------- | ----------------------------------------------------------------------------- |
| `Transforms.Agents`     | Agent definitions keyed by agent ID.                                          |
| `Transforms.Providers`  | Provider metadata, route options, and model inventories keyed by provider ID. |
| `Transforms.Models`     | Model inventories keyed by provider ID, then model ID.                        |
| `Transforms.MCP`        | MCP server configurations keyed by server name.                               |
| `Transforms.Skills`     | Skill definitions keyed by skill ID.                                          |
| `Transforms.References` | Additional directories keyed by reference name.                               |

Providers run before model transforms. Model transforms cannot add an undeclared provider.
Entry IDs must match their keys. References require absolute paths.
A removed provider or model cannot be restored through an explicit model route.

Each registration returns a `Registration`. Its `Dispose` method removes that callback from future snapshots and is safe to call more than once.
Existing sessions keep their captured definitions and connections.
Admitted runs save their transformed agent and skill definitions. Queued and resumed runs reuse those definitions after a plugin reload.
Changed MCP configurations get connections owned by the session or discovery request. Releasing that owner closes those connections.
MCP connect and disconnect operations manage the base configuration, not these captured connections.

Wingman replays transforms on fresh snapshots rather than caching callback results.
Callbacks must be repeatable and must not retain or concurrently mutate their drafts.
A callback can read another domain through `Host.Request`. Recursive evaluation of the same domain fails.
Invalid output rejects the snapshot without changing the stored definitions.

Agent, provider, model, catalog, tool, and MCP discovery accept `working_directory` to select a project's runtime definitions.
Project executable code runs only after the project is trusted.
Skill and reference transforms apply when a session is constructed.

## Prompt preparation

`Extension.OnPrompt` receives `protocol.Prompt` before a message enters the run queue.
It returns the replacement text. An error or empty replacement rejects the submission before persistence.
The hook also applies to ephemeral `POST /run` messages.

Admission compares retries against the original input and saves the prepared text.
Matching retries of an admitted request return the existing run without invoking the hook again.
Concurrent submissions with the same session and request ID serialize preparation.
A failed submission can invoke the hook again on retry. Hooks that change external state must account for this possibility.

## Native HTTP hooks

`Extension.HTTP.Request` receives the prepared native model request, including authentication headers.
`Extension.HTTP.Response` receives the native response before status handling and parsing.
Both callbacks can replace headers and the body. Request callbacks can also replace the method and URL.
Response callbacks can replace the status code.

Bodies are one-shot streams. Wingman transfers at most 32 KiB per body read and does not buffer the complete response.
If a request hook leaves the body unread and unchanged, Wingman preserves automatic POST replay for HTTP 307 and 308 redirects.
A partial read consumes those bytes. Returning the remaining source body preserves bytes already buffered by the SDK.
A replacement body must implement `io.ReadCloser`. Its `Close` method must interrupt a blocked read.
Plugins must set `ContentLength` to match a replacement or use `-1` for an unknown length.

Cancellation and body closure release replacement bodies and their source streams.
Hooks apply to Wingman's native HTTP model routes. An executable provider owns its own network requests and must intercept those requests itself.

## Sign-in methods

`Extension.Authentication` declares methods with an ID, integration ID, label, and kind.
Kinds are `key`, `oauth`, and `command`. The plugin implements each method's `Handle` callback.

`GET /rpc-plugins/auth` lists methods by plugin ID. It accepts `working_directory`.
`POST /rpc-plugins/auth` accepts these operations:

| Operation  | Input                                                                   |
| ---------- | ----------------------------------------------------------------------- |
| `start`    | `plugin`, `method`, optional `input`, and optional `working_directory`. |
| `status`   | `attempt` and optional `input`.                                         |
| `complete` | `attempt` and optional `input`.                                         |
| `cancel`   | `attempt`.                                                              |

The response contains the attempt ID, method, integration, status, and optional sign-in URL or instructions.
Only the owning client can access an attempt. An attempt retains the executable generation that started it.

The callback returns `pending`, `connected`, or `cancelled`.
`AuthResult.State` is private and returns to the plugin on later calls.
A `connected` result requires a credential. Wingman saves it under the plugin, client, and integration ID.
Clients receive neither the private state nor the credential. Plugin failures return a generic error and set the attempt status to `failed`.

Attempts expire ten minutes after creation. The server retains at most 128 attempt records.
Terminal attempts release their executable lease and private state immediately. Their public status remains available until expiry.
Cancellation interrupts an active callback. Expiry and server shutdown also cancel pending attempts.

## Public and custom events

`Host.SubscribeEvents` opens a client-scoped subscription, optionally filtered by exact event types.
`Subscription.Next` returns up to 32 events. An empty batch means the 20-second long poll timed out.
`Subscription.Close`, caller-context cancellation, and executable shutdown release the subscription.
A background subscription uses a context from `Host.Background`.

The public feed includes session events for the owning client.
`Host.PublishEvent(ctx, name, value)` adds custom events named `plugin.<pluginID>.<name>`.
Other plugins and API clients in the same client scope can subscribe to that name.

API clients use `POST /rpc-plugins/events`:

| Operation   | Input                   | Result                                       |
| ----------- | ----------------------- | -------------------------------------------- |
| `subscribe` | Optional `types` array. | Subscription `id`.                           |
| `next`      | Subscription `id`.      | An `events` array when events are available. |
| `close`     | Subscription `id`.      | Empty object.                                |

Each subscription holds at most 128 events. Event data is limited to 64 KiB, and the server allows at most 256 subscriptions.
Overflow fails the subscription. Consumers must subscribe again and refresh their state.
HTTP subscriptions expire after one minute without a subscription operation.
This feed is not a durable archive. Persisted session history remains available through the session APIs.
