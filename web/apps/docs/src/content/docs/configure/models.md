---
title: "Models"
description: "Select models with model refs and custom routes."
---

# Models

Select a model with its provider and model ID:

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

## Model Variants

A variant is a named set of model options. Add `#variant` to select one:

```text
openai/gpt-5.6-terra#high
```

The catalog lists the valid variants for each route. Wingman returns an error
before the provider call when a selected variant is not available.

The catalog supports `none`, `low`, `medium`, `high`, `xhigh`,
and `max` for the built-in OpenAI GPT-5.6 routes. These names select OpenAI
reasoning-effort values. They do not specify a fixed token count or latency.

Request provider options and final HTTP values can override a variant. The full
model reference, including the variant, remains in the model-call record.

### Select a Variant in the Console

1. In the Session composer, open the model menu.
2. Select a model, then select its variant when it has variants.
3. Select Provider default to use the model without a named variant.

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

Change a provider's destination in `wingman.json` to use a gateway while keeping the same model references.
See [Providers](/configure/providers#route-a-provider-through-a-gateway) for configuration and authentication.

## Custom Model Routes

For shared custom models, [define a provider](/configure/providers#add-a-custom-provider) in `wingman.json`.
For a model outside the catalog on one agent or request, use `model_route`:

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

Catalog entries and configuration-defined models take precedence over `model_route`.

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

Read [WingModels](/concepts/wingmodels) for supported providers and protocols.
