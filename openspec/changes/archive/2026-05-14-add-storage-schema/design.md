## Context

tai needs a single, durable, queryable home for triaged review comments. Today's `pr-review-triage` skill writes a JSON sidecar per PR; tai replaces this with a per-user SQLite database that the CLI mediates. Every other tai capability — install, import, triage state, the bundled slash commands — reads or writes through this schema.

The user has already nailed down the shape at a high level:

- One database per user, global (not per-repo).
- Comments live under either a PR or a branch (not all reviews are tied to a PR — a user can run a local AI review against a branch and triage those comments).
- The fields are mandatory (`category`, `severity`, `why_fix`, `suggested_fix`, `consequences`); `batch_id` is nullable.
- Idempotent re-import requires preserving external provenance.
- The storage layer is non-opinionated about *source* of comments — GitHub bot, human, locally-run AI, all stored identically.

This document records the technical decisions behind the schema, the migration runner, and the dependency choices, with alternatives considered. The corresponding `specs/storage/spec.md` carries the normative requirements.

## Goals / Non-Goals

**Goals:**

- A schema that captures the full set of fields the existing `pr-review-triage` skill records, with `NOT NULL` enforcement on the five enrichment fields.
- A model that treats PRs and standalone branches symmetrically as "review targets" without resorting to polymorphic-kind tables.
- Idempotent re-import: running `/tai:import` twice for the same PR doesn't duplicate or destroy state.
- A migration mechanism simple enough to maintain by hand for the lifetime of the project — embedded numbered SQL files, no third-party migration library.
- Cross-platform single-binary distribution. The SQLite driver MUST be pure Go.

**Non-Goals:**

- Query API design. This proposal defines the schema; how subcommands query it is each feature proposal's concern.
- A `tai forget` verb. Schema supports cascade-delete; the verb is specified in `add-triage-state`.
- Encryption at rest. The database stores PR review metadata, not secrets. If a user needs encryption they can use a filesystem-level mechanism.
- Multi-user / shared / synced storage. Each user's tai install is independent.
- Schema versioning that supports downgrades. Migrations are forward-only; users who need an older schema use an older binary.
- Backup/restore tooling. Deferred until someone asks.

## Decisions

### D1. Two parent tables (`prs`, `branches`) over a unified `review_scopes` table

Comments and batches belong to either a PR or a branch. Both options were considered:

- **A: Unified `review_scopes` table** — one parent with `scope_kind`, `pr_number`, `branch_name`, `title`, `url` columns, where some are nullable depending on `kind`. Uniform joins; awkward nullable-cosmetic columns.
- **B: Two parent tables** — `prs` and `branches` separately, with an XOR `CHECK` constraint on `comments.pr_id` / `comments.branch_id`. Cleaner per-table semantics; slightly awkward "all comments for current scope" queries (handled with a small helper).

Chose **B**. The XOR constraint is a one-liner. PRs and branches are first-class with their own columns now (PR has `head_branch`, `url`; branch has just `name`) and will diverge further as PR-specific concepts (merge state, head SHA, review state) accrue. Polymorphic-kind tables tend to accumulate per-kind cosmetic columns that the schema can't enforce — the XOR + dedicated table approach makes the constraint structural.

### D2. Comments link to exactly one parent via an XOR constraint

```sql
comments(
  pr_id     INTEGER REFERENCES prs(id)      ON DELETE CASCADE,
  branch_id INTEGER REFERENCES branches(id) ON DELETE CASCADE,
  CHECK ((pr_id IS NOT NULL AND branch_id IS NULL) OR
         (pr_id IS NULL     AND branch_id IS NOT NULL)),
  …
)
```

Batches mirror the same shape. The DB enforces "exactly one parent" — no application-layer guarding required.

