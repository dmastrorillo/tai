# storage Specification

## Purpose
TBD - created by archiving change add-storage-schema. Update Purpose after archive.
## Requirements
### Requirement: Database file location

The system SHALL store all persistent triage state in a single SQLite database file at `<data_dir>/tai.db`, where `<data_dir>` is resolved per the `cli-framework` capability's data-directory rules.

The file MUST be created lazily on first write (not at startup). The file path MUST NOT be configurable through any mechanism other than the data-directory contract already exposed by `cli-framework` (i.e. `TAI_DATA_DIR` / `XDG_DATA_HOME`).

#### Scenario: Default location on Linux

- **WHEN** the CLI runs on Linux with `$XDG_DATA_HOME` unset and `$TAI_DATA_DIR` unset
- **THEN** the resolved database path is `~/.local/share/tai/tai.db`

#### Scenario: Database created lazily

- **WHEN** `tai --help` is invoked and no command writes to the database
- **THEN** no `tai.db` file is created

#### Scenario: TAI_DATA_DIR honoured

- **WHEN** `$TAI_DATA_DIR` is set to `/tmp/tai-test`
- **AND** a command writes to the database
- **THEN** the database file is created at `/tmp/tai-test/tai.db`

### Requirement: Connection-open policy

Each database connection opened by the CLI SHALL execute the following pragmas as part of opening, before any user-driven query runs:

1. `PRAGMA journal_mode = WAL` (write-ahead logging)
2. `PRAGMA foreign_keys = ON` (enforce FK constraints)
3. `PRAGMA busy_timeout = 1000` (wait up to 1 second on contended writes)

If any pragma fails the system MUST exit with error code `DB_OPEN_FAILED`.

#### Scenario: Foreign keys enabled on every connection

- **WHEN** any command opens the database
- **THEN** `PRAGMA foreign_keys` returns `1` for that connection

#### Scenario: WAL mode active

- **WHEN** any command opens the database for the first time
- **THEN** `PRAGMA journal_mode` returns `wal`

#### Scenario: Pragma failure aborts

- **WHEN** the database file exists but cannot be opened (e.g. corrupt header)
- **THEN** the CLI exits with code `3` and error code `DB_OPEN_FAILED`

### Requirement: Migration runner

The system SHALL apply embedded SQL migrations on every CLI startup that needs database access. Migrations are stored as numbered SQL files (`001_init.sql`, `002_…`) embedded into the binary at build time.

A `migrations` table records which versions have been applied. The runner SHALL:

1. Open the database (creating the file if missing).
2. Apply the connection-open policy (`PRAGMA` calls above).
3. Read the highest applied version from the `migrations` table (treat missing table as version 0).
4. For each unapplied migration in ascending version order: execute the migration inside a transaction, then insert a row into `migrations` recording its version, name, and `applied_at` timestamp.

If any migration fails, the transaction MUST roll back and the CLI MUST exit with `DB_MIGRATION_FAILED`. Subsequent commands MUST refuse to run until the migration succeeds.

Migrations are forward-only. The runner MUST NOT support down-migrations.

#### Scenario: Fresh database applies all migrations

- **WHEN** the CLI runs against a non-existent database file
- **THEN** the file is created
- **AND** every embedded migration is applied in order
- **AND** the `migrations` table contains one row per migration with monotonically increasing `version` values

#### Scenario: Second startup is a no-op

- **WHEN** the CLI runs twice in succession against the same database
- **THEN** the second invocation observes no new migrations to apply
- **AND** completes without error

#### Scenario: Failed migration rolls back

- **WHEN** a migration file contains a syntax error
- **THEN** the CLI exits with code `3` and error code `DB_MIGRATION_FAILED`
- **AND** no partial state from the failed migration persists in the database

### Requirement: Schema — `repos` table

The system SHALL provide a `repos` table with the following columns and constraints:

| Column | Type | Constraints |
|---|---|---|
| `id` | INTEGER | PRIMARY KEY |
| `owner_name` | TEXT | NOT NULL, UNIQUE |
| `created_at` | INTEGER | NOT NULL (Unix epoch seconds) |

`owner_name` is the canonical repo identity (e.g. `"acme/app"`) as resolved by the `cli-framework` repo-context rules.

#### Scenario: Insert a new repo

- **WHEN** the importer inserts a row with `owner_name = 'acme/app'`
- **THEN** the row is persisted with an auto-assigned `id`

#### Scenario: Duplicate owner_name rejected

- **WHEN** an insert attempts a second row with `owner_name = 'acme/app'`
- **THEN** the insert fails and the CLI exits with code `3` and error code `DB_CONSTRAINT_VIOLATION`

### Requirement: Schema — `prs` table

The system SHALL provide a `prs` table with the following columns and constraints:

| Column | Type | Constraints |
|---|---|---|
| `id` | INTEGER | PRIMARY KEY |
| `repo_id` | INTEGER | NOT NULL, REFERENCES `repos(id)` ON DELETE CASCADE |
| `number` | INTEGER | NOT NULL |
| `title` | TEXT | NOT NULL |
| `url` | TEXT | NOT NULL |
| `head_branch` | TEXT | NOT NULL |
| `created_at` | INTEGER | NOT NULL |

