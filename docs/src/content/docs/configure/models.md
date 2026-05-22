---
title: "Models"
description: "Choose models with model refs and custom routes."
---

# Models

Wingman selects models with provider-qualified model refs.

```text
provider/model
```

Examples:

```text
anthropic/claude-sonnet-4-6
openai/gpt-5.5
opencode/claude-sonnet-4-6
```

The provider part selects the provider catalog entry. The model part selects a model under that provider.

## Agent Default Model

Agents usually define a default `model_ref`:

```json
{
  "name": "Assistant",
  "instructions": "Be concise and helpful.",
  "tools": ["read", "glob", "grep"],
  "model_ref": "anthropic/claude-sonnet-4-6",
  "options": { "max_tokens": 4096 }
}
```

The model belongs to the agent definition, not the session. A session can use different agents or models on different turns.

## Per-Message Model Override

Message requests can override the agent's model for one turn:

```json
{
  "agent_id": "agt_...",
  "model_ref": "openai/gpt-5.5",
  "message": "Use this model for this turn."
}
```

If neither the message nor the agent provides a model, Wingman returns an error before the first provider call.

## Provider Routes And Model Refs

Provider route overlays change where cataloged model refs are sent. They do not change the model ref itself.

For example, this config routes `openai/*` refs through a gateway:

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

Agents still use normal refs:

```json
{
  "name": "Assistant",
  "model_ref": "openai/gpt-5.5"
}
```

Use provider route overlays when a known provider should go through a proxy, local gateway, or provider-compatible endpoint. See [Providers](/configure/providers) for auth and route details.

## Custom Model Routes

Use `model_route` when the catalog does not know the model or when a specific agent/request needs explicit route metadata.

```json
{
  "name": "custom-openai",
  "model_ref": "openai/gpt-4.1",
  "model_route": {
    "api": "openai_responses",
    "base_url": "https://api.openai.com/v1",
    "env": ["OPENAI_API_KEY"],
    "context_window": 1047576,
    "max_output": 32768,
    "capabilities": {
      "tools": true,
      "images": true,
      "structured_output": true
    }
  }
}
```

If `model_ref` is already in the catalog, the catalog route wins. `model_route` is the escape hatch for uncataloged models and explicit custom deployments.

## Choosing Between Provider Config And `model_route`

| Need | Use |
|---|---|
| Store a provider API key | [Provider auth](/configure/providers#store-provider-auth) |
| Route all `openai/*` refs through a gateway | [Provider route overlay](/configure/providers#route-a-provider-through-a-gateway) |
| Disable auth for an unauthenticated gateway | `provider.<id>.options.auth: false` |
| Use a cataloged model with a different runtime endpoint | Provider route overlay |
| Use a model not in the catalog | `model_route` |
| Target an endpoint that needs a different wire protocol | Not supported unless Wingman implements that protocol |

## Supported Protocols

Custom routes must use one of Wingman's supported protocols:

```text
openai_responses
openai_completions
anthropic_messages
```

The endpoint must speak the selected protocol. A route alone cannot make an unsupported API compatible.

## Catalog

Wingman's embedded catalog provides provider defaults, model metadata, and capability flags. It is intentionally small and only includes fields the runtime, API, or docs use.

Catalog details live in [WingModels](/concepts/wingmodels#catalog).

## Current Limits

Wingman does not currently provide:

- Generic provider discovery.
- First-class Ollama, Gemini, or Bedrock provider families.
- A config-file way to define entirely new providers.
- A root default model that automatically applies to all agents and messages.

Agents should set `model_ref`, or callers should pass `model_ref` on message requests.
