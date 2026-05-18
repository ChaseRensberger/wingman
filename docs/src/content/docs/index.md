---
title: "Introduction"
draft: false
order: 1
---
# Introduction

Wingman is an open-source client-agnostic agent harness. It is designed to be extremely portable (written in go and compiled to a binary with zero external dependencies) and performant (spawn fleets of agents as go routines). A Wingman client can be any application that speaks the HTTP API: the built-in web UI, a CLI, an editor plugin, a Formation runner, or a third-party integration.

The mission of Wingman is to have:

- The fastest agent harness
- The most extendable agent harness
- The most portable agent harness

I built Wingman because I wanted a performant agent harness that wasn't tied to any specific kind of application (like all the harnesses that are mostly just coding TUIs).

Unsurprisingly, some non-negligible percentage of this project was built with assistance of Wingman itself. That being said, every feature, every loop, and every variable was designed with intention and I plan to maintain that level of quality as I iterate on the project and accept contributions from others.

I have taken inspiration from more projects than I can count but to name a few (that I reference daily):

- [Pi](https://pi.dev)
- [OpenCode](https://opencode.ai/)
- [Shelley](https://github.com/boldsoftware/shelley)
- [Vercel's entire AI stack](https://open-agents.dev/)

and many many more...

Also as an aside, I Wingman (as I've already mentioned) is written in golang and can thus has serveral features (like [WingModels](/core/wingmodels)) that can be imported and used as an SDK for your go application. This documentation is focused largely on the complete Wingman runtime but I suspect there will be people that want to use just the SDK without all the extra fluff. I will update the documentation in time to streamline that process.

## Next Steps

- **Want to start using Wingman?** Check out the [Quickstart](/quickstart)
- **Have an idea for how I can improve the project?** [Create an issue](https://github.com/chaserensberger/wingman/issues)
- **Want to contribute?** [Open a PR](https://github.com/chaserensberger/wingman/pulls)
