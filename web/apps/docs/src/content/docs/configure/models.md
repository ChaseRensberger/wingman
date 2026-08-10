---
title: "Models"
description: "Choose models with model refs and custom routes."
---

# Models

Wingman selects models with provider-qualified references.

```text
provider/model
```

Examples:

```text
anthropic/claude-sonnet-5
openai/gpt-5.6-terra
google/gemini-3.6-flash
openrouter/moonshotai/kimi-k2.7-code
deepseek/deepseek-v4-pro
opencode/claude-sonnet-5
opencode-go/kimi-k3
```

The provider selects a catalog entry. The model selects a model in that entry.

## Agent Default Model

Agents can define a default `model_ref`:

```json
{
  "name": "Assistant",
  "instructions": "Be concise and helpful.",
  "tools": ["read", "glob", "grep"],
  "model_ref": "anthropic/claude-sonnet-5",
  "options": { "max_tokens": 4096 }
}
```

The model belongs to the agent definition, not the session. A session can use
different agents or models on different turns.

## Per-Message Model Override

Message requests can override the agent's model for one turn:

```json
{
  "agent_id": "agt_...",
  "model_ref": "openai/gpt-5.6-terra",
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
  "model_ref": "openai/gpt-5.6-terra"
}
```

Use a provider route overlay when a known provider should use a proxy, local
gateway, or compatible endpoint. See [Providers](/configure/providers) for auth
and route details.

## Custom Model Routes

Use configuration-defined providers for daemon-wide custom providers and
models. Use `model_route` only when one agent or request needs explicit route
metadata. This metadata travels with that agent or request.

For example, a custom provider in `~/.config/wingman/wingman.json` can add `exe-openai/gpt-5.6-terra` to the normal provider and model APIs:

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

Agents can then use the custom ref directly:

```json
{
  "name": "Assistant",
  "model_ref": "exe-openai/gpt-5.6-terra"
}
```

Use `model_route` for an uncataloged route that belongs in an agent or request:

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

If `model_ref` is known through the embedded catalog or config-defined models,
that metadata wins. Use `model_route` for uncataloged models and custom deployments.

## Supported Protocols

Custom routes must use one of Wingman's supported protocols:

```text
openai_responses
openai_completions
openai_compatible_chat
anthropic_messages
gemini_generate
```

## Catalog

Wingman's embedded catalog provides provider defaults, model metadata, and capability flags.

Catalog details live in [WingModels](/concepts/wingmodels#catalog).
