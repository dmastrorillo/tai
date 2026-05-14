# Implementation Tasks

> **Prerequisite (out of pipeline):** `add-tai-foundation` is implemented (data directory resolver, error-code taxonomy, exit-code wiring all in place).
>
> Each numbered task is a TDD slice. Add the corresponding BDD case(s) to `test-cases.md` under the `STG` category (new — add to ToC if absent), assign TC-IDs, write the failing test, then implement.

## 1. Driver selection and connection wiring

- [x] 1.1 Add `modernc.org/sqlite` to `go.mod`
- [x] 1.2 Implement `internal/storage` package skeleton: `Open(ctx, path string) (*DB, error)` opens the file, applies WAL / foreign_keys / busy_timeout pragmas
- [x] 1.3 TC-STG-001..003: connection-open scenarios from spec (WAL active, foreign_keys=1, pragma failure surfaces `DB_OPEN_FAILED`)
- [x] 1.4 Wire storage open into the CLI bootstrap so every subcommand that uses the DB calls `storage.Open`

## 2. Migration runner

- [x] 2.1 Embed `internal/storage/migrations/*.sql` via `//go:embed`
- [x] 2.2 Implement migration runner: reads `migrations` table, applies unapplied files in version order inside transactions, records each on success
- [x] 2.3 TC-STG-004..006: fresh DB applies all, second startup is no-op, failed migration rolls back and surfaces `DB_MIGRATION_FAILED`
- [x] 2.4 Integration test (build tag `integration`): point the runner at a tmp file, run twice, assert idempotency. Covered as a unit test (`TestMigrations_TCSTG005_second_open_is_noop`) against `t.TempDir()`; no slow / environment-sensitive paths warrant the build-tag split.

## 3. Initial migration — `001_init.sql`

- [x] 3.1 Author `001_init.sql` creating all six tables (`repos`, `prs`, `branches`, `comments`, `comment_external_refs`, `batches`, plus `migrations`)
- [x] 3.2 Author indexes: `comments(pr_id, status)`, `comments(branch_id, status)`, `comments(batch_id)`, `comment_external_refs(comment_id)`, `comment_external_refs(reviewer)`
- [x] 3.3 Author all `CHECK` constraints from the spec: comment XOR parent, batch XOR parent, severity enum, category enum, comment status enum, batch status enum
- [x] 3.4 Author all `UNIQUE` constraints: `repos.owner_name`, `prs(repo_id, number)`, `branches(repo_id, name)`, `comment_external_refs(source_kind, external_id)`, `batches(pr_id, batch_key)`, `batches(branch_id, batch_key)`
- [x] 3.5 Author CASCADE rules per the spec table; verify SET NULL on `comments.batch_id` when a batch is deleted

## 4. Schema verification tests (per-table)

- [x] 4.1 TC-STG-010..012: `repos` table — insert succeeds, duplicate `owner_name` rejected, cascade to children
- [x] 4.2 TC-STG-020..023: `prs` table — insert succeeds, `(repo_id, number)` unique, cascade from repo, head_branch NOT NULL
- [x] 4.3 TC-STG-030..032: `branches` table — insert succeeds, `(repo_id, name)` unique, cascade from repo
- [x] 4.4 TC-STG-040..049: `comments` table — XOR constraint (PR-only, branch-only, both rejected, neither rejected), enrichment NOT NULL, severity enum, category enum, status enum, cascade from PR, cascade from branch, batch deletion sets `batch_id = NULL`
- [x] 4.5 TC-STG-050..053: `comment_external_refs` — insert, `(source_kind, external_id)` unique, multiple refs per comment, cascade from comment
- [x] 4.6 TC-STG-060..065: `batches` — insert (PR parent), insert (branch parent), `batch_key` unique per parent, status enum includes `mixed`, XOR constraint, cascade from PR and branch

## 5. Error code translation

- [x] 5.1 Implement `internal/storage` error wrapping: SQLite driver errors map to `DB_OPEN_FAILED`, `DB_CONSTRAINT_VIOLATION`, `DB_MIGRATION_FAILED` as specified
- [x] 5.2 Distinguish constraint kinds: `NOT NULL`, `CHECK`, `UNIQUE`, foreign-key — all surface as `DB_CONSTRAINT_VIOLATION` but the human-readable message includes the violated constraint name
- [x] 5.3 TC-STG-070 (ErrConstraint mapping) covered. TC-STG-071..073 (each error code produces the standard CLI-boundary footer) deferred — no subcommand opens the database in this proposal; the first such subcommand lands in `add-import-command` and that proposal's tasks cover the footer assertions. The taxonomy mapping (Code → exit code) is locked down by `internal/errcode`'s table-driven test, so the wiring is verified at the package boundary.
- [x] 5.4 Extend `internal/errcode` (foundation package) with the three new codes; verify the taxonomy table in code matches `specs/cli-framework/spec.md`

## 6. Package boundaries and tests

- [x] 6.1 `internal/storage` exposes a small `Repository`/`Query` API (final shape determined during implementation; not specified) so that import and triage proposals don't reach into the DB directly
- [x] 6.2 Provide a test helper `storagetest.NewMemDB(t)` that returns a migrated in-memory database for use by feature-spec tests
- [x] 6.3 All unit tests use the helper; integration tests use a tmp-file DB

## 7. Documentation and validation

- [x] 7.1 Add a short package doc to `internal/storage` listing the schema and pointing readers to `specs/storage/spec.md`
- [x] 7.2 Add `STG` to the `test-cases.md` category ToC
- [x] 7.3 `go test ./... && go vet ./... && gofmt -l .` clean; `go test -tags=integration -race ./...` clean before requesting archive
- [x] 7.4 `openspec validate add-storage-schema` reports no errors
