---
title: "Providers"
description: "Connect provider accounts and route model requests."
---

# Providers

A provider is a service that runs models. Wingman includes Anthropic, DeepSeek, Google Gemini, OpenAI, OpenCode Zen, OpenCode Go, and OpenRouter.
Select a model with a reference such as `openai/gpt-5.6-terra`. See [Models](/configure/models).

## Save credentials

Use the Console Providers page, or save a key through the local managed service.
Set `ANTHROPIC_API_KEY` to your key before running:

```bash
wingman api setProviderAuth \
  -d "$(jq -n --arg key "$ANTHROPIC_API_KEY" '{providers: {anthropic: {type: "api_key", key: $key}}}')"
```

Wingman stores the key in its database. Clients do not need the provider key.
To read saved credential status, run `wingman api getProviderAuth`. The response does not return secrets or report environment credentials.
To remove it, run `wingman api deleteProviderAuth --param provider=anthropic`.

OpenCode Go uses provider ID `opencode-go`. Store its credentials separately from `opencode`, even though both read `OPENCODE_API_KEY`.

## OpenAI Codex subscription

OAuth connects an account without copying its password into Wingman.
The `openai` provider supports API keys and ChatGPT accounts with Codex access through OAuth.

In Console > Providers > OpenAI, select Connect in browser on the Wingman computer.
This requires the local callback port `1455` to be free.
For a remote server, select Connect headless and follow the displayed URL and code.

Only one login can be pending. It ends after five minutes or a server restart.
Connecting OAuth replaces the saved OpenAI API key. Saving an API key replaces OAuth.
OAuth uses the connected subscription. API keys use OpenAI Platform billing.

## Environment variables

A foreground server and the Go model library can read keys from the process environment:

| Provider            | Variable             |
| ------------------- | -------------------- |
| Anthropic           | `ANTHROPIC_API_KEY`  |
| OpenAI              | `OPENAI_API_KEY`     |
| OpenCode Zen and Go | `OPENCODE_API_KEY`   |
| Google              | `GEMINI_API_KEY`     |
| OpenRouter          | `OPENROUTER_API_KEY` |
| DeepSeek            | `DEEPSEEK_API_KEY`   |

`wingman service start` and `wingman service restart` import catalog provider keys from your shell into storage.
They do not replace saved credentials. Use `GET /provider/{name}` to read the provider's authentication source.

## Route a provider through a gateway

A gateway forwards model requests to a provider.
Set `provider.<id>.options.baseURL` in `wingman.json` to change a provider destination without changing model references.

This example uses the exe.dev gateway from inside an exe.dev box:

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

`auth: false` prevents Wingman from sending stored or environment credentials on this route.
If omitted or `true`, Wingman uses stored credentials first, then catalog environment variables.
Restart Wingman after route changes.

## Add a custom provider

Use a separate provider ID to keep direct and gateway routes available.
Define its model IDs, protocol, and destination in `wingman.json`:

```json
{
  "provider": {
    "local": {
      "options": { "baseURL": "http://127.0.0.1:8000/v1", "auth": false },
      "models": {
        "my-model": {
          "api": "openai_compatible_chat",
          "capabilities": { "tools": true }
        }
      }
    }
  }
}
```

Replace `my-model`, the URL, and capabilities with values supported by your model server.
After restarting Wingman, select `local/my-model`.
See [Configuration Schema](/reference/config-schema#provider) for route fields and model metadata.

## exe.dev gateway example

The [example configuration](https://github.com/chaserensberger/wingman/blob/main/wingman.example.json) routes OpenAI and Anthropic through exe.dev without provider keys.
The Anthropic gateway base URL is `http://169.254.169.254/gateway/llm/anthropic/v1`.
These addresses work inside exe.dev boxes. Keep normal references such as `openai/gpt-5.6-terra` and `anthropic/claude-sonnet-5`.
