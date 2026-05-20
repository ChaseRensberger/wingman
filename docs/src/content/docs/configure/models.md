---
title: "Models"
description: "Choose models with model refs and custom routes."
---

# Models

Wingman selects models with provider-qualified model refs:

```text
provider/model
```

Examples:

```text
anthropic/claude-sonnet-4-6
openai/gpt-5.5
opencode/claude-sonnet-4-6
```

Agents can define a default `model_ref`. Message requests can override that model per turn.

```json
{
  "agent_id": "agt_...",
  "model_ref": "anthropic/claude-sonnet-4-6",
  "message": "Use this model for this turn."
}
```

For custom or not-yet-cataloged models, provide `model_route` with the protocol, base URL, and capability metadata Wingman needs to make the request.

See [WingModels](/concepts/wingmodels) for the model SDK and catalog details.
