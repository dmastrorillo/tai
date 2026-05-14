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

Subsequent proposals MAY extend this taxonomy. Each new code MUST specify its exit code and its meaning in the spec that introduces it.

#### Scenario: Adding a new code does not change existing codes

- **WHEN** a future proposal adds a new code to the taxonomy
- **THEN** the existing codes retain their exit codes and meanings unchanged

#### Scenario: Install codes inherit foundation contract

- **WHEN** a command exits with `INSTALL_TARGET_UNWRITABLE`
- **THEN** stderr's footer line matches `^\[exit 3: INSTALL_TARGET_UNWRITABLE\]$`
- **AND** the error follows the human-readable error contract
