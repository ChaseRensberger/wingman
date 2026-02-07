---
title: "Providers"
group: "Primitives"
order: 1
draft: true
---

# Providers

Providers are simply a company that makes models. Every model provider that wingman supports has a plaintext id for its name (e.g. "anthropic") which can be used to (for example) retrieve what models are supported by that provider. If you are using the Wingman SDK, you have a typed interface for using models:

```go
import (
    "wingman/provider/anthropic"
)

p := anthropic.New()
```

Under the hood this creates an internal anthropic client for you to make calls to the anthropic endpoint url. If you want to modify the default
