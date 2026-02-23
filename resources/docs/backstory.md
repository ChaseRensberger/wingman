---
title: "Backstory"
group: "Extra"
order: 0
---
# Backstory

I got the idea for Wingman in May 2025 while playing with [OpenCode](https://opencode.ai). I liked OpenCode's approach to agents — specifically the client/server relationship between the TUI frontend and the agentic backend. If you come from the web application world, this idea is nothing to write home about. Still, I couldn't help but feel like composing agents over HTTP was interesting enough to pursue.

"So how is this different from OpenCode's server?" The short answer is fewer features. The slightly longer answer is that Wingman is opinionated about the relationships between providers, agents, and sessions — and introduces two additional primitives, fleets and formations — while being written in Go to take advantage of the language's built-in concurrency for running agents in parallel.

I also found OpenCode's server to be well-suited for their specific use case (powering a great agentic coding application) but not particularly flexible if you're trying to adapt it for something else. Wingman is meant to be that more general-purpose layer.
