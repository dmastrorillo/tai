## Context

The `command-framework` foundation specifies that each bundled slash command carries an integer `version` and a `content_hash` in its frontmatter, and that the binary embeds a cumulative ledger of every hash ever shipped for each command. The classification of a file on disk against that ledger drives the install command's behaviour:

| Disk-file state | Hash matches… | Action |
|---|---|---|
| missing | — | write current version |
| up-to-date | the current build's hash | skip silently |
| stale-but-untouched | a prior entry in the ledger (not current) | overwrite silently |
| user-modified | no entry in the ledger | prompt (or `--force` / skip) |

This document records the technical decisions behind the ledger file format, the install/uninstall command flow, the prompt UX, and the build-time ledger update helper.

## Goals / Non-Goals

**Goals:**

- A ledger format that is reviewable in PRs (the diff to a JSON file is the diff to the install behaviour of a future version).
- An install command whose behaviour is fully deterministic given the binary's embedded ledger and the disk state — no clock-dependence, no network.
- A prompt UX that defaults to safe (`[y/N]` with default `N`) and is non-interactive-friendly (skip rather than hang).
- An uninstall command that doesn't accidentally delete files it didn't write.

**Non-Goals:**

- Binary self-update. Reserved as a follow-on capability (`tai update`) once a distribution channel is chosen. Mechanism (GitHub Releases download, `brew upgrade tai` shell-out, etc.) depends on the channel, so it is impossible to spec meaningfully today. This proposal restricts itself to installing bundled slash commands; the binary's own version is whatever the package manager / curl-installer left on disk.
- Installing for other users on the system (no `--system` global install). One user, one home directory.
- Plugin / third-party command support. Only the commands embedded in this binary are recognised.
- Migration of pre-existing user-written slash commands at `~/.claude/commands/`. tai owns only its `tai/` subdirectory.

## Decisions

### D1. One ledger file per command, JSON array, oldest-first

A bundled command at `commands/<verb>.md` has a ledger at `commands/<verb>.ledger.json`:

```json
[
  "sha256:8b1a9953c4611296a827abf8c47804d7",
  "sha256:e3b0c44298fc1c149afbf4c8996fb924"
]
```

The array is ordered oldest-first. Appending happens at the end. The build-time helper appends exactly one entry per release in which the command body changed.

The current build's hash MUST be the last entry in the ledger; otherwise the build fails (caught by a `go test` check, not a runtime check).

**Alternatives considered:**

- One combined `ledger.json` for all commands. Simpler embed; PR diffs to a multi-command release touch one shared file and conflict more often during parallel feature work. Per-command files are friendlier to git.
- A flat list of `(verb, hash, version)` triples. More data per release but redundant — the current `version` is already in each command's frontmatter, and the install command only needs the hash list.
- A binary format. Premature optimisation; JSON is human-readable and trivial to diff.

### D2. Build-time ledger helper, not runtime regeneration

A small developer tool — `cmd/tai-ledger/main.go` — does one job: for each `commands/<verb>.md`, recompute the body hash and, if absent from `commands/<verb>.ledger.json`, append it. Runs in the release pipeline before `go build`. Local developers run it after changing a command body.

A `go test` check verifies that every command's current hash equals the last entry of its ledger; this catches "forgot to run the helper" before merge.

**Alternatives considered:**

- Regenerate hashes from git history at build time. Tempting but fragile when files are renamed or moved. A checked-in ledger is reviewable, diffable, and survives renames cleanly (a rename is "new file with a fresh ledger").
- `go generate` directive embedded in the command file. Works, but `go generate` has to be invoked manually and is often forgotten. A Makefile / release-script step is more explicit.
- A pre-commit hook. Too aggressive — every local edit to a command body would mutate the ledger, polluting branches with hash entries that may never reach a release.

### D3. Four file-state classifications, no `--force` for `stale-but-untouched`

The classification table is exhaustive over `(file exists?, hash in ledger?, hash is current?)`. There is no need for `--force` to apply to stale-but-untouched files; that case overwrites silently. `--force` exists only to suppress the prompt on `user-modified`.

**Alternatives considered:**

- Always prompt unless `--force`. Annoying — every routine upgrade would require user input.
- Never prompt, always overwrite. The "warn if drifted" requirement explicitly forbids this.

### D4. Prompt is `[y/N]` with default `N`; non-interactive stdin skips; env var or `--force` overrides

The prompt format:

```
The file at ~/.claude/commands/tai/import.md has been modified locally.
Overwrite with the version bundled in tai 0.7.0? [y/N]
```

Three overrides bypass the prompt entirely and overwrite:

1. `--force` — explicit per-invocation override.
2. `TAI_ACCEPT_COMMAND_UPDATES=1` — environment variable, persistent across invocations. Intended for users who keep tai up to date as part of a personal automation (shell rc, dotfiles bootstrap, etc.).
3. Interactive answer `y`/`Y` at the prompt.

If `STDIN` is not a TTY and neither override is set, the prompt is suppressed and the file is left in place. The summary reports it as `prompted-skipped`. The exit code is `0` — failing to update a user-modified file is not an error.

The env var and the flag have identical effect; the flag takes precedence over the env var only in the sense that explicit usually beats implicit (operationally they produce the same outcome). When both are set, no conflict arises.

