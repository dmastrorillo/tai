## Why

`tai` (Triage AI) is a Go CLI that owns the persistence and I/O layer for the PR-review-triage workflow currently implemented as a single Claude skill writing JSON sidecar files. The CLI is invoked both directly by humans and indirectly by Claude commands that tai itself bundles. Before any feature can land, we need a foundation: where data lives on disk, how the CLI knows which repo it is in, what the error/output contract is, and how the bundled slash commands are paired with CLI verbs and versioned.

This proposal establishes those foundations so every subsequent feature proposal (`add-storage-schema`, `add-install-command`, `add-import-command`, `add-triage-state`, `add-triage-command`) can reference a shared, documented contract rather than re-litigating it.

## What Changes

- Introduce a global per-user data directory at `$XDG_DATA_HOME/tai/` (falling back to `~/.local/share/tai/` on Linux/macOS, `%LOCALAPPDATA%\tai\` on Windows). The SQLite database lives here. The directory is created lazily on first use.
- Define repo identity as the normalised `owner/name` parsed from the current working directory's `origin` remote URL (`git@github.com:acme/app.git` and `https://github.com/acme/app.git` both yield `acme/app`).
- Add a global `--repo <owner/name>` flag that overrides repo auto-detection on every command.
- Hard-error with the stable error code `REPO_NOT_FOUND` when a command needs repo context, no `--repo` flag is given, and the working directory is not inside a git repository with an `origin` remote.
- Define the human-readable error contract: every error written to stderr is prose explaining what went wrong, a "What to do" block of remediation steps, and a footer line `[exit <code>: <ERROR_CODE>]` with a stable code. The same prose serves both human and AI consumers — no JSON output mode.
- Reserve a stable error-code taxonomy in the foundation spec. Initial entries: `REPO_NOT_FOUND`, `REPO_FLAG_INVALID`, `DATA_DIR_UNWRITABLE`, `UNKNOWN_SUBCOMMAND`, `INTERNAL_ERROR`. Each subsequent proposal extends this list.
- Define exit-code conventions: `0` success, `1` usage error, `2` precondition error (e.g. `REPO_NOT_FOUND`), `3` data/state error, `70` internal error.
- Establish the command-framework convention: every user-facing tai verb (`tai <verb>`) has a paired Claude slash command `/tai:<verb>` installed under `~/.claude/commands/tai/<verb>.md`. The slash command markdown drives a conversation and shells out to `tai <verb>` for state operations.
- Define the slash-command frontmatter schema. Each bundled `.md` carries `name`, `description`, `category`, `tags`, `version` (monotonically-incrementing integer, scoped per command, independent of CLI semver), and `content_hash` (sha256 of the body bytes shipped in this build). The hash exists so `tai install` can distinguish "untouched but older" from "user-modified".
- Require that the build embeds, for each bundled command, the cumulative ledger of every `content_hash` ever shipped for that command. `tai install` (specified in a later proposal) reconciles disk state against this ledger.

## Capabilities

### New Capabilities

- `cli-framework`: Global data directory, repo-context detection, `--repo` flag, human-readable error contract, stable error-code taxonomy, and exit-code conventions. The contract every tai subcommand obeys.
- `command-framework`: Convention that every `tai <verb>` has a paired `/tai:<verb>` Claude slash command, the frontmatter schema for bundled commands (including versioning and content hashing), and the rule that the binary embeds a cumulative hash ledger per command.

### Modified Capabilities

None. This is the first change; there are no existing specs to modify.

## Impact

- Establishes conventions every subsequent proposal references. Errors raised from any future subcommand must conform to the contract defined here.
- No production code yet — proposals up to and including this one are spec-only. Implementation begins once the bootstrap (project layout, Go module, `test-cases.md` skeleton, walking-skeleton `tai --version`) is in place. Bootstrap is explicitly out of scope for OpenSpec and will be performed directly before any spec is applied.
- No external dependencies introduced. Foundation references only the Go standard library plus the planned `urfave/cli` choice from `CLAUDE.md`.
- Distribution model assumed: tai is shipped as a single static Go binary that embeds its bundled slash-command markdowns and their hash ledgers via `//go:embed`. Detailed install semantics (write paths, idempotency, override flag, prompt-on-modified) are deferred to `add-install-command`.
