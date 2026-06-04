# cli-framework Specification

## Purpose
TBD - created by archiving change add-tai-foundation. Update Purpose after archive.
## Requirements
### Requirement: Global data directory location

The system SHALL store all global per-user state (the SQLite database and any future global files) under a single base directory whose location follows the XDG Base Directory Specification.

The default location is the first available of:
1. `$TAI_DATA_DIR` if the environment variable is set and non-empty.
2. `$XDG_DATA_HOME/tai/` if `$XDG_DATA_HOME` is set and non-empty.
3. `~/.local/share/tai/` on Linux and macOS.
4. `%LOCALAPPDATA%\tai\` on Windows.

The directory MUST be created lazily on first write, not at startup. If the directory cannot be created or is not writable, the system MUST exit with error code `DATA_DIR_UNWRITABLE`.

#### Scenario: Default location on Linux without XDG override

- **WHEN** the CLI runs on Linux with `$XDG_DATA_HOME` unset and `$TAI_DATA_DIR` unset
- **THEN** the resolved data directory is `~/.local/share/tai/`

#### Scenario: XDG_DATA_HOME override

- **WHEN** `$XDG_DATA_HOME` is set to `/var/data` and `$TAI_DATA_DIR` is unset
- **THEN** the resolved data directory is `/var/data/tai/`

#### Scenario: TAI_DATA_DIR overrides XDG_DATA_HOME

- **WHEN** both `$TAI_DATA_DIR` and `$XDG_DATA_HOME` are set
- **THEN** `$TAI_DATA_DIR` wins and is used verbatim (no `tai/` suffix appended)

#### Scenario: Directory created lazily

- **WHEN** the CLI starts and the data directory does not exist
- **THEN** the CLI does not create the directory at startup
- **AND** the directory is created the first time a command attempts to write to it

#### Scenario: Unwritable directory

- **WHEN** the resolved data directory is on a read-only filesystem
- **AND** a command attempts to write to it
- **THEN** the CLI exits with code `3` and error code `DATA_DIR_UNWRITABLE`
- **AND** stderr explains that the directory is not writable and suggests setting `$TAI_DATA_DIR`

### Requirement: Human-readable error contract

When the CLI fails, it SHALL write a human-readable error message to stderr with the following structure:

```
Error: <one-line summary>

What to do:
  • <remediation step>
  • <additional remediation step if applicable>