UNIQUE constraint on `(repo_id, number)`.

#### Scenario: PR uniquely identified within a repo

- **WHEN** two repos both have a PR numbered 1
- **THEN** both inserts succeed; both rows coexist with distinct `id` values

#### Scenario: Duplicate PR number within a repo rejected

- **WHEN** an insert attempts a second `prs` row with the same `(repo_id, number)`
- **THEN** the insert fails with `DB_CONSTRAINT_VIOLATION`

#### Scenario: Repo deletion cascades to PRs

- **WHEN** a `repos` row is deleted
- **THEN** all `prs` rows referencing that repo are deleted

### Requirement: Schema — `branches` table

The system SHALL provide a `branches` table with the following columns and constraints:

| Column | Type | Constraints |
|---|---|---|
| `id` | INTEGER | PRIMARY KEY |
| `repo_id` | INTEGER | NOT NULL, REFERENCES `repos(id)` ON DELETE CASCADE |
| `name` | TEXT | NOT NULL |
| `created_at` | INTEGER | NOT NULL |

UNIQUE constraint on `(repo_id, name)`.

#### Scenario: Branch uniquely identified within a repo

- **WHEN** the importer inserts `(repo_id=1, name='feat/x')`
- **THEN** the row is persisted

#### Scenario: Duplicate branch within a repo rejected

- **WHEN** an insert attempts a second `branches` row with the same `(repo_id, name)`
- **THEN** the insert fails with `DB_CONSTRAINT_VIOLATION`

#### Scenario: Repo deletion cascades to branches

- **WHEN** a `repos` row is deleted
- **THEN** all `branches` rows referencing that repo are deleted

### Requirement: Schema — `comments` table

The system SHALL provide a `comments` table with the following columns and constraints:

| Column | Type | Constraints |
|---|---|---|
| `id` | INTEGER | PRIMARY KEY |
| `pr_id` | INTEGER | NULLABLE, REFERENCES `prs(id)` ON DELETE CASCADE |
| `branch_id` | INTEGER | NULLABLE, REFERENCES `branches(id)` ON DELETE CASCADE |
| `batch_id` | INTEGER | NULLABLE, REFERENCES `batches(id)` ON DELETE SET NULL |
| `severity` | TEXT | NOT NULL, CHECK in (`critical`, `major`, `minor`, `nitpick`) |
| `category` | TEXT | NOT NULL, CHECK in (`security`, `correctness`, `feature-regression`, `code-quality`, `performance`, `testing`) |
| `file` | TEXT | NOT NULL |
| `lines` | TEXT | NOT NULL |
| `source` | TEXT | NOT NULL (display string, e.g. `"coderabbit + greptile"`) |
| `title` | TEXT | NOT NULL |
| `description` | TEXT | NOT NULL |
| `why_fix` | TEXT | NOT NULL |
| `suggested_fix` | TEXT | NOT NULL |
| `consequences` | TEXT | NOT NULL |
| `status` | TEXT | NOT NULL, DEFAULT `pending`, CHECK in (`pending`, `accepted`, `dismissed`, `completed`) |
| `resolution` | TEXT | NULLABLE |
| `dismissed_by` | TEXT | NULLABLE |
| `dismiss_reason` | TEXT | NULLABLE |
| `created_at` | INTEGER | NOT NULL |
| `updated_at` | INTEGER | NOT NULL |

CHECK constraint enforcing exactly one parent: `(pr_id IS NOT NULL AND branch_id IS NULL) OR (pr_id IS NULL AND branch_id IS NOT NULL)`.

Indexes: `(pr_id, status)`, `(branch_id, status)`, `batch_id`.

#### Scenario: Comment with valid PR parent

- **WHEN** a comment is inserted with `pr_id = 1` and `branch_id = NULL`
- **THEN** the insert succeeds

#### Scenario: Comment with valid branch parent

- **WHEN** a comment is inserted with `pr_id = NULL` and `branch_id = 1`
- **THEN** the insert succeeds

#### Scenario: Comment with both parents rejected

- **WHEN** a comment is inserted with both `pr_id` and `branch_id` non-NULL
- **THEN** the insert fails with `DB_CONSTRAINT_VIOLATION`

#### Scenario: Comment with no parents rejected

- **WHEN** a comment is inserted with both `pr_id` and `branch_id` NULL
- **THEN** the insert fails with `DB_CONSTRAINT_VIOLATION`

#### Scenario: Required enrichment field missing

- **WHEN** a comment is inserted without `why_fix`
- **THEN** the insert fails with `DB_CONSTRAINT_VIOLATION`

#### Scenario: Invalid severity value

- **WHEN** a comment is inserted with `severity = 'urgent'`
- **THEN** the insert fails with `DB_CONSTRAINT_VIOLATION`

#### Scenario: Invalid status value

- **WHEN** an update sets `status = 'archived'`
- **THEN** the update fails with `DB_CONSTRAINT_VIOLATION`

#### Scenario: PR deletion cascades to comments

