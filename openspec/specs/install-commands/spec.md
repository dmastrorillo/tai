# install-commands Specification

## Purpose
TBD - created by archiving change pivot-to-ai-as-code. Update Purpose after archive.
## Requirements
### Requirement: `tai install-commands` ships TAI's built-in slash commands into every target

The system SHALL accept `tai install-commands` to install TAI's own built-in slash-command assets (bundled with the `tai` binary) into every configured target. For each target, the system writes the bundled commands into `<target.root>/<target.commands>/tai/`, creating the `tai/` subdirectory if needed.

If no targets are configured, the command MUST exit with `TAI_NOT_CONFIGURED` and the "what to do" bullets MUST point at `tai config target add`.

A target whose `commands` sub-path is falsy SHALL be skipped, with a warning on stderr naming the target.

On success, the system SHALL print a one-line summary to stdout of the form `installed <N> command(s) into <M> target(s)`, with a trailing `(<K> stale built-in(s) removed)` parenthetical when `K > 0`. When every configured target was skipped via a falsy `commands` sub-path, the system SHALL instead print `all <N> target(s) skipped — nothing installed` so the zero-count case is distinct from a successful no-op.

#### Scenario: Install with one configured target

- **WHEN** a single target `~/.claude` is configured with default sub-paths
- **AND** the user runs `tai install-commands`
- **THEN** every bundled command file is written under `~/.claude/commands/tai/`

#### Scenario: Install with multiple targets

- **WHEN** two targets are configured: `~/.claude` and `~/.opencode`
- **AND** the user runs `tai install-commands`
- **THEN** every bundled command file is written into both `~/.claude/commands/tai/` and `~/.opencode/commands/tai/`

#### Scenario: Install with no targets

- **WHEN** no targets are configured and the user runs `tai install-commands`
- **THEN** the command exits with `TAI_NOT_CONFIGURED`

#### Scenario: Falsy commands sub-path skips that target with warning

- **WHEN** a target has `commands: ""` configured and the user runs `tai install-commands`
- **THEN** that target receives no files
- **AND** stderr contains a warning naming the skipped target

### Requirement: Idempotent re-runs overwrite within the `tai/` subdirectory only

Re-running `tai install-commands` SHALL overwrite the existing files under `<target.root>/<target.commands>/tai/` without prompting. Files outside that subdirectory MUST NOT be touched.

The `tai/` subdirectory is treated as TAI-owned namespace; user-authored content placed outside that subdirectory is preserved.

#### Scenario: Re-run replaces existing built-ins

- **WHEN** `tai install-commands` has run once, and the user re-runs it after a TAI binary upgrade with a new bundled command
- **THEN** the new command appears in the target's `commands/tai/` subdirectory
- **AND** the user's other content under `commands/` (outside `commands/tai/`) is unchanged

### Requirement: Removing built-ins

When the running `tai` binary no longer bundles a command that a previous `tai install-commands` invocation installed, the next `tai install-commands` SHALL remove the stale file from `<target.root>/<target.commands>/tai/`. Because the `tai/` subdirectory is wholly TAI-owned, no manifest is needed — re-running computes the target state from the current bundle.

#### Scenario: Stale built-in removed

- **WHEN** TAI bundled a command `legacy.md` in a previous version and an old install put it at `~/.claude/commands/tai/legacy.md`
- **AND** the new TAI binary no longer bundles `legacy.md`
- **AND** the user runs `tai install-commands`
- **THEN** `~/.claude/commands/tai/legacy.md` is removed
