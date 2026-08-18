---
title: "Providers"
description: "Configure provider authentication, routes, and model gateways."
---

# Providers

Providers are model services that Wingman calls. Examples include Anthropic,
OpenAI, and OpenCode Zen.

Provider configuration has three parts:

| Concern | Where it lives | What it controls |
|---|---|---|
| Provider metadata | WingModels catalog and `~/.config/wingman/wingman.json` | Provider IDs, default base URLs, environment variable names, model capabilities, and supported protocols. |
| Provider credentials | SQLite authentication store through `/provider/auth` | API keys that the Wingman server uses. |
| Provider route and model configuration | `~/.config/wingman/wingman.json` | Runtime route changes and custom provider or model definitions. |

Agents store `model_ref` values such as `openai/gpt-5.6-terra`. A provider route
overlay changes the destination without changing the agent.

OpenCode Go uses the `opencode-go` provider ID and the `OPENCODE_API_KEY` environment variable.
Stored credentials use the provider ID as a key. Configure `opencode-go` separately when you use the authentication API.

## OpenAI Codex Subscription

The `openai` provider supports metered OpenAI API keys and ChatGPT Plus/Pro through Codex OAuth.
In the Console, open **Providers > OpenAI**. On the Wingman host, select **Connect in browser**.
If the daemon is remote or headless, select **Connect headless**. Then open the displayed URL in any browser.
Enter the displayed code.

Browser OAuth requires Wingman to bind `localhost:1455`. Complete this flow in a browser on the daemon host.
The callback targets that localhost address. Headless OAuth uses the device flow for remote daemons.
Only one OpenAI OAuth attempt can be pending. An attempt expires after five minutes.
Wingman stores authorization attempts in memory. If Wingman restarts, it cancels the in-progress attempt.
After the server returns, start a new attempt.

Codex OAuth routes supported `openai/*` model refs through the Codex backend.
For example, use `openai/gpt-5.6-terra`. Codex OAuth is separate from `OPENAI_API_KEY`.
An API key uses standard OpenAI Platform billing. OAuth uses the connected ChatGPT subscription limits.
To remove either credential, disconnect OpenAI from the provider page.

Only one OpenAI credential is active for each Wingman daemon. A new OAuth
connection replaces a saved API key. Saving an API key replaces OAuth.

## Store Provider Auth

To store provider API keys, use `PUT /provider/auth`.

The commands use HTTP Basic authentication. Load managed-service credentials with `source ~/.config/wingman/service.env`.
The username defaults to `wingman`. See
[HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

The server stores credentials in SQLite. Clients do not need access to the shell environment that supplied the key.

To view auth status, run:

```bash
curl -sS http://localhost:2323/provider/auth \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" | jq
```

The response reports stored SQLite credentials only. It does not return secrets.
It does not report credentials that Wingman resolves from environment variables.

To remove a provider credential, run:

```bash
curl -sS -X DELETE http://localhost:2323/provider/auth/anthropic \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}"
```

## Environment Variables

WingModels and a foreground `wingman serve` process can read catalog environment
variables, including:

- `ANTHROPIC_API_KEY`
- `OPENAI_API_KEY`
- `OPENCODE_API_KEY`
- `GEMINI_API_KEY`
- `OPENROUTER_API_KEY`
- `DEEPSEEK_API_KEY`

When you run `wingman service start` or `wingman service restart`, Wingman
imports any of these variables from that command's environment into its stored
provider credentials. It imports a key only when the provider has no saved
credential, so it does not replace a key entered in the Console or an OAuth
connection.

For example:

```bash
export OPENAI_API_KEY="..."
wingman service start
```

Use `/provider/auth` with the Wingman server. The daemon then owns the stored credential.

`/provider/auth` does not return secret values. It shows credentials saved by
the service import, Console, or API.
To view the provider's effective authentication source, use
`GET /provider/{id}`.

## Route A Provider Through A Gateway

To send a cataloged provider through a gateway or proxy, use
`provider.<id>.options.baseURL`.

This configuration routes `openai/*` refs through the exe.dev LLM Gateway:

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

With this configuration, agents keep the normal catalog model refs:

```json
{
  "name": "Assistant",
  "instructions": "Be helpful and concise.",
  "model_ref": "openai/gpt-5.6-terra"
}
```

The route changes at run time. The stored agent still uses `openai/gpt-5.6-terra`.

## Add A Custom Provider

Use a configuration-defined provider for a separate provider ID and model list.
Do not rewrite an existing catalog provider.

This configuration separates gateway references from direct provider references.
Agents can use `exe-openai/gpt-5.6-terra`. `openai/*` continues to use OpenAI directly.

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

After you restart the server, the provider appears at `/provider`. Its models appear at `/provider/exe-openai/models`.
Agents can use:

```text
exe-openai/gpt-5.6-terra
```

## Auth Behavior

`auth` controls whether Wingman sends credentials on a provider route.

| Config | Behavior |
|---|---|
| omitted | Use normal authentication resolution: stored `/provider/auth` credentials first, then catalog environment variables. |
| `true` | Same as omitted. |
| `false` | Send no stored or environment credential for this provider route. |

Use `auth: false` only for unauthenticated gateways or local endpoints. Wingman must not send a provider credential to these endpoints.

Routes can override credential transport. `authHeader` sets the header name.
`authScheme` prefixes the credential value (for example, `Bearer`). `query` adds static query parameters.
These route values apply to the route. They do not store credentials.

## exe.dev Gateway Example

exe.dev boxes expose provider-compatible LLM gateways at
`http://169.254.169.254/gateway/llm/{provider}`.

Use custom provider IDs to keep direct provider refs and exe.dev gateway refs separate:

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

Overlay the built-in providers to route all existing `openai/*` and `anthropic/*` refs through exe.dev:

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

With the overlay approach, use the normal model refs:

```text
openai/gpt-5.6-terra
anthropic/claude-sonnet-5
```
