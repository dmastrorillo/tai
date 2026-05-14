-- 001_init.sql — tai's v1 schema.
--
-- Six tables establish the data model:
--   repos              — repo identity (`owner/name`)
--   prs                — PRs under a repo
--   branches           — standalone branches under a repo
--   batches            — groupings of related comments under a PR or branch
--   comments           — triaged review comments under a PR or branch
--   comment_external_refs — provenance for idempotent re-import
--
-- Plus the framework's own `migrations` table tracking which versions
-- have been applied.
--
-- See openspec/specs/storage/spec.md for the normative contract.

CREATE TABLE IF NOT EXISTS migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at INTEGER NOT NULL
);

CREATE TABLE repos (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_name TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL
);

CREATE TABLE prs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id     INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    number      INTEGER NOT NULL,
    title       TEXT NOT NULL,
    url         TEXT NOT NULL,
    head_branch TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    UNIQUE(repo_id, number)
);

CREATE TABLE branches (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id    INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE(repo_id, name)
);

-- Batches sit between a PR/branch and its comments. They group comments
-- that share a corrective action. Exactly one parent (XOR pr_id / branch_id).
CREATE TABLE batches (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    pr_id      INTEGER NULL REFERENCES prs(id)      ON DELETE CASCADE,
    branch_id  INTEGER NULL REFERENCES branches(id) ON DELETE CASCADE,
    batch_key  TEXT NOT NULL,
    title      TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','accepted','dismissed','completed','mixed')),
    created_at INTEGER NOT NULL,
    CHECK (
        (pr_id IS NOT NULL AND branch_id IS NULL)
        OR
        (pr_id IS NULL AND branch_id IS NOT NULL)
    )
);
CREATE UNIQUE INDEX idx_batches_pr_key     ON batches(pr_id, batch_key)     WHERE pr_id IS NOT NULL;
CREATE UNIQUE INDEX idx_batches_branch_key ON batches(branch_id, batch_key) WHERE branch_id IS NOT NULL;

CREATE TABLE comments (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    pr_id          INTEGER NULL REFERENCES prs(id)      ON DELETE CASCADE,
    branch_id      INTEGER NULL REFERENCES branches(id) ON DELETE CASCADE,
    batch_id       INTEGER NULL REFERENCES batches(id)  ON DELETE SET NULL,

    severity       TEXT NOT NULL
        CHECK (severity IN ('critical','major','minor','nitpick')),
    category       TEXT NOT NULL
        CHECK (category IN ('security','correctness','feature-regression','code-quality','performance','testing')),
    file           TEXT NOT NULL,
    lines          TEXT NOT NULL,
    source         TEXT NOT NULL,
    title          TEXT NOT NULL,
    description    TEXT NOT NULL,
    why_fix        TEXT NOT NULL,
    suggested_fix  TEXT NOT NULL,
    consequences   TEXT NOT NULL,

    status         TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','accepted','dismissed','completed')),
    resolution     TEXT NULL,
    dismissed_by   TEXT NULL,
    dismiss_reason TEXT NULL,

    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,

    -- Exactly one parent: PR or branch.
    CHECK (
        (pr_id IS NOT NULL AND branch_id IS NULL)
        OR
        (pr_id IS NULL AND branch_id IS NOT NULL)
    )
);
CREATE INDEX idx_comments_pr_status     ON comments(pr_id, status);
CREATE INDEX idx_comments_branch_status ON comments(branch_id, status);
CREATE INDEX idx_comments_batch         ON comments(batch_id);

CREATE TABLE comment_external_refs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    comment_id  INTEGER NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
    source_kind TEXT NOT NULL,
    external_id TEXT NOT NULL,
    reviewer    TEXT NULL,
    UNIQUE(source_kind, external_id)
);
CREATE INDEX idx_external_refs_comment ON comment_external_refs(comment_id);
