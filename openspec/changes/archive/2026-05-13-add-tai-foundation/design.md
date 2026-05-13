## Context

`tai` is a Go CLI shipped as a single static binary, used both directly by humans and indirectly by Claude (via bundled `/tai:<verb>` slash commands). It is the persistence and I/O layer for the PR-review-triage workflow currently implemented as a single Claude skill writing JSON sidecar files.

Every subsequent feature proposal — storage schema, install command, import command, triage state, triage command — will reference foundational conventions: where data lives, how the current repo is identified, what an error looks like, how bundled slash commands are paired with CLI verbs, and how those commands are versioned. Without those conventions written down once, each feature proposal would re-litigate them inconsistently.

This document captures the technical decisions behind those conventions and the alternatives considered. It is intentionally light on implementation detail — the corresponding specs in `specs/cli-framework/` and `specs/command-framework/` carry the normative requirements; this document explains the reasoning behind those requirements.

## Goals / Non-Goals

**Goals:**

- A single, documented home for tai's global state on disk, discoverable via standard OS conventions and overridable for testing.
- A deterministic, dependency-free repo-identity scheme that works inside any git clone and degrades gracefully (with a helpful error) outside one.
- A single error format that reads naturally to humans and to a Claude conversation, with stable codes so callers (human or AI) can recognise common conditions.
- A slash-command convention that lets future verbs slot in without bespoke install logic per command, and that detects user-modified command files reliably.

**Non-Goals:**

- Database schema, table layout, migrations — deferred to `add-storage-schema`.
- Install/uninstall command behaviour, prompt UX, override flags — deferred to `add-install-command`.
- Any actual feature behaviour (import, triage, list, show, accept, dismiss) — deferred to the four feature proposals.
- A JSON output mode. Output is human-readable prose for both audiences.
- Configuration files. tai is configured by flags and environment variables; there is no `tai.yaml`.
- Multi-tenant or shared-storage models. Each user's tai install is independent; data is per-user, not synced.
- Project bootstrap (Go module, directory layout, walking-skeleton `tai --version`). Bootstrap happens directly, outside the OpenSpec pipeline, before any spec is applied.

## Decisions

### D1. Data directory follows the XDG Base Directory Specification

The SQLite database and any future global state live under `$XDG_DATA_HOME/tai/`, defaulting to `~/.local/share/tai/` on Linux/macOS and `%LOCALAPPDATA%\tai\` on Windows. The directory is created lazily on first write.

A `TAI_DATA_DIR` environment variable overrides the default for the lifetime of the process. This exists primarily so the test suite can point the binary at a tmp directory; it is not advertised to end users.

**Alternatives considered:**

- `~/.tai/` — short and familiar (matches `.aws`, `.docker`), but pollutes `$HOME` and conflicts with the convention CLAUDE.md adopts in §"Testing layout" implicitly favouring XDG-style locations.
- `$XDG_CONFIG_HOME/tai/` — wrong category. The SQLite database is application *data*, not user-editable configuration.
- macOS-specific `~/Library/Application Support/tai/` — closer to native conventions on macOS but breaks symmetry with Linux. Most modern Go CLIs (gh, hub, fly) use XDG paths cross-platform.

### D2. Repo identity is the normalised `owner/name` parsed from the `origin` remote URL

When a command needs repo context, tai reads `git config --get remote.origin.url` (via the standard library's `os/exec`, not a git library) and parses it into `owner/name`. Both SSH (`git@github.com:acme/app.git`) and HTTPS (`https://github.com/acme/app.git`) forms normalise to `acme/app`. The trailing `.git` is stripped.

If the working directory is not a git repo, has no `origin` remote, or the URL cannot be parsed, the command errors with `REPO_NOT_FOUND`. A global `--repo <owner/name>` flag overrides detection on every command.

**Alternatives considered:**

- Use the repo's filesystem root path as identity — clones to different paths would appear as different repos, breaking the "same repo across machines" property colleagues will want.
- Call the GitHub API to resolve identity — introduces a network dependency, an authentication burden, and a vendor lock-in for tai's persistence layer. tai stores `owner/name` regardless of the host; the host (GitHub vs GitLab) becomes a column in the schema, deferred to `add-storage-schema`.
- Use a config file `tai.yaml` at the repo root — adds a configuration step before tai is usable and another file in users' repos. The git remote is already there.

### D3. Single human-readable error format covers both audiences

