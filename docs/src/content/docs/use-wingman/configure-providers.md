---
title: "Configure Providers"
description: "Store provider API keys for model calls."
---

# Configure Providers

Wingman stores model provider credentials in its local auth store.

Provider metadata and routes come from the WingModels catalog plus optional `wingman.jsonc` provider overlays. Only credentials are persisted in SQLite.

## Set Anthropic Auth

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

## Check Auth Status

```bash
curl -sS http://localhost:2323/provider/auth | jq
```

The response reports whether each provider is configured. It does not return the secret.

## Remove Provider Auth

```bash
curl -sS -X DELETE http://localhost:2323/provider/auth/anthropic
```

## Provider Environment Variables

When using WingModels directly as a Go SDK, provider clients can read environment variables such as `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, and `OPENCODE_API_KEY`.

For the Wingman server, prefer the auth API so clients do not need access to your shell environment.

## Route Through A Gateway

Use `provider.<id>.options.baseURL` when a cataloged provider should send requests to a gateway or proxy. This does not create a provider in SQLite; it changes runtime routing for model refs that use that provider ID.

Example for exe.dev boxes:

```jsonc
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

`auth: false` means Wingman will not send stored `/provider/auth` credentials or catalog environment credentials for that provider route.