**Alternatives considered:** a single `parent_id` plus a `parent_kind` column. Simpler row, but loses foreign-key integrity (the FK can't be conditional on `parent_kind`). XOR with two FKs is the SQL-native way.

### D3. PR head branch denormalised onto `prs`

The PR row carries `head_branch TEXT NOT NULL`. This avoids a `gh api` round-trip for `tai status` (which surfaces the current branch's PR by looking up `branches.name` or `prs.head_branch` against the current git context) and lets the CLI work offline.

**Alternatives considered:** force a join to `branches` whenever the head branch is needed. Tighter normalisation but worse offline behaviour. Branch names are short strings; the duplication is cheap.

### D4. External-source provenance lives in `comment_external_refs`

```sql
comment_external_refs(
  id         INTEGER PRIMARY KEY,
  comment_id INTEGER NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
  source_kind TEXT NOT NULL,   -- e.g. 'github-pr-comment', 'github-review-body',
                               --       'github-issue-comment', 'manual'
  external_id TEXT NOT NULL,   -- stringified vendor id, e.g. '12345'
  reviewer   TEXT,             -- author login when known
  UNIQUE(source_kind, external_id)
)
```

The unique constraint on `(source_kind, external_id)` makes idempotent re-import a single upsert per ref: look up the ref, find or create the comment, attach.

The `comments.source` column remains as a denormalised display string (`"coderabbit + greptile"`) populated by the importer. Querying *which external ids* a comment came from joins through `comment_external_refs`.

**Alternatives considered:**

- Inline JSON column `external_refs` on `comments`. Cheaper to write (no join). Lookup-by-external-id needs `json_each`, which works but isn't indexable in older SQLite versions. The user explicitly chose the child-table approach for v1.
- Skip external refs entirely; refuse re-import with a `--force` flag. Cheap, but bad UX when PRs accumulate new review activity over time.

### D5. Status enum implemented via `CHECK` constraints, not separate tables

```sql
comments.status TEXT NOT NULL DEFAULT 'pending'
  CHECK (status IN ('pending', 'accepted', 'dismissed', 'completed'))

batches.status TEXT NOT NULL DEFAULT 'pending'
  CHECK (status IN ('pending', 'accepted', 'dismissed', 'completed', 'mixed'))
```

The set of valid values is small and stable. A `statuses` lookup table would add joins for no benefit.

**Alternatives considered:** lookup tables. Worth the overhead only if the set of values changed dynamically; it does not.

### D6. Resolution / dismissal payload lives on the comment row

`comments` carries three nullable columns that are populated only when a state transition records them:

```sql
resolution      TEXT,  -- set on 'accepted' or 'completed'
dismissed_by    TEXT,  -- set on 'dismissed'
dismiss_reason  TEXT   -- set on 'dismissed'
```

This keeps "the row tells the whole story of this comment" true without a join. The state-transition spec (`add-triage-state`) is responsible for ensuring these columns are populated consistently with `status`.

**Alternatives considered:** an audit-log table (`comment_state_transitions`) recording each transition. Useful if we ever want history; overkill for v1 where the user just wants current state. Easy to add later as a parallel table without breaking the current row shape.

### D7. Cascade rules

| Parent delete | Effect |
|---|---|
| `repos` | `prs`, `branches` deleted (CASCADE) |
| `prs` | `comments` and `batches` for that PR deleted (CASCADE) |
| `branches` | `comments` and `batches` for that branch deleted (CASCADE) |
| `comments` | `comment_external_refs` deleted (CASCADE) |
| `batches` | Member comments' `batch_id` set to NULL (`ON DELETE SET NULL`) |

The asymmetry on `batches` is deliberate: deleting a batch is a grouping change, not a "delete these comments" operation. The comments survive as ungrouped rows.

`tai forget` (defined in `add-triage-state`) drives deletions from the top of the cascade tree:

- `tai forget --repo acme/app` deletes the `repos` row, cascading everything.
- `tai forget --pr 142` deletes the matching `prs` row.
- `tai forget --branch feat/x` deletes the matching `branches` row.

### D8. Migration runner: embedded numbered SQL files, no third-party library

Migrations live under `internal/storage/migrations/` as `001_init.sql`, `002_*.sql`, etc., embedded into the binary via `//go:embed`. On every CLI startup the runner:

1. Opens the database (creating the file if missing).
2. Sets `PRAGMA journal_mode=WAL`, `PRAGMA foreign_keys=ON`, `PRAGMA busy_timeout=1000`.
3. Reads `migrations` table; on missing-table error, treats it as version 0.
4. Applies each unapplied migration in version order inside a transaction. On failure, the transaction rolls back and the CLI exits with `DB_MIGRATION_FAILED`.
5. After each successful migration, inserts the version into the `migrations` table.

The `migrations` table itself is created by the first migration's idempotent header (`CREATE TABLE IF NOT EXISTS migrations …`).

**Alternatives considered:**

- `golang-migrate`, `goose`, `sql-migrate`, `pressly/goose` — popular libraries. All add a dependency, all carry more features than this project needs (down migrations, dirty-state recovery, CLI tools). We will never write a down migration in tai; forward-only is acceptable.
- Embedding migrations as Go functions instead of SQL files. SQL is reviewable in PRs and copy-pasteable into a `sqlite3` shell for debugging.

### D9. SQLite driver: `modernc.org/sqlite` (pure Go, no CGo)

The binary must cross-compile cleanly on Linux, macOS, and Windows without a C toolchain.

**Alternatives considered:**

- `mattn/go-sqlite3` — the canonical Go binding. Requires CGo, complicates cross-compilation, breaks single-binary distribution unless every build environment has gcc/clang.
- `crawshaw/sqlite` — also CGo.
- `modernc.org/sqlite` — a pure-Go SQLite3 transliteration. Slightly slower than CGo on hot paths; tai's working-set is tens to low-hundreds of comments, so performance is not a concern.

### D10. Connection lifecycle: single-process, single connection, opened per command

Each invocation of `tai <verb>` opens the database, runs migrations, performs its work, and closes the database. Long-lived processes are not a use case.

WAL mode + a 1-second busy-timeout handles the edge case of two `tai` processes racing (e.g. a human running `tai list` while a Claude invocation runs `tai accept` in another terminal).

**Alternatives considered:** keep a connection pool for read-mostly workloads. Premature for v1; CLI invocations are short.

## Risks / Trade-offs

- **[Schema migration is forward-only]** Users who downgrade to an older binary against a newer schema get undefined behaviour (likely missing-column errors on read). → Documented limitation. If it ever bites, recovery is "back up the DB, point at it with an older binary, accept data loss for the new columns". Acceptable for v1.

- **[Pure-Go SQLite performance]** `modernc.org/sqlite` is ~2-3x slower than the CGo binding on heavy workloads. tai's working set is tiny. → Accept the trade-off; revisit if profiling ever surfaces SQLite as a bottleneck.

- **[XOR CHECK constraint on `comments`]** Programmatically inserting a comment that's a child of both a PR and a branch (or neither) fails with `DB_CONSTRAINT_VIOLATION`. The error code surfaces this clearly to the importer. → Importers MUST set exactly one of `pr_id`/`branch_id`; this is part of the import contract specified in `add-import-command`.

- **[Reviewer attribution lives in `comment_external_refs.reviewer`]** Pulling "all comments by reviewer X across all my repos" requires a join. → Acceptable; this is not a hot path. If it ever is, add an index on `comment_external_refs.reviewer`.

- **[No audit trail of state transitions]** Once a comment is `dismissed`, the previous state is lost (only the current snapshot survives). → Acceptable; users who want history have git for the codebase. If audit-log requirements emerge, add a `comment_state_transitions` table in a follow-on proposal without breaking existing schema.

- **[CASCADE makes `tai forget --repo` destructive]** A typo on `tai forget --repo acme/wrong-app` wipes a repo's data. → The `tai forget` verb (specified in `add-triage-state`) MUST prompt for confirmation. The schema's CASCADE behaviour is correct; the safety belongs at the verb level.

- **[Status `'mixed'` on batches]** Adds a value the user might not encounter for some time, but baking it in v1 avoids a CHECK-constraint migration later when individual-override semantics ship. → Cheap to include now.

- **[Bundle the SQLite driver = ~1MB binary growth]** `modernc.org/sqlite` is large. → Acceptable; single-binary distribution is the priority.

## Migration Plan

Not applicable. There is no prior tai database state in the world yet. The first migration (`001_init.sql`) creates the entire v1 schema. The migration runner is verified by integration tests that exercise the "fresh database" and "second startup is a no-op" paths.

## Open Questions

(None remaining — D1–D10 cover the decisions surfaced during exploration. Future schema extensions are the concern of the proposals that introduce them.)
