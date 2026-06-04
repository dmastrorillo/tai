# config Specification

## Purpose
TBD - created by archiving change pivot-to-ai-as-code. Update Purpose after archive.
## Requirements
### Requirement: Config file location and lazy creation

The system SHALL store TAI's local configuration in a single YAML file whose location follows the XDG Base Directory Specification.

The default location is the first available of:
1. `$TAI_CONFIG` if the environment variable is set and non-empty.
2. `$XDG_CONFIG_HOME/tai/config.yml` if `$XDG_CONFIG_HOME` is set and non-empty.
3. `~/.config/tai/config.yml` on Linux and macOS.
4. `%AppData%\tai\config.yml` on Windows.

The file MUST NOT be created on first invocation, first `--help`, or first `--version`. It MUST be created lazily on the first write — any of `tai config set`, `tai config edit`, `tai config target add`. When created via `tai config edit`, the file MUST be populated with a commented template documenting every supported field before being opened in `$EDITOR`.

If the resolved config file cannot be created or its parent directory is not writable, the system MUST exit with error code `CONFIG_UNWRITABLE`.

#### Scenario: Default location on Linux with no overrides

- **WHEN** TAI runs on Linux with `$XDG_CONFIG_HOME` unset and `$TAI_CONFIG` unset
- **THEN** the resolved config path is `~/.config/tai/config.yml`

#### Scenario: TAI_CONFIG overrides XDG_CONFIG_HOME

- **WHEN** `$TAI_CONFIG` is set to `/tmp/test/config.yml` and `$XDG_CONFIG_HOME` is also set
- **THEN** the resolved config path is `/tmp/test/config.yml`

#### Scenario: First-invocation no-write

- **WHEN** a fresh user runs `tai --help` with no config file present
- **THEN** the command succeeds with exit 0 and writes help to stdout
- **AND** no config file is created on disk

#### Scenario: Lazy creation on first write

- **WHEN** a fresh user runs `tai config target add ~/.claude` with no config file present
- **THEN** the parent directory is created if needed
- **AND** a new config file is written with the new target

### Requirement: Config schema

The config file SHALL accept the following top-level keys, all optional individually:

- `repo-url`: a remote git URL (SSH `git@host:path`, `ssh://`, or `https://`). Local paths and `file://` URLs are not accepted.
- `targets`: an array of objects. Each object has a required `root` (filesystem path) and optional `skills`, `commands`, `agents` (sub-paths relative to `root`). Sub-paths default to `skills`, `commands`, `agents` respectively.
- `update-check-interval`: a Go-style duration string (e.g. `6h`, `30m`). Default `6h`. Value `0` disables the background update check.

`repo-url` and `targets` are co-required for any operation that writes to a target or reads from the source repo. Either may be set without the other only when the user is mid-setup; operations that require both MUST exit with error `TAI_NOT_CONFIGURED` when only one is set.

A target sub-path set to a falsy YAML value (`""`, `false`, `null`) MUST be treated as "skip this category" for that target. Targets with all three sub-paths falsy SHALL be rejected as `CONFIG_INVALID` at validation time.

#### Scenario: Sub-path defaults

- **WHEN** the config contains a target with `root: ~/.claude` and no sub-paths
- **THEN** the effective sub-paths for that target resolve to `~/.claude/skills`, `~/.claude/commands`, `~/.claude/agents`

#### Scenario: Falsy sub-path skips category with warning

- **WHEN** the config contains a target with `commands: ""` and the source repo has commands to sync
- **THEN** TAI prints a warning to stderr naming the target and the skipped category
- **AND** no commands are written to that target

#### Scenario: Local path rejected at config set time

- **WHEN** the user runs `tai config set repo-url file:///tmp/repo`
- **THEN** the command exits with `CONFIG_INVALID_REPO_URL`
- **AND** the existing config is not modified

#### Scenario: Missing co-required field

- **WHEN** the config has `repo-url` set but `targets` empty, and the user runs `tai sync`
- **THEN** the command exits with `TAI_NOT_CONFIGURED`
- **AND** the "what to do" bullets name `tai config target add` as the resolution

### Requirement: `tai config show` surface

