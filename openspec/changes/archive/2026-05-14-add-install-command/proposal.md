## Why

The `command-framework` foundation defines the *contract* — every `tai <verb>` has a paired `/tai:<verb>` Claude slash command, versioned with `version` + `content_hash` frontmatter, with a cumulative hash ledger accessible at runtime. It deferred the *mechanism* — where the ledger lives, how the install command reads it, how stale-but-untouched is distinguished from user-modified, how the install/uninstall verbs behave. This proposal closes that loop.

`tai install` is the entry point for everyone except the binary's distribution channel: after `brew install tai` (or equivalent), users run `tai install` once to wire the bundled slash commands into Claude. Running it again after a `tai` upgrade refreshes any commands whose bodies changed. The version+hash machinery exists so a user-edited command is not silently overwritten on the next upgrade.

## What Changes

- Introduce `tai install` — idempotent verb that writes every bundled slash command to `~/.claude/commands/tai/<verb>.md`, creating the target directory if missing, and reconciling files already on disk against the embedded hash ledger.
- Introduce `tai uninstall` — symmetric verb that removes every recognised tai command from the target directory and removes the directory itself if empty afterward. Files that don't correspond to a bundled tai command are left untouched.
- Define the concrete on-disk and embedded format of the hash ledger. One ledger JSON file per bundled command at `commands/<verb>.ledger.json`, checked into the tai repository, embedded into the binary via `//go:embed`. The file is an ordered array of hash strings, oldest first.
- Define a build-time helper (`tai-ledger update` or equivalent — a developer-only tool, not user-facing) that recomputes each command's current hash and appends new hashes to the corresponding ledger before each release. The helper is invoked from the build pipeline; running it manually is the path used during development.
- Define the four file-state classifications (`missing`, `up-to-date`, `stale-but-untouched`, `user-modified`) and the install command's behaviour for each:
  - `missing` — write current version
  - `up-to-date` — skip silently
  - `stale-but-untouched` — overwrite silently
  - `user-modified` — prompt the user (`Overwrite? [y/N]`), default no. With `--force` or the environment variable `TAI_ACCEPT_COMMAND_UPDATES=1`, overwrite without prompting. With non-interactive stdin and neither override set, skip and report in the summary.
- Provide flags on `tai install`:
  - `--commands-dir <path>` overrides the target directory (default `~/.claude/commands/tai/`).
  - `--force` overwrites user-modified files without prompting.
- Provide a flag on `tai uninstall`:
  - `--commands-dir <path>` mirrors install's override.
  - `--force` removes user-modified files without prompting (default is to leave them in place and report them in the summary).
- Honour the environment variable `TAI_ACCEPT_COMMAND_UPDATES=1` across both `tai install` and `tai uninstall`. When set to `1`, the verb behaves as if `--force` were passed. This is intended for users who keep tai up to date in a non-interactive context (a login shell hook, a personal Makefile, etc.) and accept that local edits will be overwritten.
- Emit a human-readable summary at the end of every `tai install` / `tai uninstall` invocation listing per-command outcome (`installed`, `updated`, `skipped`, `prompted-skipped`, `removed`, `not-found`) and a one-line aggregate.
- Reserve `tai update` (binary self-update) as a follow-on capability. Distribution channel (Homebrew, `curl | sh`, GitHub Releases, …) is undecided, and the self-update mechanism depends on the channel chosen. This proposal does NOT spec binary self-update; a future proposal will when distribution is settled.
- Reserve new error codes in the `cli-framework` taxonomy: `INSTALL_TARGET_UNWRITABLE` (exit 3), `INSTALL_INVALID_TARGET` (exit 1), `INSTALL_LEDGER_CORRUPT` (exit 70). The "user declined to overwrite" path is NOT an error — it produces a successful exit with a summary line reporting the skip.

## Capabilities

### New Capabilities

- `install`: The `tai install` and `tai uninstall` verbs, the concrete hash-ledger file format and embedding mechanism, the four file-state classifications, the prompt/force/dry-run UX, and the install-layer error codes.

### Modified Capabilities

- `cli-framework`: Extends the error-code taxonomy with `INSTALL_TARGET_UNWRITABLE`, `INSTALL_INVALID_TARGET`, `INSTALL_LEDGER_CORRUPT`. Additive, per the taxonomy's append-only rule.

## Impact

- No new third-party dependencies. The install command uses the standard library (`os`, `io/fs`, `embed`, `path/filepath`) plus the `cmdframework` package introduced by the foundation.
- The build pipeline gains one new step: invoking the ledger-update helper before tagging a release. The helper is not part of the user-facing CLI; it ships in a separate `cmd/tai-ledger/` binary or as a `go generate` directive. Detailed packaging is a `design.md` decision.
- `tai install` is the first user-facing verb that does not require repo context. It is added to the foundation's "Commands that do not require repo context" carve-out.
- This proposal does NOT touch the SQLite database. No migrations, no rows written. Install is purely a filesystem operation against `~/.claude/commands/tai/`.
- Binary size grows by a few KB for the embedded ledgers; immaterial.
