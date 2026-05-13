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

### Requirement: Repo identity detection

The system SHALL identify the current repository by parsing the normalised `owner/name` from the `origin` git remote URL of the working directory.

The parser MUST accept SSH (`git@github.com:acme/app.git`, `ssh://git@github.com/acme/app.git`) and HTTPS (`https://github.com/acme/app.git`, `https://github.com/acme/app`) URL forms and normalise them to `owner/name` by stripping any trailing `.git` suffix. The parser MUST NOT depend on a third-party git library; reading `git config --get remote.origin.url` via the standard library is sufficient.

#### Scenario: SSH origin URL

- **WHEN** the working directory's `origin` remote is `git@github.com:acme/app.git`
- **THEN** the resolved repo identity is `acme/app`

#### Scenario: HTTPS origin URL with .git suffix

- **WHEN** the working directory's `origin` remote is `https://github.com/acme/app.git`
- **THEN** the resolved repo identity is `acme/app`

#### Scenario: HTTPS origin URL without .git suffix

- **WHEN** the working directory's `origin` remote is `https://github.com/acme/app`
- **THEN** the resolved repo identity is `acme/app`

#### Scenario: Not inside a git repository

- **WHEN** the working directory is not inside any git repository
- **AND** a command requires repo context
- **AND** no `--repo` flag is provided
- **THEN** the CLI exits with code `2` and error code `REPO_NOT_FOUND`
- **AND** stderr explains that the directory is not a git repository and suggests `cd`-ing into one or passing `--repo`

#### Scenario: Git repository without origin remote

- **WHEN** the working directory is a git repository with no `origin` remote
- **AND** a command requires repo context
- **AND** no `--repo` flag is provided
- **THEN** the CLI exits with code `2` and error code `REPO_NOT_FOUND`
- **AND** stderr explains that no `origin` remote was found and suggests `git remote add origin …` or passing `--repo`

### Requirement: Global --repo override flag

The system SHALL accept a global `--repo <owner/name>` flag on every command that uses repo context. When provided, the flag overrides auto-detection. The value MUST match the format `<owner>/<name>` where `<owner>` and `<name>` each contain at least one character and no `/`.

#### Scenario: --repo overrides detection

- **WHEN** the command runs inside a git repository whose `origin` is `acme/app`
- **AND** the user passes `--repo acme/other`
- **THEN** the resolved repo identity for this invocation is `acme/other`

#### Scenario: --repo allows running outside a git repo

- **WHEN** the working directory is not inside any git repository
- **AND** the user passes `--repo acme/app`
- **THEN** the CLI proceeds with repo identity `acme/app` (no `REPO_NOT_FOUND` error)

#### Scenario: Malformed --repo value

- **WHEN** the user passes `--repo just-a-name` (no slash)
- **THEN** the CLI exits with code `1` and error code `REPO_FLAG_INVALID`
- **AND** stderr explains the expected `owner/name` format

### Requirement: Commands that do not require repo context

The system SHALL allow `tai --help`, `tai --version`, and any subcommand explicitly marked as repo-independent to run without resolving repo identity.

When such a command runs outside a git repository and without a `--repo` flag, it MUST NOT exit with `REPO_NOT_FOUND`.

#### Scenario: tai --help outside a git repo

- **WHEN** `tai --help` is invoked from a directory that is not inside any git repository
- **THEN** the CLI prints help to stdout and exits with code `0`

#### Scenario: tai --version outside a git repo

- **WHEN** `tai --version` is invoked from a directory that is not inside any git repository
- **THEN** the CLI prints the version string to stdout and exits with code `0`

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

The system SHALL maintain a stable, append-only taxonomy of error codes. Each error code is an uppercase snake-case identifier and is tied to a single exit code.

Once shipped, an error code MUST NOT be renamed or repurposed. Codes MAY be marked deprecated but their exit code and meaning MUST remain stable.

The initial taxonomy is:

| Code | Exit | Meaning |
|------|------|---------|
| `REPO_NOT_FOUND` | 2 | Working directory is not inside a git repo with an `origin` remote, and no `--repo` was provided. |
| `REPO_FLAG_INVALID` | 1 | The value passed to `--repo` does not match `<owner>/<name>`. |
| `DATA_DIR_UNWRITABLE` | 3 | The resolved data directory cannot be created or written to. |
| `UNKNOWN_SUBCOMMAND` | 1 | The user invoked a subcommand the CLI does not recognise. |
| `INTERNAL_ERROR` | 70 | An unexpected internal failure (panic recovery, I/O failure not anticipated by a more specific code). |

Subsequent proposals MAY extend this taxonomy. Each new code MUST specify its exit code and its meaning in the spec that introduces it.

#### Scenario: Adding a new code does not change existing codes

- **WHEN** a future proposal adds `COMMENT_INVALID_SCHEMA` to the taxonomy
- **THEN** the existing codes `REPO_NOT_FOUND`, `REPO_FLAG_INVALID`, `DATA_DIR_UNWRITABLE`, `UNKNOWN_SUBCOMMAND`, and `INTERNAL_ERROR` retain their exit codes and meanings unchanged

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

