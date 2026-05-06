---
title: "Introduction"
draft: false
order: 1
---

Wingman is an open-source client-agnostic agent harness. It is designed to be extremely portable (written in go and compiled to a binary with zero external dependencies) and performant (spawn fleets of agents as go routines).

I built Wingman because I wanted a performant agent harness that wasn't tied to any specific kind of application (like all the harnesses that are mostly just coding TUIs) while also being highly extendable and dependable. I also believe thoroughly in open source and building things for the people that build things.

Some non-negligible percentage of this project was built with the assistance of LLM coding agents (and eventually Wingman itself). That being said, this is not your typical *Show HN* slop post and every feature was built with intention (and I'd love feedback on all of them). At the time of writing this I have been working on this project for 6 months, and 3 months ago I interviewed at Y Combinator with this project. During the interview the first question I was asked was: "Why haven't you launched yet?" and the answer was because I just couldn't ship junk. I can also promise that every word in this documentation was written by hand.

I have taken inspiration from more projects than I can count but to name a few (that I reference daily):

- [Pi](https://pi.dev)
- [OpenCode](https://opencode.ai/) (also I stole their docs theme)
- [Shelley](https://github.com/boldsoftware/shelley)
- [Vercel's entire AI stack](https://open-agents.dev/)

and many many more...

## Next Steps

- **Want to start using Wingman?** Check out the [Quickstart](/quickstart)
- **Have an idea for how I can improve the project?** [Create an issue](https://github.com/chaserensberger/wingman/issues)
- **Want to contribute?** [Open a PR](https://github.com/chaserensberger/wingman/pulls)
- **Want to talk to me directly/join the community?** [Join the discord]()
