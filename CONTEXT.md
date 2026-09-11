# CONTEXT

Glossary for TAI. Definitions only — no implementation details, no decisions. Both implementation and design rationale live in `openspec/` — the change proposal under `openspec/changes/<change>/` carries the reasoning, and the capability spec under `openspec/specs/<capability>/` carries the settled contract.

---

## TAI

The CLI itself. Originally meant "Triage AI" (the product's first incarnation as an AI-assisted PR triage tool). On the pivot to AI-as-code, the name is kept standalone — TAI is no longer an acronym. The Triage AI origin is preserved as backstory in the README.

## Source repo

The git repository that holds the AI assets a team (or individual) wants to share across developer machines. Identified by `repo-url` in TAI's local config. Has a fixed templated structure: `skills/`, `commands/`, `agents/`, `workflows/`, `standards/`, `plugins.yml`. Not all folders need to be populated. The source repo is the source of truth for the assets it contains; the developer's local copies in their target(s) are derivative.

A source repo is not company-specific — any team, group, or individual can author one. "Company-repo" is informal shorthand; the canonical term is **source repo**. The CLI surface for source-repo operations is namespaced under `tai repo ...` (e.g. `tai repo init`).

`repo-url` must be a remote git URL — SSH (`git@host:path`), `ssh://`, or `https://`. Local paths and `file://` URLs are rejected at config-set time; the clone/fetch/poll lifecycle assumes a real remote upstream.

## Target

A root directory on the developer's machine into which TAI copies assets (e.g. `~/.claude`, `~/.opencode`, or a custom path). One TAI installation may be configured with multiple targets — every sync writes to every configured target. TAI is agnostic about which AI tool a target belongs to; the only thing that matters is the layout of subdirectories inside the target, which is itself configurable.

## Asset

Generic term for the unit TAI **moves** from source repo to target: a skill, a command, or an agent. TAI treats assets as opaque files (or folders) sorted into category buckets at the source. It does not interpret their contents.

The three asset categories — **skill**, **command**, **agent** — are borrowed from the AI-tooling ecosystem (most notably Claude Code) and are not redefined here. TAI's job is to copy files into the directory the target expects; the meaning of each category is decided by the AI tool consuming them.

Workflows and standards (defined below) are **not** assets — they are source-repo content that stays in the clone and is read on demand, never copied to a target.

## Workflow

A YAML file under the source repo's `workflows/` directory describing an ordered series of skill and command invocations that compose into a larger task. Workflows live only in the clone; `tai workflow run <name>` reads one and emits markdown instructions that the AI follows. Workflows reference skills and commands; agents are not steps in a workflow.

## Standard

A markdown document under the source repo's `standards/` directory capturing a team-wide convention or guideline (e.g. estimation methodology, security baseline, code-review checklist). Standards may be nested. They are referenced by colon-namespaced logical name (`devops:security:best-practices`) — names are always lowercased, regardless of how the on-disk file or directory is cased. The on-disk file path is an authoring concern and is never exposed to consumers; two files whose lowercased names would collide are flagged with a warning.

Standards stay in the clone. The AI loads a standard on demand via `tai standards load <name>` when a skill, command, workflow, or human points at it. TAI does not nudge the AI to consult standards on its own — that pull is the team's responsibility, expressed through their own skills and commands.

## Plugin

A code-bearing extension to TAI. A plugin is a standalone executable plus an `assets/` directory, installed under `~/.local/share/tai/plugins/<name>/`. The directory name is the plugin's identity: it is also the plugin's top-level CLI verb (`tai <name> <args...>`), the namespace prefix for its skill and agent asset filenames (`tai-<name>-*`), the subdirectory name for its commands inside target command directories (`<commands>/tai-<name>/`), and the key under which TAI tracks its installed state.

Plugins are the opinionated layer of TAI: each plugin owns its own commands and may ship its own runtime state (e.g. a SQLite database).

## First-party plugin

A plugin whose canonical source is registered in `core/internal/plugins/registry.go::builtin`. Resolved by name alone — `tai plugins install <name>` works without a `--source` flag. Today the only first-party plugin is `triage`. First-party plugins are released from this repo via the `release-cycle` capability and trusted by default; no install-time warning is shown.

## Third-party plugin

A plugin not in the built-in registry. Installed by explicit source: `tai plugins install <name> --source <host>/<org>/<repo>`, or declared in a source repo's `plugins.yml` with the same explicit source. Treated as untrusted code by default — `tai` prompts for confirmation before installing one, and a `plugins.yml` carrying any third-party entry triggers an aggregate prompt on `tai sync`. Trust is persisted per source repo via a content-hash of `plugins.yml` in `<TAI_DATA_DIR>/state/trust.json`; the prompt re-fires when the file changes. "External plugin" is informal shorthand for the same thing.

## plugins.yml

A YAML file at the root of the source repo listing the plugins TAI should auto-install on `tai sync`. It is **additive, not authoritative** — a developer may install additional plugins beyond what plugins.yml declares via `tai plugins install <name>`, and those are not removed when the file changes. Removing a plugin from plugins.yml does not uninstall it from developer machines.

## Board

A browser surface the `triage` plugin serves on localhost so a developer can look at every pending review comment in a scope at once and make quick calls on the obvious ones. The board is a capture surface, not a writer: it reads comments and records the developer's intents, and it never changes a comment's status.

## Intent

One developer call on one comment (or one batch), captured on the board and not yet persisted. An intent is `accept`, `dismiss`, or `unanswered`. `unanswered` covers every comment the developer did not decide.

Any intent may carry a free-text note: a refinement to the suggested fix on an accept, the reasoning behind a dismiss, or on an `unanswered`, whatever the developer wants from the conversation about it ("explain this one to me", "I need to see the surrounding code first").

Intents are input to triage, not the outcome of it. Every intent still passes through the AI's triage loop, where a dismissal can be challenged, a note can be questioned, and a proposed fix can be sparred over.

**Intents live in a file; decisions live in the database.**

## Decision

The persisted outcome of triage for one comment: `accepted`, `dismissed`, or `completed`. A decision is written only by the `tai triage accept` / `tai triage dismiss` / `tai triage complete` verbs, and only after the comment has been through the triage conversation. An `intent` is not a decision — the word "decision" is reserved for state that has reached the database.

## Bulk pass

The act of working the board: reviewing the pending comments in a scope and recording intents on the easy ones, so the one-at-a-time triage conversation is left with the comments that genuinely need discussion. A bulk pass narrows the conversation; it does not replace it.