- **WHEN** a `prs` row is deleted
- **THEN** all `comments` rows referencing that PR are deleted

#### Scenario: Batch deletion preserves comments

- **WHEN** a `batches` row is deleted
- **THEN** member comments are NOT deleted; their `batch_id` is set to NULL

### Requirement: Schema — `comment_external_refs` table

The system SHALL provide a `comment_external_refs` table tracking the external provenance of each comment:

| Column | Type | Constraints |
|---|---|---|
| `id` | INTEGER | PRIMARY KEY |
| `comment_id` | INTEGER | NOT NULL, REFERENCES `comments(id)` ON DELETE CASCADE |
| `source_kind` | TEXT | NOT NULL (e.g. `github-pr-comment`, `github-review-body`, `github-issue-comment`, `manual`) |
| `external_id` | TEXT | NOT NULL |
| `reviewer` | TEXT | NULLABLE (author login or display name when known) |

UNIQUE constraint on `(source_kind, external_id)`.

#### Scenario: Two refs from different sources for one comment

- **WHEN** an importer attaches `(source_kind=github-pr-comment, external_id=12345)` and `(source_kind=github-pr-comment, external_id=12346)` to the same `comment_id`
- **THEN** both rows persist

#### Scenario: Duplicate external ref rejected

- **WHEN** an importer attempts a second row with the same `(source_kind, external_id)`
- **THEN** the insert fails with `DB_CONSTRAINT_VIOLATION`

#### Scenario: Comment deletion cascades to refs

- **WHEN** a `comments` row is deleted
- **THEN** all `comment_external_refs` rows referencing that comment are deleted

### Requirement: Schema — `batches` table

The system SHALL provide a `batches` table:

| Column | Type | Constraints |
|---|---|---|
| `id` | INTEGER | PRIMARY KEY |
| `pr_id` | INTEGER | NULLABLE, REFERENCES `prs(id)` ON DELETE CASCADE |
| `branch_id` | INTEGER | NULLABLE, REFERENCES `branches(id)` ON DELETE CASCADE |
| `batch_key` | TEXT | NOT NULL (display key, e.g. `B1`) |
| `title` | TEXT | NOT NULL |
| `status` | TEXT | NOT NULL, DEFAULT `pending`, CHECK in (`pending`, `accepted`, `dismissed`, `completed`, `mixed`) |
| `created_at` | INTEGER | NOT NULL |

CHECK constraint enforcing exactly one parent (same XOR rule as `comments`). UNIQUE constraint on `(pr_id, batch_key)` AND `(branch_id, batch_key)` — keys are scoped per review target.

#### Scenario: Batch with PR parent

- **WHEN** a batch is inserted with `pr_id = 1`, `batch_key = 'B1'`
- **THEN** the row is persisted

#### Scenario: Duplicate batch key within a PR rejected

- **WHEN** a second batch with `(pr_id=1, batch_key='B1')` is inserted
- **THEN** the insert fails with `DB_CONSTRAINT_VIOLATION`

#### Scenario: Batch status 'mixed' is valid

- **WHEN** an update sets `batches.status = 'mixed'`
- **THEN** the update succeeds

#### Scenario: Batch with no parent rejected

- **WHEN** a batch is inserted with both `pr_id` and `branch_id` NULL
- **THEN** the insert fails with `DB_CONSTRAINT_VIOLATION`

### Requirement: Schema — `migrations` table

The system SHALL provide a `migrations` table tracking applied schema versions:

| Column | Type | Constraints |
|---|---|---|
| `version` | INTEGER | PRIMARY KEY |
| `name` | TEXT | NOT NULL |
| `applied_at` | INTEGER | NOT NULL |

#### Scenario: First migration recorded

- **WHEN** the runner applies `001_init.sql`
- **THEN** a row exists in `migrations` with `version = 1` and a non-null `applied_at`

### Requirement: Storage-layer error codes

The system SHALL extend the `cli-framework` error-code taxonomy with three storage-layer codes:

| Code | Exit | Meaning |
|---|---|---|
| `DB_OPEN_FAILED` | 3 | The database file exists or can be created, but a connection-level operation (open, pragma) failed. |
| `DB_MIGRATION_FAILED` | 3 | One or more migrations failed to apply; the database is in its pre-migration state. |
| `DB_CONSTRAINT_VIOLATION` | 3 | An insert or update violated a `NOT NULL`, `CHECK`, `UNIQUE`, or foreign-key constraint. |

These codes are append-only additions to the foundation taxonomy; they MUST NOT alter the exit codes or meanings of pre-existing codes.

#### Scenario: Open failure surfaces DB_OPEN_FAILED

- **WHEN** the database file cannot be opened
- **THEN** the CLI exits with code `3`
- **AND** stderr's footer line is `[exit 3: DB_OPEN_FAILED]`

#### Scenario: Constraint violation surfaces DB_CONSTRAINT_VIOLATION

- **WHEN** a write fails because a `CHECK` constraint is violated
- **THEN** the CLI exits with code `3`
- **AND** stderr's footer line is `[exit 3: DB_CONSTRAINT_VIOLATION]`

