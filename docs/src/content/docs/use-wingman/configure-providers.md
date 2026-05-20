---
title: "Configure Providers"
description: "Store provider API keys for model calls."
---

# Configure Providers

Wingman stores model provider credentials in its local auth store. API keys do not belong in `wingman.jsonc`.

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