Every error written to stderr follows this template:

```
Error: <one-line summary of what went wrong>

What to do:
  • <remediation step 1>
  • <remediation step 2>

[exit <code>: <ERROR_CODE>]
```

The footer line is mandatory and machine-recognisable. Claude (and humans) can parse the `ERROR_CODE` token to branch behaviour while reading the prose for context. The same output stream serves both consumers.

**Alternatives considered:**

- JSON error output mode (e.g. `--json` or auto-on when stdout is piped) — doubles the surface area of every command, doubles the test matrix, and offers no real benefit since Claude consumes prose natively and the error codes provide machine-parseable hooks.
- Plain "Error: X" with no remediation block — leaves AI consumers without enough context to recover unaided. The remediation block doubles as documentation that explains the failure mode without users having to consult external docs.
- Stable codes without a footer — codes embedded in the message body are harder to extract reliably. A fixed-position footer is trivially regex-matchable.

### D4. Exit-code map follows `sysexits.h` traditions, narrowed

| Code | Meaning |
|------|---------|
| `0`  | Success |
| `1`  | Usage error (unknown subcommand, malformed flag, conflicting options) |
| `2`  | Precondition error (`REPO_NOT_FOUND`, no PR specified when one was required) |
| `3`  | Data/state error (invalid input payload, schema mismatch, conflict) |
| `70` | Internal error (panic, unexpected I/O failure, programmer error) |

Five codes is enough to drive CI scripting without inventing a full taxonomy.

**Alternatives considered:**

- Just `0` and `1` — too coarse for a CLI whose primary caller is an AI making branching decisions.
- A unique exit code per error code — explodes the contract and makes new error codes a breaking change to scripts.

### D5. Slash commands live under `~/.claude/commands/tai/<verb>.md`

The command-framework convention is: every user-facing tai verb `tai <verb>` has a paired Claude slash command `/tai:<verb>`, with the markdown file written to `~/.claude/commands/tai/<verb>.md`. The slash command body drives a conversation in Claude and shells out to `tai <verb>` for state operations. The pairing is one-to-one: every bundled slash command corresponds to exactly one CLI verb, and vice versa for verbs intended to be AI-driven (some verbs like `tai install` may not have a paired slash command — see D7).

**Alternatives considered:**

