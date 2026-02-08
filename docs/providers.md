---
title: "Providers"
group: "Primitives"
order: 100
draft: true
---

# Providers

Providers are just an interface so that it's easy to translate between a model provider's specific typing and the typing Wingman uses. If you read the *Introduction*, this project was largely inspired by OpenCode's server. Instead of using Vercel's AI SDK, I've opted to define provider translation within Wingman. The con of this pattern (assuming it doesn't change) is that Wingman will likely never have the comprehensive support of the models you'll find on [models.dev](https://models.dev), the pro is that the core dependencies are pretty limited.

```go
import (
    "wingman/provider/anthropic"
)

p := anthropic.New()
```

Under the hood this creates an internal anthropic client for you to make calls to the anthropic endpoint url. If you want to modify the default
