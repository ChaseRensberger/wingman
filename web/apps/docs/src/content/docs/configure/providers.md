---
title: "Providers"
description: "Configure provider authentication, routes, and model gateways."
---

# Providers

Providers are model services that Wingman calls. Examples include Anthropic,
OpenAI, and OpenCode Zen.

Store credentials through the Console or `/provider/auth`.
Set custom routes and models in `~/.config/wingman/wingman.json`.
Agents select a model with a reference such as `openai/gpt-5.6-terra`.

OpenCode Go uses the `opencode-go` provider ID and the `OPENCODE_API_KEY` environment variable.
Stored credentials use the provider ID as a key. Configure `opencode-go` separately when you use the authentication API.

## OpenAI Codex Subscription

The `openai` provider supports metered OpenAI API keys and ChatGPT Plus/Pro through Codex OAuth.
In the Console, open **Providers > OpenAI**. On the Wingman host, select **Connect in browser**.
If the daemon is remote or headless, select **Connect headless**. Then open the displayed URL in any browser.
Enter the displayed code.

Browser login requires a free `localhost:1455` port and a browser on the Wingman host.
Use headless login for a remote server.
Only one login attempt can be pending. It expires after five minutes or a server restart.

Codex OAuth uses the connected ChatGPT subscription. An API key uses OpenAI Platform billing.
Both use `openai/*` model references. Disconnect OpenAI from the provider page to remove the saved credential.

Only one OpenAI credential is active for each Wingman daemon. A new OAuth
connection replaces a saved API key. Saving an API key replaces OAuth.

## Store Provider Auth

These commands connect to the local managed service. Set `ANTHROPIC_API_KEY` to your key, then save it:

```bash
wingman api setProviderAuth \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

Wingman stores the key in SQLite. Clients do not need the provider key.

To view auth status, run:

```bash
wingman api getProviderAuth | jq
```

The response reports stored SQLite credentials only. It does not return secrets.
It does not report credentials that Wingman resolves from environment variables.

To remove a provider credential, run:

```bash
wingman api deleteProviderAuth --param provider=anthropic
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

`wingman service start` and `wingman service restart` import these keys from the command's environment.
They do not replace saved credentials, including OAuth connections.

For example:

```bash
export OPENAI_API_KEY="..."
wingman service start
```

Use `GET /provider/{id}` to see which authentication source the provider uses.

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

## Add A Custom Provider

Define a new provider ID to keep gateway and direct routes separate.
This example adds `exe-openai/gpt-5.6-terra` without changing `openai/*` routes:

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

| Config  | Behavior                                                                                                             |
| ------- | -------------------------------------------------------------------------------------------------------------------- |
| omitted | Use normal authentication resolution: stored `/provider/auth` credentials first, then catalog environment variables. |
| `true`  | Same as omitted.                                                                                                     |
| `false` | Send no stored or environment credential for this provider route.                                                    |

Set `auth: false` for routes that need no provider credential. This prevents Wingman from sending stored or environment keys to them.

Routes can override credential transport. `authHeader` sets the header name.
`authScheme` prefixes the credential value (for example, `Bearer`). `query` adds static query parameters.
These fields do not store credentials.

## exe.dev Gateway Example

exe.dev boxes expose provider-compatible LLM gateways at
`http://169.254.169.254/gateway/llm/{provider}`.

To keep direct routes available, use a [custom provider](#add-a-custom-provider).
To route existing OpenAI and Anthropic references through exe.dev, use:

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
