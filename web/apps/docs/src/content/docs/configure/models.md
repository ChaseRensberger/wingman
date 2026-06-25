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

## Provider Routes and Model Refs

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

Use provider route overlays when a known provider should go through a proxy, local gateway, or provider-compatible endpoint. See [Providers](/docs/configure/providers) for auth and route details.

## Custom Model Routes

Use config-defined providers for daemon-wide custom providers and models. Use `model_route` only when a specific agent/request needs explicit route metadata that should travel with that agent or request.

For example, a custom provider in `~/.config/wingman/wingman.json` can add `exe-openai/gpt-5.5` to the normal provider and model APIs:

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
        "gpt-5.5": {
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

Agents can then use the custom ref directly:

```json
{
  "name": "Assistant",
  "model_ref": "exe-openai/gpt-5.5"
}
```

Use `model_route` for one-off uncataloged routes that should remain part of the agent/request payload:

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

If `model_ref` is already known through the embedded catalog or config-defined models, that metadata wins. Use `model_route` for one-off uncataloged models and explicit custom deployments.

## Choosing Between Provider Config and `model_route`

| Need | Use |
|---|---|
| Store a provider API key | [Provider auth](/docs/configure/providers#store-provider-auth) |
| Route all `openai/*` refs through a gateway | [Provider route overlay](/docs/configure/providers#route-a-provider-through-a-gateway) |
| Add a reusable custom provider/model visible to the web UI | [Custom provider config](/docs/configure/providers#add-a-custom-provider) |
| Disable auth for an unauthenticated gateway | `provider.<id>.options.auth: false` |
| Use a cataloged model with a different runtime endpoint | Provider route overlay |
| Use a model not in the catalog across agents | Config-defined provider model |
| Use a one-off model route for one agent/request | `model_route` |

## Supported Protocols

Custom routes must use one of Wingman's supported protocols:

```text
openai_responses
openai_completions
anthropic_messages
```

Choose the protocol that matches the endpoint.

## Catalog

Wingman's embedded catalog provides provider defaults, model metadata, and capability flags.

Catalog details live in [WingModels](/docs/concepts/wingmodels#catalog).
