---
title: "Providers"
description: "Configure provider auth, provider routes, and model gateways."
---

# Providers

Providers are model services that Wingman can call. Examples include Anthropic,
OpenAI, and OpenCode Zen.

Provider configuration has three parts:

| Concern | Where it lives | What it controls |
|---|---|---|
| Provider metadata | WingModels catalog and `~/.config/wingman/wingman.json` | Provider IDs, default base URLs, environment variable names, model capabilities, and supported protocols. |
| Provider credentials | SQLite auth store through `/provider/auth` | API keys used by the Wingman server. |
| Provider route and model config | `~/.config/wingman/wingman.json` | Runtime routing changes and custom provider/model definitions. |

Agents store `model_ref` values such as `openai/gpt-5.6-terra`. A provider route
overlay changes where that reference is sent without changing the agent.

OpenCode Go uses the `opencode-go` provider ID. It uses the same
`OPENCODE_API_KEY` environment variable as OpenCode Zen. Stored credentials use
the provider ID as a key. If you use the auth API, configure `opencode-go` separately.

## OpenAI Codex Subscription

The `openai` provider supports a metered OpenAI API key. It also supports
ChatGPT Plus/Pro through Codex OAuth. In the Console, open **Providers > OpenAI**.
On the machine that runs Wingman, select **Connect in browser**. If the daemon is
remote or headless, select **Connect headless**. Then open the displayed URL in
any browser. Enter the displayed code.

Browser OAuth requires Wingman to bind `localhost:1455`. Complete this flow in a
browser on the daemon host because the callback targets that localhost address.
Headless OAuth uses the device flow. It is appropriate for remote daemons. Only
one OpenAI OAuth attempt can be pending at a time. An attempt expires after five
minutes. Wingman stores authorization attempts in memory. If Wingman restarts,
the restart cancels an in-progress attempt. After the server returns, start a new attempt.

Codex OAuth routes supported `openai/*` model refs through the Codex backend.
For example, use `openai/gpt-5.6-terra`. Codex OAuth is separate from
`OPENAI_API_KEY`. An API key uses standard OpenAI Platform billing. OAuth uses
the limits of the connected ChatGPT subscription. To remove either credential,
disconnect OpenAI from the provider page.

Only one OpenAI credential is active per Wingman daemon. Starting a new OAuth
connection replaces a saved API key. Saving an API key replaces OAuth.

## Store Provider Auth

To store provider API keys, use `PUT /provider/auth`.

The commands use `WINGMAN_DAEMON_PASSWORD` with HTTP Basic authentication. See
[HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

The server stores credentials in SQLite. Clients do not need access to the shell
environment that supplied the key.

To view auth status, run:

```bash
curl -sS http://localhost:2323/provider/auth \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" | jq
```

The response reports stored SQLite credentials only. It does not return secrets.
It does not report credentials that Wingman can resolve from environment variables.

To remove a provider credential, run:

```bash
curl -sS -X DELETE http://localhost:2323/provider/auth/anthropic \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}"
```

## Environment Variables

When you use WingModels directly as a Go SDK, provider clients can read catalog
environment variables, including:

- `ANTHROPIC_API_KEY`
- `OPENAI_API_KEY`
- `OPENCODE_API_KEY`
- `GEMINI_API_KEY`
- `OPENROUTER_API_KEY`
- `DEEPSEEK_API_KEY`

When you use the Wingman server, use `/provider/auth`. The daemon then owns the
stored credential.

`/provider/auth` is not a complete view of effective authentication. If no stored
credential is available, a route with auth enabled can use environment variables
declared by its model metadata. To view the provider's effective auth source, use
`GET /provider/{id}`.

## Route A Provider Through A Gateway

To send a cataloged provider through a gateway or proxy, use
`provider.<id>.options.baseURL`.

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

With this configuration, agents keep normal catalog model refs:

```json
{
  "name": "Assistant",
  "instructions": "Be helpful and concise.",
  "model_ref": "openai/gpt-5.6-terra"
}
```

The route changes at runtime. The stored agent still uses `openai/gpt-5.6-terra`.

## Add A Custom Provider

If you want a separate provider ID and model list, use a configuration-defined
provider instead of rewriting an existing catalog provider.

This configuration keeps gateway references separate from direct provider
references. Agents can use `exe-openai/gpt-5.6-terra`. `openai/*` continues to
use OpenAI directly.

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

After you restart the server, the provider appears at `/provider`. Its models
appear at `/provider/exe-openai/models`. Agents can use:

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

Use `auth: false` only for unauthenticated gateways or local endpoints where
Wingman must not send a provider credential.

Routes can also override credential transport. `authHeader` sets the header name.
`authScheme` prefixes the credential value (for example, `Bearer`). `query` adds
static query parameters. These options apply to the route. They do not store credentials.

## exe.dev Gateway Example

exe.dev boxes expose provider-compatible LLM gateways at
`http://169.254.169.254/gateway/llm/{provider}`.

If you want direct provider refs and exe.dev gateway refs side by side, use custom
provider IDs:

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

After you restart Wingman, use these refs:

```text
exe-openai/gpt-5.6-terra
exe-anthropic/claude-sonnet-5
```

If you want all existing `openai/*` and `anthropic/*` refs to route through
exe.dev, overlay the built-in providers:

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

With the overlay approach, use normal model refs:

```text
openai/gpt-5.6-terra
anthropic/claude-sonnet-5
```
