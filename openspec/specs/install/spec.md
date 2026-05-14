# install Specification

## Purpose

The `install` capability defines the `tai install` and `tai uninstall` verbs that wire the binary's bundled Claude slash commands into the user's `~/.claude/commands/tai/` directory. It owns the concrete on-disk and embedded format of the per-command hash ledger, the four file-state classifications (`missing`, `up-to-date`, `stale-but-untouched`, `user-modified`), the prompt / `--force` / non-interactive UX, and the install-layer error codes. `tai install` is the entry point users run once after `brew install tai` (or equivalent) and after every upgrade to refresh any commands whose bodies changed without silently overwriting locally edited files.

## Requirements

### Requirement: `tai install` writes bundled commands to the target directory

The system SHALL provide a `tai install` subcommand that writes every bundled slash-command markdown to the target directory, creating the directory (and any missing parents) if needed.

The default target directory is `~/.claude/commands/tai/` on Linux and macOS, and `%USERPROFILE%\.claude\commands\tai\` on Windows. The path `~` is resolved via the user's home directory at runtime.

If the target directory or any required parent cannot be created or is not writable, the CLI MUST exit with code `3` and error code `INSTALL_TARGET_UNWRITABLE`.

#### Scenario: Fresh install creates the directory

- **WHEN** `tai install` runs and `~/.claude/commands/tai/` does not exist
- **THEN** the directory is created
- **AND** every bundled command's markdown file is written under it

#### Scenario: Idempotent re-run does nothing observable

- **WHEN** `tai install` is run twice in succession with no changes between runs
- **THEN** the second run produces a summary of `Installed: 0, Updated: 0, Skipped: <n> (up to date)`
- **AND** no file modification times change on the second run

#### Scenario: Unwritable target directory

- **WHEN** `tai install` runs and the target directory is on a read-only filesystem
- **THEN** the CLI exits with code `3` and error code `INSTALL_TARGET_UNWRITABLE`

### Requirement: `tai install` is repo-independent

The `tai install` and `tai uninstall` subcommands SHALL NOT require repo context. They MUST NOT invoke the repo resolver and MUST NOT produce `REPO_NOT_FOUND` when run outside a git repository.

#### Scenario: tai install outside a git repo

- **WHEN** `tai install` is invoked from a directory that is not a git repository
- **THEN** the install proceeds normally (no `REPO_NOT_FOUND`)

### Requirement: File-state classification against the hash ledger

The system SHALL classify each target file's state by inspecting the file on disk and comparing its body hash against the embedded cumulative ledger for that command. The four classifications are:

| Classification | Disk-file state | Action |
|---|---|---|
| `missing` | No file at target path | Write current version |
| `up-to-date` | File body hash equals current build's hash | Skip silently |
| `stale-but-untouched` | File body hash appears in the ledger but is not the current entry | Overwrite silently |
| `user-modified` | File body hash does not appear in the ledger | Prompt the user (or `--force` / skip per stdin-tty rules) |

The body hash MUST be computed using the same algorithm specified by `command-framework` (sha256 over body bytes only, excluding the `---` delimiter line).

#### Scenario: missing → write

- **WHEN** the target file does not exist
- **THEN** the install writes the bundled version
- **AND** the summary reports `Installed: 1 command`

#### Scenario: up-to-date → skip

- **WHEN** the target file exists and its body hash equals the current build's hash
- **THEN** the install does not modify the file
- **AND** the summary reports the command under `Skipped (up to date)`

#### Scenario: stale-but-untouched → overwrite silently

- **WHEN** the target file's body hash appears in the ledger but is not the current entry
- **THEN** the install overwrites the file without prompting
- **AND** the summary reports the command under `Updated`

#### Scenario: user-modified with interactive stdin and no override → prompt

- **WHEN** the target file's body hash is absent from the ledger
- **AND** stdin is a TTY
- **AND** neither `--force` nor `TAI_ACCEPT_COMMAND_UPDATES=1` is set
- **THEN** the install prompts `Overwrite? [y/N]` with default `N`
- **AND** answering `n` (or pressing return) leaves the file unchanged; the summary reports it under `Prompted-skipped`
- **AND** answering `y` overwrites the file; the summary reports it under `Updated`

#### Scenario: user-modified with non-interactive stdin and no override → skip

- **WHEN** the target file's body hash is absent from the ledger
- **AND** stdin is not a TTY
- **AND** neither `--force` nor `TAI_ACCEPT_COMMAND_UPDATES=1` is set
- **THEN** the install does not modify the file
- **AND** the summary reports it under `Prompted-skipped`
- **AND** the CLI exits with code `0`

#### Scenario: user-modified with --force → overwrite

- **WHEN** the target file's body hash is absent from the ledger
- **AND** `--force` is provided
- **THEN** the install overwrites the file without prompting
- **AND** the summary reports the command under `Updated`

#### Scenario: user-modified with truthy TAI_ACCEPT_COMMAND_UPDATES → overwrite

- **WHEN** the target file's body hash is absent from the ledger
- **AND** `TAI_ACCEPT_COMMAND_UPDATES` is set to any truthy value (case-insensitive `1`, `true`, `yes`, `on`, `y`, `t`)
- **AND** `--force` is not provided
- **THEN** the install overwrites the file without prompting
- **AND** the summary reports the command under `Updated`

#### Scenario: Falsy or empty TAI_ACCEPT_COMMAND_UPDATES is ignored

- **WHEN** `TAI_ACCEPT_COMMAND_UPDATES` is unset, empty, or set to a non-truthy value (case-insensitive `0`, `false`, `no`, `off`, `n`, `f`, or any unrecognised string)
- **THEN** the install behaves as if the env var were unset

### Requirement: `tai install` flags

The system SHALL accept the following flags on `tai install`:

- `--commands-dir <path>`: Override the target directory. The path MUST resolve to a writable directory (or a path whose parent is a writable directory).
- `--force`: Overwrite user-modified files without prompting.

Malformed `--commands-dir` (empty string, path traversal attempt outside a writable area) SHALL exit with code `1` and error code `INSTALL_INVALID_TARGET`.

#### Scenario: --commands-dir override

- **WHEN** `tai install --commands-dir /tmp/cmds` is invoked
- **AND** `/tmp/cmds` exists and is writable
- **THEN** bundled commands are written under `/tmp/cmds/`

#### Scenario: --force overwrites user-modified

- **WHEN** a target file's body hash is absent from the ledger
- **AND** `tai install --force` is invoked
- **THEN** the file is overwritten without a prompt

#### Scenario: Malformed --commands-dir

- **WHEN** `tai install --commands-dir ""` is invoked
- **THEN** the CLI exits with code `1` and error code `INSTALL_INVALID_TARGET`

### Requirement: `tai uninstall` removes recognised tai commands

The system SHALL provide a `tai uninstall` subcommand that removes files in the target directory iff they correspond to a bundled tai command and their body hash appears in that command's ledger.

A file in the target directory whose filename matches `<verb>.md` for a known verb but whose body hash is NOT in the ledger MUST be treated as user-modified and left in place — unless `--force` is provided or `TAI_ACCEPT_COMMAND_UPDATES=1` is set in the environment, in which case it is removed.

A file in the target directory whose filename does not match any known verb MUST be left in place.

After processing, if the target directory is empty, it MUST be removed. If non-empty, it MUST be preserved.

#### Scenario: Uninstall removes installed files

- **WHEN** every file in `~/.claude/commands/tai/` corresponds to a bundled verb with a hash in the ledger
- **AND** `tai uninstall` runs
- **THEN** all files are removed
- **AND** the empty directory is removed
- **AND** the summary reports each command under `Removed`

#### Scenario: Uninstall leaves user-modified file in place

- **WHEN** a file `import.md` exists with a body hash not in the ledger
- **AND** `tai uninstall` runs without `--force` and without `TAI_ACCEPT_COMMAND_UPDATES=1`
- **THEN** the file is not removed
- **AND** the summary reports it under `Prompted-skipped`

#### Scenario: Uninstall ignores unrelated files

- **WHEN** the target directory also contains `unrelated.md` (no matching tai verb)
- **AND** `tai uninstall` runs
- **THEN** `unrelated.md` is preserved
- **AND** the directory is preserved because it is non-empty

#### Scenario: Uninstall --force removes user-modified

- **WHEN** a file with a body hash not in the ledger exists
- **AND** `tai uninstall --force` runs
- **THEN** the file is removed

#### Scenario: Uninstall with TAI_ACCEPT_COMMAND_UPDATES=1 removes user-modified

- **WHEN** a file with a body hash not in the ledger exists
- **AND** `TAI_ACCEPT_COMMAND_UPDATES=1` is set in the environment
- **AND** `tai uninstall` runs without `--force`
- **THEN** the file is removed

### Requirement: Hash-ledger file format and embedding

The system SHALL store, for every bundled command at `commands/<verb>.md`, a sibling ledger file at `commands/<verb>.ledger.json` containing a JSON array of body-hash strings, ordered oldest-first, each formatted as `sha256:<64 lowercase hex chars>`.

The ledger files SHALL be embedded into the binary via `//go:embed`. At runtime, the `cmdframework.Ledger(verb)` function defined by `command-framework` returns the parsed array for the requested verb.

