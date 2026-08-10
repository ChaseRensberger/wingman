---
title: "Providers"
description: "Configure provider auth, provider routes, and model gateways."
---

# Providers

Providers are model services that Wingman can call, such as Anthropic, OpenAI,
and OpenCode Zen.

Provider configuration has three separate pieces:

| Concern | Where it lives | What it controls |
|---|---|---|
| Provider metadata | WingModels catalog and `~/.config/wingman/wingman.json` | Provider IDs, default base URLs, environment variable names, model capabilities, and supported protocols. |
| Provider credentials | SQLite auth store through `/provider/auth` | API keys used by the Wingman server. |
| Provider route and model config | `~/.config/wingman/wingman.json` | Runtime routing changes and custom provider/model definitions. |

Agents store `model_ref` values such as `openai/gpt-5.6-terra`. A provider route
overlay can change where that reference is sent without changing the agent.

OpenCode Go uses the `opencode-go` provider ID and the same `OPENCODE_API_KEY` environment variable as OpenCode Zen. Stored credentials are keyed by provider ID, so configure `opencode-go` separately when using the auth API.

## OpenAI Codex Subscription

The `openai` provider supports a metered OpenAI API key or ChatGPT Plus/Pro
through Codex OAuth. In the Console, open **Providers > OpenAI** and choose
**Connect in browser** on the machine running Wingman. For a remote or headless
daemon, choose **Connect headless**, open the displayed URL in any browser, and
enter its code.

Browser OAuth requires Wingman to bind `localhost:1455`; complete that flow in
a browser on the daemon host because the callback targets that localhost
address. Headless OAuth uses the device flow instead, so it is appropriate for
remote daemons. Only one OpenAI OAuth attempt can be pending at a time, and an
attempt expires after five minutes. Authorization attempts are held in memory,
so a Wingman restart cancels an in-progress attempt; start a new one after the
server returns.

Codex OAuth routes supported `openai/*` model refs through the Codex backend;
for example, use `openai/gpt-5.6-terra`. It is separate from `OPENAI_API_KEY`:
an API key uses standard OpenAI Platform billing, while OAuth uses the limits of
the connected ChatGPT subscription. Disconnect OpenAI from the provider page to
remove either credential.

Only one OpenAI credential is active per Wingman daemon. Starting a new OAuth
connection replaces a saved API key, and saving an API key replaces OAuth.

## Store Provider Auth

Store provider API keys with `PUT /provider/auth`.

The commands use `WINGMAN_DAEMON_PASSWORD` with HTTP Basic authentication. See [HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

The server persists credentials in SQLite. Clients do not need access to the
shell environment that supplied the key.

Check auth status:

```bash
curl -sS http://localhost:2323/provider/auth \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" | jq
```

The response reports stored SQLite credentials only. It does not return secrets
or report credentials that Wingman can resolve from environment variables.

Remove a provider credential:

```bash
curl -sS -X DELETE http://localhost:2323/provider/auth/anthropic \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}"
```

## Environment Variables

When using WingModels directly as a Go SDK, provider clients can read environment variables from the catalog, including:

- `ANTHROPIC_API_KEY`
- `OPENAI_API_KEY`
- `OPENCODE_API_KEY`
- `GEMINI_API_KEY`
- `OPENROUTER_API_KEY`
- `DEEPSEEK_API_KEY`

When using the Wingman server, prefer `/provider/auth`. The daemon then owns the
stored credential.

`/provider/auth` is not a complete view of effective authentication: when no
stored credential is available, a route with auth enabled can still use the
environment variables declared by its model metadata. Use `GET /provider/{id}`
to inspect the provider's effective auth source.

## Route A Provider Through A Gateway

Use `provider.<id>.options.baseURL` to send a cataloged provider through a
gateway or proxy.

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
  "model_ref": "openai/gpt-5.6-terra"
}
```

The route changes at runtime. The persisted agent still says `openai/gpt-5.6-terra`.

## Add A Custom Provider

Use a config-defined provider when you want a separate provider ID and model list instead of rewriting an existing catalog provider.

This keeps gateway references separate from direct provider references. Agents
can use `exe-openai/gpt-5.6-terra`, while `openai/*` keeps using OpenAI directly.

```json
{
  "provider": {
    "exe-openai": {
      "name": "exe.dev OpenAI Gateway",
      "options": {
        "baseURL": "http://169.254.169.254/gateway/llm/openai/v1",
        "auth": false
      },
      "models": {
        "gpt-5.6-terra": {
          "api": "openai_responses",
          "context_window": 1050000,
          "max_output": 128000,
          "capabilities": {
            "tools": true,
            "images": true,
            "reasoning": true,
            "structured_output": true
          }
        }
      }
    }
  }
}
```

After restarting the server, the provider appears at `/provider`, its models
appear at `/provider/exe-openai/models`, and agents can use:

```text
exe-openai/gpt-5.6-terra
```

## Auth Behavior

`auth` controls whether Wingman sends credentials on a provider route.

| Config | Behavior |
|---|---|
| omitted | Use normal auth resolution: stored `/provider/auth` credentials first, then catalog environment variables. |
| `true` | Same as omitted. |
| `false` | Send no stored or environment credential for this provider route. |

Set `auth: false` only for unauthenticated gateways or local endpoints where Wingman should not send a provider credential.

Routes can also override credential transport: `authHeader` sets the header
name, `authScheme` prefixes the credential value (for example, `Bearer`), and
`query` adds static query parameters. These options apply to the route; they do
not store credentials.

## exe.dev Gateway Example

exe.dev boxes expose provider-compatible LLM gateways at `http://169.254.169.254/gateway/llm/{provider}`.

Use custom provider IDs when you want to keep direct provider refs and exe.dev gateway refs available side by side:

```json
{
  "provider": {
    "exe-openai": {
      "name": "exe.dev OpenAI Gateway",
      "options": {
        "baseURL": "http://169.254.169.254/gateway/llm/openai/v1",
        "auth": false
      },
      "models": {
        "gpt-5.6-terra": {
          "api": "openai_responses",
          "context_window": 1050000,
          "max_output": 128000,
          "capabilities": {
            "tools": true,
            "images": true,
            "reasoning": true,
            "structured_output": true
          }
        }
      }
    },
    "exe-anthropic": {
      "name": "exe.dev Anthropic Gateway",
      "options": {
        "baseURL": "http://169.254.169.254/gateway/llm/anthropic/v1",
        "auth": false
      },
      "models": {
        "claude-sonnet-5": {
          "api": "anthropic_messages",
          "context_window": 1000000,
          "max_output": 64000,
          "capabilities": {
            "tools": true,
            "images": true,
            "reasoning": true
          }
        }
      }
    }
  }
}
```

Use these refs after restarting Wingman:

```text
exe-openai/gpt-5.6-terra
exe-anthropic/claude-sonnet-5
```

If you want all existing `openai/*` and `anthropic/*` refs to route through exe.dev instead, overlay the built-in providers:

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

Use normal model refs with the overlay approach:

```text
openai/gpt-5.6-terra
anthropic/claude-sonnet-5
```
