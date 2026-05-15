## MODIFIED Requirements

### Requirement: Error-code taxonomy

The system SHALL maintain a stable, append-only taxonomy of error codes. Each error code is an uppercase snake-case identifier and is tied to a single exit code.

Once shipped, an error code MUST NOT be renamed or repurposed. Codes MAY be marked deprecated but their exit code and meaning MUST remain stable.

The taxonomy is:

| Code | Exit | Meaning |
|------|------|---------|
| `REPO_NOT_FOUND` | 2 | Working directory is not inside a git repo with an `origin` remote, and no `--repo` was provided. |
| `REPO_FLAG_INVALID` | 1 | The value passed to `--repo` does not match `<owner>/<name>`. |
| `DATA_DIR_UNWRITABLE` | 3 | The resolved data directory cannot be created or written to. |
| `UNKNOWN_SUBCOMMAND` | 1 | The user invoked a subcommand the CLI does not recognise. |
| `INTERNAL_ERROR` | 70 | An unexpected internal failure (panic recovery, I/O failure not anticipated by a more specific code). |
| `DB_OPEN_FAILED` | 3 | The database file exists or can be created, but a connection-level operation (open, pragma) failed. |
| `DB_MIGRATION_FAILED` | 3 | One or more migrations failed to apply; the database is in its pre-migration state. |
| `DB_CONSTRAINT_VIOLATION` | 3 | An insert or update violated a `NOT NULL`, `CHECK`, `UNIQUE`, or foreign-key constraint. |
| `INSTALL_TARGET_UNWRITABLE` | 3 | The install target directory cannot be created or is not writable. |
| `INSTALL_INVALID_TARGET` | 1 | The value passed to `--commands-dir` is malformed. |
| `INSTALL_LEDGER_CORRUPT` | 70 | An embedded ledger file failed to parse at runtime. |
| `IMPORT_INVALID_JSON` | 1 | The stdin payload is not valid JSON. |
| `IMPORT_SCHEMA_INVALID` | 3 | The stdin JSON parses but fails one or more schema rules. |
| `IMPORT_AMBIGUOUS_REFS` | 3 | A comment's `external_refs` resolve to more than one existing comment row. |
| `TRIAGE_NO_SCOPE` | 2 | The current branch matches no PR and no branch row, and no `--pr`/`--branch` was provided. |
| `TRIAGE_AMBIGUOUS_SCOPE` | 2 | The current branch matches both a `prs.head_branch` and a `branches.name` row. |
| `TRIAGE_NOT_FOUND` | 2 | The referenced PR, branch, comment, or batch does not exist in the resolved scope. |
| `TRIAGE_INVALID_FLAGS` | 1 | Conflicting or missing flags on a triage verb (e.g. `--pr` + `--branch`, missing `--reason`, `--id` + `--batch`). |
| `TRIAGE_CONFIRMATION_REQUIRED` | 1 | `tai forget` was invoked non-interactively without `--yes` or a truthy `TAI_ACCEPT_DESTRUCTIVE`. |

Subsequent proposals MAY extend this taxonomy. Each new code MUST specify its exit code and its meaning in the spec that introduces it.

#### Scenario: Adding a new code does not change existing codes

- **WHEN** a future proposal adds a new code to the taxonomy
- **THEN** the existing codes retain their exit codes and meanings unchanged

#### Scenario: Triage codes inherit foundation contract

- **WHEN** a command exits with `TRIAGE_NO_SCOPE`
- **THEN** stderr's footer line matches `^\[exit 2: TRIAGE_NO_SCOPE\]$`
- **AND** the error follows the human-readable error contract
