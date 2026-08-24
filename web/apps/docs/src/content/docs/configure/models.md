---
title: "Models"
description: "Select models with model refs and custom routes."
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

The provider selects a catalog entry. The model selects a model from that entry.

## Model Variants

A catalog model can provide named request variants. Add `#variant` to the model
reference:

```text
openai/gpt-5.6-terra#high
```

The catalog lists the valid variants for each route. Wingman returns an error
before the provider call when a selected variant is not available.

The initial variant catalog supports `none`, `low`, `medium`, `high`, `xhigh`,
and `max` for the built-in OpenAI GPT-5.6 routes. These names select OpenAI
reasoning-effort values. They do not specify a fixed token count or latency.

Request provider options and final HTTP values can override a variant. The full
model reference, including the variant, remains in the model-call record.

### Select a Variant in the Console

1. In the Session composer, open the model menu.
2. Select a model, then select its variant when it has variants.
3. Select **Provider default** to use the model without a named variant.

The Agent editor has separate model and Variant menus.

The Session composer remembers the last variant for each model. It ignores a
saved variant when the current catalog does not list that variant.

## Agent Default Model

Agents can have a default `model_ref`:

```json
{
  "name": "Assistant",
  "instructions": "Be concise and helpful.",
  "tools": ["read", "glob", "grep"],
  "model_ref": "anthropic/claude-sonnet-5",
  "options": { "max_tokens": 4096 }
}
```

The model belongs to the agent definition, not the session. A session can use a
different agent or model on each turn.

## Per-Message Model Override

Message requests can override the agent model for one turn:

```json
{
  "agent_id": "agt_...",
  "model_ref": "openai/gpt-5.6-terra",
  "message": "Use this model for this turn."
}
```

Wingman returns an error before the first provider call if neither the message nor the agent provides a model.

## Provider Routes and Model Refs

Provider route overlays change the destination for cataloged model refs. They do
not change the model ref.

For example, this configuration routes `openai/*` refs through a gateway:

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

Agents continue to use the normal refs:

```json
{
  "name": "Assistant",
  "model_ref": "openai/gpt-5.6-terra"
}
```

Use a provider route overlay for a known provider with a proxy, local gateway, or compatible endpoint.
See [Providers](/configure/providers) for authentication and route details.

## Custom Model Routes

Use configuration-defined providers for daemon-wide custom providers and models.
Use `model_route` when one agent or request needs route metadata. This metadata
travels with that agent or request.

For example, a custom provider in `~/.config/wingman/wingman.json` adds
`exe-openai/gpt-5.6-terra` to the normal provider and model APIs:

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

Agents can use the custom ref directly:

```json
{
  "name": "Assistant",
  "model_ref": "exe-openai/gpt-5.6-terra"
}
```

For an uncataloged route in an agent or request, use `model_route`:

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

Metadata from the embedded catalog or configuration-defined models takes precedence over `model_route`.
Use `model_route` for uncataloged models and custom deployments.

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

Wingman's embedded catalog provides provider defaults, model metadata, and
capability flags.

Catalog details live in [WingModels](/concepts/wingmodels#catalog).
