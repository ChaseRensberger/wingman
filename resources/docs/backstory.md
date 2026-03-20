---
title: "Backstory"
group: "Extra"
draft: true
order: 10000
---

# Backstory

Wingman started as an attempt to separate agent orchestration from any single client experience.

The original inspiration came from working with [OpenCode](https://opencode.ai) and liking the split between an interactive client and an agentic backend. That general shape is familiar in web systems, but it felt especially useful for LLM workflows where you may want multiple frontends, multiple execution environments, and a runtime that can be reused outside one product.

Wingman takes that idea in a more opinionated direction. It focuses on explicit relationships between providers, agents, and sessions, then adds fleets and formations as first-class orchestration primitives. Writing it in Go also made it natural to lean into concurrency as a core part of the design.

The goal is not to compete on surface-area or to reproduce every feature from other agent systems. The goal is to provide a small, composable runtime that can be self-hosted, embedded, and adapted to different workflows.