**Alternatives considered:**

- Default to yes (overwrite without asking by default). Surprising; users running `tai install` in a script might overwrite intentional customisations.
- Bail with an error code when non-interactive and a modified file is found. Hostile to scripted use; many users will accept "tai install installs what it can and tells you about what it skipped" as the contract.
- Use a config file (`~/.tai.yaml`) for the persistent default instead of an env var. tai is intentionally configurationless; env vars are the project's chosen knob for cross-invocation behaviour.

### D5. Uninstall removes only files recognised as ours

`tai uninstall` walks the target directory and removes a file iff:

1. Its filename is `<verb>.md` for some `<verb>` the current binary knows about, AND
2. Its content hash appears in that verb's ledger (i.e. it's a tai-shipped version, possibly older).

A file with a tai-style name but an unknown hash is treated as `user-modified` and left in place (`--force` removes it). Files with names that don't match any known verb are never touched.

After removing files, if the target directory is empty, it is removed too. If not (some other tool's files live there), the directory is preserved.

**Alternatives considered:**

- Remove every file in the directory. Wrong; the directory may host other commands from other tools eventually.
- Track installation in a manifest. Adds state for no gain; the embedded ledgers already tell us which hashes are "ours".

### D6. `--commands-dir` accepts any writable directory; default is platform-dependent

Default target directory:

| Platform | Default |
|---|---|
| Linux / macOS | `~/.claude/commands/tai/` |
| Windows | `%USERPROFILE%\.claude\commands\tai\` |

`--commands-dir <path>` overrides the default. The path MUST be a directory that either exists and is writable, or whose parent exists and is writable (the leaf is created). Malformed path → exit `1` with `INSTALL_INVALID_TARGET`.

**Alternatives considered:**

- Honor `$CLAUDE_COMMANDS_DIR` if set. No documented Claude convention for this env var; would couple tai to a non-public surface. Skip until Claude publishes one.

### D7. `tai install` is repo-independent

Per the foundation contract, repo-independent verbs are explicitly listed. `tai install` and `tai uninstall` join `--help` and `--version` in that list. They never invoke the repo resolver.

### D8. Summary format is single-block, human-readable, tail-includes exit info

```
$ tai install
Installed:   2 commands (import, triage)
Updated:     0 commands
Skipped:     0 commands (up to date)
Prompted-skipped: 1 command (uninstall: user declined)
Errors:      0

[exit 0]
```

If any error code fired mid-run, the standard error footer (`[exit N: CODE]`) replaces the success line and is the last thing on stderr; the summary still goes to stdout (best-effort).

**Alternatives considered:**

- A line per command instead of grouped counts. Noisy when nothing changed; a one-line "all up to date" is much friendlier.

### D9. No `--dry-run` flag

Install/uninstall are small, fast, idempotent operations. Adding a dry-run mode would double the test matrix for marginal benefit — users curious about what would change can simply look at the summary of the real run, since the operation is idempotent and the worst-case outcome of running it is "your commands are now up to date".

**Alternatives considered:**

- Provide `--dry-run` for parity with bigger CLIs. Rejected as bloat. If users ever ask for it, easy to add.

## Risks / Trade-offs

- **[Ledger corruption]** A malformed `ledger.json` embedded in the binary breaks every install. Caught at build time by a `go test`; runtime fall-back is `INSTALL_LEDGER_CORRUPT` → exit 70. → Build check is the primary defence; runtime check is defence in depth.

- **[User edits frontmatter, not body]** A user who edits the frontmatter (e.g. tweaks `description`) but not the body has a file whose body hash still matches the ledger. tai treats this as `up-to-date` and overwrites the frontmatter silently. → Documented limitation: the contract is "body bytes are tai's, frontmatter is tai's, file is tai's". Users who want a customised description should not edit installed files — a future proposal could add user-overlay support if anyone asks.

- **[Concurrent `tai install` invocations]** Two processes installing at once could race when creating the directory. Not a real-world problem (the install verb is run once interactively after upgrade) but worth noting. → No locking; the operations are idempotent and POSIX `mkdir`/`rename` are atomic enough that the worst outcome is one process losing to the other and re-running succeeds.

- **[Removed verb leaves a stale file]** If a future tai release retires the `foo` verb, `tai install` no longer knows about `foo.md`; `tai uninstall` leaves it alone (filename matches no known verb). → Acceptable. A retired-verb cleanup helper can ship in a later proposal if needed; not worth the surface area today.

- **[Prompts in TUI / non-standard terminals]** `[y/N]` reading via the standard library handles common cases; subtler issues (raw-mode terminals, ANSI escapes in piped input) are unlikely to affect this verb's audience. → Defer until reported.

- **[Default Windows path]** `~/.claude/...` is a Unix-ism; Windows users expect `%USERPROFILE%\...`. The CLI resolves `~` correctly cross-platform via `os.UserHomeDir`, but the documentation must show the right form. → Spec writes both forms; help text resolves at runtime.

## Migration Plan

This is the first proposal that touches `~/.claude/commands/tai/`. There is no prior state to migrate. Existing tai users (after future releases) re-run `tai install`; the hash-ledger machinery handles upgrades from any prior version.

## Open Questions

(None remaining.)
