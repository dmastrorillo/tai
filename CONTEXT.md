# CONTEXT

Glossary for TAI. Definitions only — no implementation details, no decisions. Implementation lives in `openspec/`, design rationale in `docs/adr/`.

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

## plugins.yml

A YAML file at the root of the source repo listing the plugins TAI should auto-install on `tai sync`. It is **additive, not authoritative** — a developer may install additional plugins beyond what plugins.yml declares via `tai plugins <name> install`, and those are not removed when the file changes. Removing a plugin from plugins.yml does not uninstall it from developer machines.