[exit <exit_code>: <ERROR_CODE>]
```

The summary line MUST start with `Error: ` and describe what went wrong in plain language. The "What to do" block MUST appear unless there is genuinely no remediation the user can take, in which case the block is omitted. The footer line MUST be the last line written to stderr, MUST start with `[exit `, and MUST contain the numeric exit code followed by `: ` and the stable error code in uppercase snake case.

The CLI MUST NOT offer a JSON error mode. The same prose serves human and AI consumers.

#### Scenario: REPO_NOT_FOUND error format

- **WHEN** a command exits with `REPO_NOT_FOUND`
- **THEN** stderr contains an "Error:" line, a "What to do:" block, and a final line of the form `[exit 2: REPO_NOT_FOUND]`

#### Scenario: Internal error has no remediation block

- **WHEN** a command exits with `INTERNAL_ERROR` because of an unexpected panic
- **THEN** stderr contains the "Error:" line and the footer `[exit 70: INTERNAL_ERROR]`
- **AND** the "What to do:" block MAY be omitted

#### Scenario: Footer is the last line

- **WHEN** any error is written to stderr
- **THEN** the very last line of stderr matches the regex `^\[exit \d+: [A-Z][A-Z0-9_]*\]$`

### Requirement: Error-code taxonomy

The system SHALL maintain a stable, append-only taxonomy of error codes. Each error code is an uppercase snake-case identifier and is tied to a single exit code. Once shipped, an error code MUST NOT be renamed or repurposed. Codes MAY be marked deprecated but their exit code and meaning MUST remain stable.

The taxonomy spans both core and first-party plugins. Codes are organised into groups by emitting layer:

**Core foundation (emitted by `core/`)**

| Code | Exit | Meaning |
|------|------|---------|
| `UNKNOWN_SUBCOMMAND` | 1 | The user invoked a subcommand the CLI does not recognise. |
| `INTERNAL_ERROR` | 70 | An unexpected internal failure (panic recovery, unanticipated I/O failure). |
| `DATA_DIR_UNWRITABLE` | 3 | The resolved data directory cannot be created or written to. |
| `CONFIG_UNWRITABLE` | 3 | The resolved config file cannot be written. |
| `CONFIG_INVALID` | 1 | The config file is malformed or violates a structural rule. |
| `CONFIG_INVALID_REPO_URL` | 1 | `repo-url` is not a remote git URL (local paths and `file://` are rejected). |
| `CONFIG_KEY_NOT_SCRIPTABLE` | 1 | `tai config set` was used on a nested or array key. |
| `CONFIG_DUPLICATE_TARGET` | 1 | `tai config target add` was invoked for a `root` that already exists. |
| `CONFIG_TARGET_NOT_FOUND` | 1 | `tai config target remove` was invoked for a `root` that does not exist. |
| `CONFIG_EDITOR_UNSET` | 1 | `tai config edit` was invoked with no `$EDITOR` env var set. |
| `TAI_NOT_CONFIGURED` | 2 | An operation requiring both `repo-url` and `targets` was run with at least one missing. |
| `MISSING_ARG` | 1 | A subcommand was invoked with the wrong number of positional arguments (typically too few). |
| `REPO_FETCH_FAILED` | 3 | `git fetch` against the source repo failed for reasons other than offline (e.g. auth, 4xx). |
| `REPO_INIT_TARGET_NOT_EMPTY` | 1 | `tai repo init <path>` invoked on a non-empty directory. |
| `REPO_INIT_GIT_UNAVAILABLE` | 3 | `git` is not on PATH after `tai repo init` wrote the scaffold. |
| `WORKFLOW_INVALID` | 3 | A workflow YAML file fails schema validation or uses a reserved name. |
| `WORKFLOW_NOT_FOUND` | 2 | `tai workflow run <name>` referenced a workflow that does not exist. |
| `STANDARD_INVALID` | 3 | A standard's filename collides with a reserved word. |
| `STANDARD_NOT_FOUND` | 2 | `tai standards load <name>` referenced a standard that does not exist. |
| `PLUGIN_UNKNOWN` | 2 | `tai plugins <name> install` for a name not in the registry and no `--source`. |
| `PLUGIN_NAME_RESERVED` | 1 | An install was attempted for a plugin whose name collides with a reserved core verb. |
| `PLUGIN_ASSET_NAMING` | 3 | A plugin's skill/agent filename does not start with `tai-<plugin>-`. |
| `PLUGIN_FETCH_UNAUTHORIZED` | 3 | A 401/403 was returned by the release host during a plugin fetch. |
| `PLUGIN_FETCH_FAILED` | 3 | A non-401/403 failure during plugin fetch (network, 404, malformed asset). |

**Storage layer (emitted by plugins that use the shared storage helper, currently `triage`)**

| Code | Exit | Meaning |
|------|------|---------|
| `DB_OPEN_FAILED` | 3 | The database file exists or can be created, but a connection-level operation failed. |
| `DB_MIGRATION_FAILED` | 3 | One or more migrations failed to apply; the database is in its pre-migration state. |
| `DB_CONSTRAINT_VIOLATION` | 3 | An insert or update violated a `NOT NULL`, `CHECK`, `UNIQUE`, or foreign-key constraint. |

**Triage plugin (emitted by `plugins/triage`)**

| Code | Exit | Meaning |
|------|------|---------|
| `REPO_NOT_FOUND` | 2 | Working directory is not inside a git repo with an `origin` remote and no `--repo` was provided. |
| `REPO_FLAG_INVALID` | 1 | The value passed to `--repo` does not match `<owner>/<name>`. |
| `INSTALL_TARGET_UNWRITABLE` | 3 | The install target directory cannot be created or written to. |
| `INSTALL_INVALID_TARGET` | 1 | The value passed to `--commands-dir` is malformed. |
| `INSTALL_LEDGER_CORRUPT` | 70 | An embedded ledger file failed to parse at runtime. |
| `IMPORT_INVALID_JSON` | 1 | The stdin payload is not valid JSON. |
| `IMPORT_SCHEMA_INVALID` | 3 | The stdin JSON parses but fails one or more schema rules. |
| `IMPORT_AMBIGUOUS_REFS` | 3 | A comment's `external_refs` resolve to more than one existing comment row. |
| `TRIAGE_NO_SCOPE` | 2 | The current branch matches no PR and no branch row, and no `--pr`/`--branch` was provided. |
| `TRIAGE_AMBIGUOUS_SCOPE` | 2 | The current branch matches both a `prs.head_branch` and a `branches.name` row. |
| `TRIAGE_NOT_FOUND` | 2 | The referenced PR, branch, comment, or batch does not exist in the resolved scope. |
| `TRIAGE_INVALID_FLAGS` | 1 | Conflicting or missing flags on a triage verb. |
| `TRIAGE_CONFIRMATION_REQUIRED` | 1 | `tai forget` (now `tai triage forget`) was invoked non-interactively without `--yes` or a truthy `TAI_ACCEPT_DESTRUCTIVE`. |

Subsequent proposals MAY extend this taxonomy. Each new code MUST specify its emitting layer, exit code, and meaning in the spec that introduces it. Plugins that emit codes outside their own layer's table MUST register them via `pkg/errcode` so the taxonomy remains the single source of truth.

#### Scenario: Adding a new code does not change existing codes

- **WHEN** a future proposal adds a new code to the taxonomy
- **THEN** the existing codes retain their exit codes and meanings unchanged

#### Scenario: Core foundation codes are emitted by core only

- **WHEN** a command exits with `TAI_NOT_CONFIGURED`
- **THEN** stderr's footer line matches `^\[exit 2: TAI_NOT_CONFIGURED\]$`
- **AND** the emitter is part of the `core/` binary

#### Scenario: Plugin codes flow through the same template

- **WHEN** the Triage plugin exits with `TRIAGE_NO_SCOPE`
- **THEN** stderr's footer line matches `^\[exit 2: TRIAGE_NO_SCOPE\]$`
- **AND** the error follows the same human-readable error contract as core

### Requirement: Exit-code conventions

The system SHALL use the following exit codes:

| Code | Class | Examples |
|------|-------|----------|
| 0 | Success | Command completed as intended |
| 1 | Usage error | Unknown subcommand, malformed flag, conflicting options |
| 2 | Precondition error | Required repo context missing, required PR not specified |
| 3 | Data/state error | Invalid input payload, schema mismatch, state conflict |
| 70 | Internal error | Recovered panic, unanticipated I/O failure |

Every error code in the taxonomy MUST map to exactly one exit code. The CLI MUST NOT use exit codes other than those listed without first adding them to this requirement.

#### Scenario: Unknown subcommand exits 1

- **WHEN** the user runs `tai bogus`
- **THEN** the CLI exits with code `1` and error code `UNKNOWN_SUBCOMMAND`

#### Scenario: Successful command exits 0

- **WHEN** any command completes without error
- **THEN** the CLI exits with code `0`

### Requirement: Stdout-vs-stderr discipline across all commands

The system SHALL treat stdout and stderr as semantically distinct channels for every command, not only the error path:

- **Stdout = data.** Command results — listings, content, computed output. AI agents and shell pipelines consume stdout. No decorative prose, no progress chatter, no banners.
- **Stderr = conversation.** Errors, warnings, progress, prompts, the update banner. Anything that informs but is not the command's data product.

When stdout is not a TTY, the system MUST NOT emit ANSI color codes or carriage-return-driven progress animations to stdout. Stderr progress is allowed when stderr is a TTY.

Interactive prompts (e.g. the `tai sync` overwrite confirmation) MUST be written to stderr; only the response is read from stdin.

#### Scenario: Sync overwrite prompt is on stderr

- **WHEN** the user runs `tai sync` with overwrites pending, capturing stdout to a file
- **THEN** the prompt text appears on stderr, not in the captured stdout file
- **AND** stdin is read for the response

#### Scenario: Standards load output is stdout-only

- **WHEN** the user runs `tai standards load sdlc > out.md`
- **THEN** `out.md` contains exactly the standard's body bytes
- **AND** stderr is empty (no decorative chatter)

### Requirement: Framework packages are publicly importable under `pkg/`

The system SHALL expose its error-code taxonomy, error template writer, exit-code mapping, and plugin-author SDK as public Go packages under the repo's `pkg/` directory, importable by any external Go module via paths such as `github.com/dmastrorillo/tai/pkg/errcode`. Anything under `pkg/` is part of TAI's stability contract:

- Error code identifiers are append-only. Once shipped, a code MUST NOT be renamed or repurposed.
- Exit code bindings are immutable per code.
- Exported function signatures and type identities under `pkg/` MUST NOT change in a backwards-incompatible way without a major-version release.

Anything that is not part of the public contract MUST live under `core/internal/` or `plugins/<name>/internal/` so the Go compiler prevents external import.

#### Scenario: `pkg/errcode` compiles as part of the module build

- **WHEN** `go build ./...` runs at the repo root
- **THEN** the build succeeds with no errors
- **AND** the `pkg/errcode` package is exported with every error code referenced by `core/` and `plugins/triage/` available as a public symbol

(External-module import behaviour — both the success path for `pkg/...` and the rejection path for `core/internal/...` — is enforced structurally by the Go compiler's `internal/` rule and is not asserted by tests within this module.)