The system SHALL print the current config as YAML to stdout in response to `tai config show`. If the config file does not exist, the command SHALL print an informational message naming the path and pointing at `tai config target add` and `tai config edit` as the next steps. The exit code is `0` in both cases — absence of config is not an error.

#### Scenario: Show with existing config

- **WHEN** the user runs `tai config show` with a populated config
- **THEN** stdout contains the YAML representation of the current config
- **AND** the exit code is `0`

#### Scenario: Show with no config

- **WHEN** the user runs `tai config show` with no config file present
- **THEN** stdout contains a message naming the resolved config path and the next-step commands
- **AND** the exit code is `0`

### Requirement: `tai config edit` surface

The system SHALL accept `tai config edit` to open the config file in `$EDITOR` for direct editing. When the config file does not exist, the system MUST create it with the commented template described in the lazy-creation requirement before opening it. When `$EDITOR` is unset, the command MUST exit with `CONFIG_EDITOR_UNSET`, with "what to do" bullets naming the env var.

The command MUST NOT modify any content of an existing config file beyond what the user writes through the editor — re-opening and saving an unchanged file MUST leave the bytes identical.

#### Scenario: Edit on fresh install creates the template and opens the editor

- **WHEN** no config file exists and the user runs `tai config edit` with `$EDITOR` set to a command that records its argv to a file and exits 0
- **THEN** the config file exists at the resolved path
- **AND** the file contents include every commented field documented in the template (`repo-url`, `targets`, `update-check-interval`)
- **AND** the recorded argv includes the resolved config path

#### Scenario: Edit round-trips an existing config without modification

- **WHEN** a populated config file exists and the user runs `tai config edit` with `$EDITOR` set to a no-op command that exits 0 immediately
- **THEN** the config file's bytes are identical before and after the command exits

#### Scenario: Edit without `$EDITOR` errors

- **WHEN** `$EDITOR` is unset and the user runs `tai config edit`
- **THEN** the command exits with `CONFIG_EDITOR_UNSET`
- **AND** the config file is not created or modified

### Requirement: `tai config set` surface

The system SHALL accept `tai config set <key> <value>` for scalar top-level keys (`repo-url`, `update-check-interval`). Setting a key with a nested or array path (e.g. `targets[0].root`) MUST exit with `CONFIG_KEY_NOT_SCRIPTABLE`, with "what to do" bullets pointing at the dedicated subcommand or `tai config edit`.

#### Scenario: Set scalar top-level key

- **WHEN** the user runs `tai config set repo-url git@github.com:acme/repo.git`
- **THEN** the config file is updated with the new value
- **AND** other keys are preserved

#### Scenario: Set rejects nested keys

- **WHEN** the user runs `tai config set targets[0].root ~/.claude`
- **THEN** the command exits with `CONFIG_KEY_NOT_SCRIPTABLE`
- **AND** the config file is not modified

### Requirement: `tai config target` surface

The system SHALL provide three subcommands for managing the targets array:

- `tai config target add <root> [--skills X] [--commands Y] [--agents Z]`: appends a new target. If a target with the same `root` already exists, the command MUST exit with `CONFIG_DUPLICATE_TARGET`.
- `tai config target list`: prints the configured targets as a table on stdout — columns `root`, `skills`, `commands`, `agents`. If no targets are configured, prints `(no targets configured)`.
- `tai config target remove <root>`: removes the target whose `root` matches exactly. If no matching target exists, the command MUST exit with `CONFIG_TARGET_NOT_FOUND`.

#### Scenario: Add a target

- **WHEN** the user runs `tai config target add ~/.claude --skills custom-skills`
- **THEN** the config file gains a new target with `root: ~/.claude` and `skills: custom-skills`
- **AND** `commands` and `agents` are absent (defaulting to their standard names at sync time)

#### Scenario: Add a duplicate target

- **WHEN** the user runs `tai config target add ~/.claude` and a target with that root already exists
- **THEN** the command exits with `CONFIG_DUPLICATE_TARGET`
- **AND** the existing target is preserved unchanged

#### Scenario: Remove a target

- **WHEN** the user runs `tai config target remove ~/.claude` and that target exists
- **THEN** the target is removed from the config file
- **AND** other targets are preserved
