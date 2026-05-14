## Why

`tai`'s value is persistent, structured storage of triaged review comments — replacing the per-project JSON sidecar produced by today's `pr-review-triage` skill. Every subsequent feature proposal (`add-install-command`, `add-import-command`, `add-triage-state`, `add-triage-command`) reads or writes this state. Defining the schema, identity model, and migration mechanism once, in one place, prevents each feature from re-litigating the data model.

The storage layer is intentionally non-opinionated about where comments originated. Imports from GitHub PR review tools (CodeRabbit, Greptile), from human reviewers leaving comments on a PR, and from a locally-run AI review against a branch all flow into the same schema. The schema cares about the shape of a triaged comment, not its provenance.

## What Changes

- Introduce a per-user SQLite database at `<data_dir>/tai.db` (where `<data_dir>` is the path resolved per the `cli-framework` capability). The database is created lazily on first write; it is opened with WAL mode, foreign keys enabled, and a 1 second busy-timeout.
- Introduce a tables-and-relationships schema that models four user-facing concepts: a **repo**, a **review target** (either a PR or a branch), a **comment**, and a **batch** (a group of related comments). Repos are keyed by `owner/name`; targets are children of a repo; comments are children of exactly one target.
- Use two parent tables (`prs`, `branches`) rather than a polymorphic single table. Comments and batches each carry mutually-exclusive nullable foreign keys (`pr_id` XOR `branch_id`) enforced by SQL `CHECK` constraints. PR rows carry the head branch as a denormalised column for offline display.
- Mandate the five enrichment fields documented across the OpenSpec discussions and `CLAUDE.md`: `category`, `severity`, `why_fix`, `suggested_fix`, `consequences` are all `NOT NULL` on `comments`. `batch_id` is nullable.
- Track external provenance in a dedicated child table `comment_external_refs` so that re-imports are idempotent. A single logical comment may have many external refs (e.g. an AI-deduped comment that originated from CodeRabbit and Greptile both has two refs).
- Maintain a stable status enum on every comment: `pending` → `accepted` | `dismissed` | `completed`. `resolution`, `dismissed_by`, and `dismiss_reason` are nullable columns populated by state-transition verbs (specified in `add-triage-state`).
- Maintain a parallel status enum on every batch: `pending` | `accepted` | `dismissed` | `completed` | `mixed`.
- Cascade deletes from `repos` through `prs`, `branches`, `comments`, `comment_external_refs`, and `batches`. Deleting a `batch` does NOT delete its member comments; instead the member comments' `batch_id` is set to `NULL`.
- Introduce a `migrations` table and a build-time-embedded migration runner. Numbered SQL files (`001_init.sql`, `002_…`) are applied in order on every CLI startup; applied migrations are recorded by version. The initial migration created by this proposal defines the full v1 schema.
- Reserve new error codes in the `cli-framework` taxonomy: `DB_OPEN_FAILED` (exit 3), `DB_MIGRATION_FAILED` (exit 3), `DB_CONSTRAINT_VIOLATION` (exit 3). These extend (not modify) the foundation taxonomy.

## Capabilities

### New Capabilities

- `storage`: The SQLite database location, schema (tables, columns, types, constraints, indexes, cascade rules), the migration runner, the connection-open policy (WAL mode, foreign keys, busy-timeout), and the storage-layer error codes.

### Modified Capabilities

- `cli-framework`: Extends the error-code taxonomy with three storage-layer codes (`DB_OPEN_FAILED`, `DB_MIGRATION_FAILED`, `DB_CONSTRAINT_VIOLATION`). The taxonomy is append-only per the foundation spec; this is an additive change, not a renaming.

## Impact

- Introduces a single new external dependency: a SQLite driver. The candidate is `modernc.org/sqlite` (pure-Go, no CGo) so tai remains a single static binary that cross-compiles cleanly. Final driver choice is a `design.md` decision.
- Subsequent feature proposals (`add-import-command`, `add-triage-state`, `add-triage-command`, plus the housekeeping verbs `tai forget`/`tai status` rolled into `add-triage-state`) read and write through this schema. They MUST NOT introduce their own tables; new tables require a fresh storage-extending proposal.
- No user-facing CLI verbs are introduced by this proposal. It is pure schema and infrastructure. The first user-observable behaviour built on top is `add-install-command` (which doesn't touch the DB) followed by `add-import-command` (which writes the first rows).
- The data directory location and the `--repo` flag wired by the foundation proposal remain unchanged. This proposal puts the SQLite file at a specific path under the foundation's data directory but does not modify the directory contract itself.
