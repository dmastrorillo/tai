# North Star

## What this product is

A CLI that turns a git repo into the source of truth for a developer's AI tooling (skills, commands, agents, workflows, and standards), so subscribers stay in sync with a maintained upstream the same way they stay in sync with code.

## Who it's for

A developer who subscribes to a curated AI-tooling setup maintained by someone. The setup spans installable assets (skills, commands, agents) and the things that frame how those assets are used (workflows that compose them into named multi-step processes, standards that the AI loads on demand). "Someone" can be themselves on multiple machines, a teammate or eng-lead curating for a team, or a public figure curating for a fan base. Maintainer and subscriber are roles, not separate personas; one person frequently holds both. The product treats every subscriber the same regardless of how they relate to the upstream.

Two adoption shapes both fit this user:

- Evangelist adoption. A developer discovers TAI, builds a great source repo, asks teammates to install. Friction tolerance is low; voluntary uptake is the bar.
- Mandate adoption. A CTO or eng-lead decrees TAI as the team's distribution mechanism. Friction tolerance is high; coverage and consistency are the bar.

The pipeline often runs evangelist into mandate as a team commits to the workflow. Both states are first-class.

## The job to be done

"Keep my AI tooling aligned with a maintainer I trust: the skills and commands I have available, the agents that get invoked, the workflows I can run by name, and the standards the AI should consult while working. All from one git repo, with normal review and version control behind it, and tell me when there is something new to pull."

For a maintainer authoring the upstream: "Ship a change to any layer of my AI tooling (a new skill, a revised workflow, an updated standard) and have it reach every subscriber on the next sync, with a normal git review process behind it."

## What's broken today

Curated AI tooling spreads through Slack messages, screenshots, gists, and wiki pages. Three months later, half the subscribers have the stale version of a skill, one of them improved it locally and never told anyone, two new joiners never saw the original link. Multi-step processes that one engineer figured out once get reinvented every quarter because nobody remembered the sequence. Team standards (estimation, security, code review) sit in Notion where the AI cannot consult them automatically; subscribers end up pasting the same context into prompts over and over. There is no version-controlled loop, no diff, no review, no propagation. The closer AI tooling gets to the team's actual workflow, the more this hurts.

`git clone` already solves this for assets inside a project. It does not solve it for the global AI-tooling layer that lives in `~/.claude/` and similar locations, because there is no project context to clone into.

## The wedge

Git as the source of truth for a developer's AI tooling, with the AI tool's own on-disk layout as the wire format for everything that gets installed. The CLI copies skills, commands, and agents into the directories the AI tool already reads. No proprietary store, no abstraction layer over what the AI tool understands natively, no migration cost if the tool ever stops being used.

Workflows and standards live in the same repo but are read on demand: `tai workflow run <name>` emits a markdown plan the AI follows, `tai standards load <name>` returns the standard's body for the AI to consume. The maintainer's processes and conventions travel alongside the executable assets, all under the same version control loop.

The whole product is "your AI tooling in git, accessible the way your AI tool prefers to consume each piece of it."

## Explicitly NOT for

- LLM infrastructure. No gateways, prompt vaults, eval harnesses, observability, or anything that sits between the developer and their model provider.
- Per-project AI assets. Assets that live inside a project repo are already version-controlled and distributed by `git clone`. TAI exists specifically for the global slice that `git` does not naturally cover.
- A skills marketplace, registry, or discovery layer. Plugins resolve by explicit source or a small hard-coded first-party list. There is no central index, no browsing, no ratings. The same goes for source repos themselves: TAI does not search, browse, or curate them. You bring the URL.
- Outbound notification integrations. The update banner is a pull model. TAI does not push notifications to Slack, email, webhooks, or anywhere else.
- Real-time collaboration on AI work. No shared sessions, no presence, no chat. TAI is about distribution, not about working together inside the AI tool.
- Non-AI dotfile syncing. TAI is not chezmoi, yadm, or a generic config manager. It is scoped to skills, commands, agents, and the workflows and standards that reference them.

## Success

For the team / mandate shape: a maintainer pushes any kind of change to the source repo (a new skill, a revised workflow, an updated standard). Any subscriber who runs `tai sync` (manually or because the daily banner reminded them) has the change available the next time they open their AI tool, with no manual copy-paste.

For the evangelist / fan shape: a developer finds a public source repo they want to use, runs `tai config target add ...` plus `tai config set repo-url ...` plus `tai sync`, and the maintainer's assets are live in their AI tool within minutes, with no YAML editing or environment fiddling.

Success is the capability described above being routinely true, not a coverage claim TAI itself has no way to measure.

## Revision log

- 2026-05-21: initial version.