The current build's body hash MUST be the last entry in each command's ledger. A build-time test MUST verify this invariant and fail the build if violated.

If the embedded ledger for a verb fails to parse at runtime, the CLI MUST exit with code `70` and error code `INSTALL_LEDGER_CORRUPT`.

#### Scenario: Ledger file shape

- **WHEN** the file `commands/import.ledger.json` is read
- **THEN** it contains a JSON array whose entries match `^sha256:[0-9a-f]{64}$`
- **AND** the array is ordered oldest-first

#### Scenario: Current hash is last entry

- **WHEN** the binary is built from a checked-in tree
- **THEN** for each bundled command, the body hash of `commands/<verb>.md` equals the last element of `commands/<verb>.ledger.json`

#### Scenario: Corrupt ledger surfaces at runtime

- **WHEN** an embedded ledger contains malformed JSON
- **AND** any install-touching code reads it
- **THEN** the CLI exits with code `70` and error code `INSTALL_LEDGER_CORRUPT`

### Requirement: Install summary output

`tai install` and `tai uninstall` SHALL emit a human-readable summary block to stdout at the end of every successful run. The block contains, in order:

1. A line per non-zero outcome bucket. The noun is `command` when `N == 1` and `commands` otherwise:
   - `Installed: <N> command|commands (<comma-separated verbs>)`
   - `Updated: <N> command|commands (<verbs>)`
   - `Skipped: <N> command|commands (up to date)`
   - `Prompted-skipped: <N> command|commands (<verbs and reason>)`
   - `Removed: <N> command|commands (<verbs>)` (uninstall only)
   - `Not-found: <N> command|commands (<verbs>)` (uninstall only, when a verb's file was already missing)
2. A blank line.
3. An exit-tag line `[exit 0]` when the run was fully successful, or the standard error footer `[exit <code>: <ERROR_CODE>]` on failure.

Outcome buckets with zero entries MAY be omitted. The summary's verb lists MAY be omitted when N > 5 (replaced with `…`).

#### Scenario: Clean install summary

- **WHEN** `tai install` runs against a clean target directory
- **THEN** stdout contains an `Installed: <N>` line listing every bundled verb
- **AND** the last line of stdout is `[exit 0]`

#### Scenario: Mixed-outcome summary

- **WHEN** `tai install` runs against a directory where some files are up to date, one is stale-but-untouched, and one is user-modified with non-interactive stdin
- **THEN** the summary contains both an `Updated: 1` and a `Prompted-skipped: 1` line
- **AND** the exit code is `0`
