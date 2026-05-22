---
title: "Providers"
description: "Configure provider auth, provider routes, and model gateways."
---

# Providers

Providers are the model services Wingman can call, such as Anthropic, OpenAI, or OpenCode.

Provider configuration has three separate pieces:

| Concern | Where it lives | What it controls |
|---|---|---|
| Provider metadata | WingModels catalog | Provider IDs, default base URLs, environment variable names, model capabilities, and supported protocols. |
| Provider credentials | SQLite auth store through `/provider/auth` | API keys used by the Wingman server. |
| Provider route overlays | `~/.config/wingman/wingman.json` | Runtime routing changes, such as sending `openai/*` refs through a gateway. |

Agents store `model_ref` values such as `openai/gpt-5.5`. Provider route overlays can change where that ref is sent without changing the agent.

## Store Provider Auth

Store provider API keys with `PUT /provider/auth`:

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

The server persists credentials in SQLite. Clients do not need access to your shell environment.

Check auth status:

```bash
curl -sS http://localhost:2323/provider/auth | jq
```

The response reports whether each provider is configured. It does not return secrets.

Remove a provider credential:

```bash
curl -sS -X DELETE http://localhost:2323/provider/auth/anthropic
```

## Environment Variables

When using WingModels directly as a Go SDK, provider clients can read environment variables from the catalog, including:

- `ANTHROPIC_API_KEY`
- `OPENAI_API_KEY`
- `OPENCODE_API_KEY`

When using the Wingman server, prefer `/provider/auth`. It makes credentials daemon-owned instead of client-owned.

## Route A Provider Through A Gateway

Use `provider.<id>.options.baseURL` when a cataloged provider should send requests to a gateway or proxy.

For example, this routes `openai/*` refs through the exe.dev LLM Gateway:

```json
{
  "provider": {
    "openai": {
      "options": {
        "baseURL": "http://169.254.169.254/gateway/llm/openai/v1",
        "auth": false
      }
    }
  }
}
```

With that config, agents keep normal catalog model refs:

```json
{
  "name": "Assistant",
  "instructions": "Be helpful and concise.",
  "model_ref": "openai/gpt-5.5"
}
```

The route changes at runtime. The persisted agent still says `openai/gpt-5.5`.

## Auth Behavior

`auth` controls whether Wingman sends credentials for a provider route.

| Config | Behavior |
|---|---|
| omitted | Use normal auth resolution: stored `/provider/auth` credentials first, then catalog environment variables. |
| `true` | Same as omitted. |
| `false` | Send no stored or environment credential for this provider route. |

Set `auth: false` only for unauthenticated gateways or local endpoints where Wingman should not send a provider credential.

## exe.dev Gateway Example

exe.dev boxes expose provider-compatible LLM gateways at `http://169.254.169.254/gateway/llm/{provider}`.

```json
{
  "provider": {
    "openai": {
      "options": {
        "baseURL": "http://169.254.169.254/gateway/llm/openai/v1",
        "auth": false
      }
    },
    "anthropic": {
      "options": {
        "baseURL": "http://169.254.169.254/gateway/llm/anthropic/v1",
        "auth": false
      }
    }
  }
}
```

Use normal model refs after configuring the gateway:

```text
openai/gpt-5.5
anthropic/claude-sonnet-4-6
```

## What Provider Config Does Not Do

Provider config does not:

- Store provider API keys in `wingman.json`.
- Create provider records in SQLite.
- Add new model protocols.
- Mutate persisted agents.
- Make an unsupported endpoint compatible with Wingman.

The endpoint still needs to speak one of Wingman's supported protocols. See [Models](/configure/models) for when to use `model_route` instead.

## Troubleshooting

If a provider call fails, check these in order:

1. Is the server using the config file you edited?
2. Does `curl -sS http://localhost:2323/provider/auth | jq` show the provider as configured, unless you intentionally set `auth: false`?
3. Does the agent or request use a cataloged `model_ref` such as `openai/gpt-5.5`?
4. If you set `baseURL`, does it include the provider's expected API prefix, such as `/v1`?
5. If you set `auth: false`, does the gateway actually accept unauthenticated requests?
6. If you use `model_route`, does the endpoint speak the selected protocol?

For exact config fields, see [Config Schema](/reference/config-schema#provider).