- Bundle a single mega-skill that auto-triggers based on context — less predictable (skill auto-triggering is heuristic), harder to debug, and discoverability for humans is worse (typing `/tai:` and tabbing is a clearer mental model than hoping a skill activates).
- Use Claude skills instead of commands — skills are auto-discovered; commands are explicit. For a tool whose primary value is reliable workflow execution, explicitness wins. (Note: this repo's `.claude/skills/openspec-*/SKILL.md` and `.claude/commands/opsx/*.md` show the parallel convention; tai chooses commands-only globally.)

### D6. Slash-command frontmatter carries an integer version and a content hash

Every bundled `.md` ships with frontmatter of this shape:

```yaml
---
name: "TAI: Import"
description: "…"
category: "Workflow"
tags: [tai, triage]
version: 2
content_hash: "sha256:abc123…"
---
```

`version` is a monotonically-incrementing integer per command, independent of the CLI's semver. It increments only when the command's body changes meaningfully; a CLI release that doesn't touch command bodies leaves `version` alone. `content_hash` is the sha256 of the body bytes (everything after the frontmatter) shipped in this build.

**Alternatives considered:**

- Semver per command — semver implies a public API contract between command files and something else. There isn't one. An integer is honest about what's happening: "the body changed, increment".
- No versioning — `tai install` would have no signal to decide whether an upgrade is needed beyond a hash compare. Versions also surface to humans in the frontmatter, which is useful for debugging "which version of the triage command am I running".

### D7. Build embeds a cumulative hash ledger per command

For each bundled command, the binary embeds the full history of `content_hash` values ever shipped for that command — generated at build time from the previous release's embedded ledger plus the current build's hashes. The ledger is the source of truth for `tai install`'s "is this file user-modified?" check (specified in `add-install-command`).

A file on disk is considered:

- **Up to date** if its hash equals the current build's hash for that command.
- **Stale but untouched** if its hash appears in the cumulative ledger but isn't current. Safe to overwrite without prompting.
- **User-modified** if its hash is not in the ledger at all. Overwrite requires confirmation.
- **Missing** if no file exists at the target path. Write the current version.

**Alternatives considered:**

- Always overwrite without checking — destroys user customisations silently. The user's policy is "we prefer to update, but warn if drifted", which requires hash awareness.
- Compare only against the previous version's hash — fails when a user is two or more versions behind. With a cumulative ledger, "untouched but old" works at any version gap.
- Store the ledger in a sidecar file shipped alongside the binary — fragile (users move binaries), defeats the point of a single static binary. Embed via `//go:embed`.

### D8. CLI framework is `urfave/cli` (inherited from `CLAUDE.md`)

`CLAUDE.md` already commits to `urfave/cli` for command/flag parsing. This design honours that — no relitigation. Global flags (`--repo`) and the subcommand-tree pattern map cleanly onto `urfave/cli`'s model. Decisions about *how* flags are wired (global vs. per-subcommand) are deferred to the proposals that introduce them; this proposal only reserves `--repo` as a global flag.

## Risks / Trade-offs

- **[XDG paths on macOS]** XDG isn't a macOS convention; "native" would be `~/Library/Application Support/tai/`. Choosing XDG cross-platform breaks symmetry with macOS native apps but matches every other modern Go CLI's choice. → Document the location prominently in `tai --help`; provide `TAI_DATA_DIR` for users who want to relocate.

- **[Repo identity collisions across hosts]** `acme/app` could exist on both GitHub and GitLab. tai conflates them today by only storing `owner/name`. → Reserve a `host` column in the schema (deferred to `add-storage-schema`); the foundation contract is forward-compatible because `--repo` already takes only `owner/name` and could be extended to `host/owner/name` without breaking callers.

- **[Repo identity for un-pushed clones]** A freshly-`git init`'d repo with no `origin` remote returns `REPO_NOT_FOUND`. Users who want to triage a fork's PR comments locally before pushing have no path. → Acceptable for v1; the workflow tai is built for begins with a pushed PR. Documented limitation in the spec.

- **[Error-code rot]** Once `REPO_NOT_FOUND` ships, removing or renaming it is a breaking change to anyone (human or AI) who scripted against it. → Codes are append-only. Renames require a new code; the old code is retired but its meaning is documented forever in `specs/cli-framework/spec.md`.

- **[Hash ledger size]** With ~6 commands and a release cadence of weekly hash changes, the ledger grows by ~300 entries per command per year. Each entry is ~70 bytes; even at decade scale this stays under 200 KB embedded. Negligible.

- **[Slash-command frontmatter drift]** If the frontmatter schema itself changes (e.g. we add a `min_cli_version` field), every existing installed command becomes "outdated frontmatter". → Schema changes require a foundation proposal that extends `command-framework`, with a written migration story; not a free-for-all per-feature change.

- **[urfave/cli lock-in]** Switching frameworks later would touch every subcommand. → Acceptable; this is a small CLI and the abstraction surface against `urfave/cli` will stay shallow.

## Migration Plan

Not applicable. This is the first change in a green-field repository; there is no prior state to migrate.

The order of subsequent proposals is:

1. `add-storage-schema` (depends on D1)
2. `add-install-command` (depends on D5, D6, D7)
3. `add-import-command` (depends on storage + install)
4. `add-triage-state` (depends on storage)
5. `add-triage-command` (depends on install + triage-state)

Project bootstrap (Go module init, directory layout, walking-skeleton `tai --version`) happens directly after this foundation lands and before `add-storage-schema` is applied. Bootstrap is deliberately outside the OpenSpec pipeline per the user's instruction.

## Resolved Questions

- **Q1. Short alias `-r` for `--repo`?** No. The flag is rarely typed by humans, and reserving the short alias preserves it for a future global flag where it would matter more.

- **Q2. `TAI_REPO` environment-variable override?** No. tai is not used in CI/CD pipelines where env vars would be natural. Flag-only keeps the contract minimal.

- **Q3. Should `tai --help` and `tai --version` exit `0` outside a git repo?** Yes. Codified in `specs/cli-framework/spec.md` as the "Commands that do not require repo context" requirement, with a per-command opt-in mechanism rather than a global default.

- **Q4. Hash ledger generation strategy.** Checked-in ledger file, updated by a release helper. Git-history regeneration is fragile when commands are renamed or moved, and a checked-in file is reviewable in PRs. The exact file path and serialisation format are owned by `add-install-command`; foundation only specifies the access contract (`Ledger(verb) []string`).
